package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"vigil/internal/collector"
	"vigil/internal/config"
)

type discoveredMount struct {
	Device    string
	Path      string
	FSType    string
	Size      uint64
	Suggested bool
	Selected  bool
}

func runMountsCmd(args []string) {
	fs := flag.NewFlagSet("mounts", flag.ExitOnError)
	configPath := fs.String("config", "config.toml", "path to config file")
	fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load config: %v\n", err)
	}
	configured := make(map[string]bool, len(cfg.MountChecks))
	for _, mc := range cfg.MountChecks {
		configured[mc.Path] = true
	}

	mounts := discoverMounts()
	var available []discoveredMount
	for _, m := range mounts {
		if !configured[m.Path] {
			available = append(available, m)
		}
	}

	if len(available) == 0 {
		if len(configured) > 0 {
			fmt.Println("All detected mounts are already configured.")
		} else {
			fmt.Println("No non-virtual mounts detected.")
		}
		return
	}

	pm := pickerModel{mounts: available}
	result, err := tea.NewProgram(pm).Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "picker error: %v\n", err)
		os.Exit(1)
	}
	final := result.(pickerModel)
	if final.cancelled {
		return
	}

	var selected []discoveredMount
	for _, m := range final.mounts {
		if m.Selected {
			selected = append(selected, m)
		}
	}
	if len(selected) == 0 {
		fmt.Println("No mounts selected.")
		return
	}

	var snippet strings.Builder
	for _, m := range selected {
		snippet.WriteString(fmt.Sprintf("\n[[mount_checks]]\npath = %q\n", m.Path))
	}

	fmt.Println("\nThe following will be added to your config:")
	fmt.Println(snippet.String())
	fmt.Printf("Write to %s? [Y/n] ", *configPath)

	cm := confirmModel{}
	cResult, err := tea.NewProgram(cm).Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "confirm error: %v\n", err)
		os.Exit(1)
	}
	cFinal := cResult.(confirmModel)

	if !cFinal.confirmed {
		fmt.Println("\nTOML snippet (copy manually):")
		fmt.Println(snippet.String())
		return
	}

	f, err := os.OpenFile(*configPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening config: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	if _, err := f.WriteString(snippet.String()); err != nil {
		fmt.Fprintf(os.Stderr, "error writing config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Added %d mount check(s) to %s\n", len(selected), *configPath)

	if _, err := config.Load(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: config validation failed after write: %v\n", err)
	}
}

type pickerModel struct {
	mounts    []discoveredMount
	cursor    int
	cancelled bool
}

func (m pickerModel) Init() tea.Cmd { return nil }

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case "j", "down":
			if m.cursor < len(m.mounts)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case " ":
			m.mounts[m.cursor].Selected = !m.mounts[m.cursor].Selected
		case "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m pickerModel) View() string {
	var sb strings.Builder
	sb.WriteString("Select mounts to watch (space=toggle, enter=confirm, q=quit):\n\n")

	// Find max path width for right-aligning the description columns.
	maxPath := 0
	for _, mt := range m.mounts {
		if len(mt.Path) > maxPath {
			maxPath = len(mt.Path)
		}
	}
	if maxPath < 12 {
		maxPath = 12
	}

	for i, mt := range m.mounts {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		check := "[ ]"
		if mt.Selected {
			check = "[x]"
		}
		suggested := "  "
		if mt.Suggested {
			suggested = "* "
		}
		sb.WriteString(fmt.Sprintf("%s%s %s%-*s %10s %10s  %s\n",
			cursor, check, suggested, maxPath, mt.Path, mt.FSType, formatBytes(mt.Size), mt.Device))
	}
	return sb.String()
}

type confirmModel struct {
	confirmed bool
	done      bool
}

func (m confirmModel) Init() tea.Cmd { return nil }

func (m confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y", "enter":
			m.confirmed = true
			m.done = true
			return m, tea.Quit
		case "n", "N", "q", "esc", "ctrl+c":
			m.confirmed = false
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m confirmModel) View() string {
	return ""
}

func discoverMounts() []discoveredMount {
	mountsPath := collector.HostMountsPath()
	data, err := os.ReadFile(mountsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v\n", mountsPath, err)
		return nil
	}

	var mounts []discoveredMount
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		device, path, fstype := fields[0], fields[1], fields[2]
		if !collector.ShouldCollectFS(fstype) {
			continue
		}
		if path == "/" {
			continue
		}
		var size uint64
		var stat syscall.Statfs_t
		if syscall.Statfs(path, &stat) == nil {
			size = stat.Blocks * uint64(stat.Bsize)
		}
		suggested := isSuggestedMount(path, device)
		mounts = append(mounts, discoveredMount{
			Device:    device,
			Path:      path,
			FSType:    fstype,
			Size:      size,
			Suggested: suggested,
		})
	}
	return mounts
}

func formatBytes(b uint64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)
	switch {
	case b >= TB:
		return fmt.Sprintf("%.1f TB", float64(b)/float64(TB))
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func isSuggestedMount(path, device string) bool {
	if strings.HasPrefix(path, "/media/") || strings.HasPrefix(path, "/mnt/") {
		return true
	}
	base := filepath.Base(device)
	if strings.HasPrefix(base, "sd") || strings.HasPrefix(base, "usb") {
		return true
	}
	return false
}
