package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"vigil/internal/alert"
	"vigil/internal/checker"
	"vigil/internal/collector"
	"vigil/internal/config"
	"vigil/internal/metric"
	"vigil/internal/notify"
	"vigil/internal/ntfy"
	"vigil/internal/store"
	"vigil/internal/tui"
)

// version is stamped at build time by goreleaser via -ldflags.
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		runInitCmd(os.Args[2:])
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "mounts" {
		runMountsCmd(os.Args[2:])
		return
	}

	configPath := flag.String("config", "config.toml", "path to config file")
	headless := flag.Bool("headless", false, "run collector and storage without TUI")
	once := flag.Bool("once", false, "collect one snapshot and exit")
	jsonOutput := flag.Bool("json", false, "write JSON output")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}
	if *jsonOutput && !*once {
		log.Fatal("--json requires --once")
	}
	if *once && !*jsonOutput {
		log.Fatal("--once requires --json")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if *once {
		if err := runOnceJSON(os.Stdout, cfg); err != nil {
			log.Fatalf("once json: %v", err)
		}
		return
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle SIGTERM / SIGINT for clean Docker shutdown.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigs
		cancel()
	}()

	var svcChecker *checker.ServiceChecker
	if len(cfg.HTTPChecks) > 0 || len(cfg.PortChecks) > 0 {
		svcChecker = checker.New(cfg.Services, cfg.HTTPChecks, cfg.PortChecks)
		log.Printf("service checks enabled: %d http, %d tcp", len(cfg.HTTPChecks), len(cfg.PortChecks))
	}

	runner, snapshots := collector.New(cfg.Interval.Duration, cfg.Docker.Socket, cfg.MountChecks, svcChecker)
	go runner.Run(ctx)
	if svcChecker != nil {
		go svcChecker.Run(ctx)
	}

	if cfg.Docker.Socket != "" {
		log.Printf("docker monitoring enabled: %s", cfg.Docker.Socket)
	}

	alertEngine := alert.New(cfg.Alerts)

	notifier, mute, err := buildNotifier(cfg.Notifications)
	if err != nil {
		log.Fatal(err)
	}

	// Restore any alerts that were active before the last shutdown.
	var restoredAlerts []alert.State
	if persisted, err := db.ActiveAlerts(); err == nil && len(persisted) > 0 {
		for _, a := range persisted {
			restoredAlerts = append(restoredAlerts, alert.State{
				Name: a.Name, Message: a.Message, FiredAt: a.FiredAt,
			})
		}
		alertEngine.Restore(restoredAlerts)
		log.Printf("restored %d active alert(s) from database", len(restoredAlerts))
	}

	mountHandler := alert.NewMountHandler()
	if len(cfg.MountChecks) > 0 {
		mountHandler.Restore(restoredAlerts)
		log.Printf("mount watchdog enabled: %d paths", len(cfg.MountChecks))
	}

	serviceHandler := alert.NewServiceHandler(cfg.Services.FailuresBeforeAlert)
	if len(cfg.HTTPChecks) > 0 || len(cfg.PortChecks) > 0 {
		serviceHandler.Restore(restoredAlerts)
	}

	if *headless {
		runHeadless(ctx, db, cfg, snapshots, alertEngine, notifier, mountHandler, serviceHandler)
		return
	}

	// Check if stdout is a TTY; fall back to headless if not.
	if !isTerminal(os.Stdout) {
		log.Println("stdout is not a TTY, running in headless mode")
		runHeadless(ctx, db, cfg, snapshots, alertEngine, notifier, mountHandler, serviceHandler)
		return
	}

	runTUI(ctx, cancel, db, cfg, snapshots, alertEngine, restoredAlerts, notifier, mute, mountHandler, serviceHandler)
}

type loopConfig struct {
	writeBufCap     int
	containerBufCap int
	flushInterval   time.Duration
	purgeInterval   time.Duration
	retention       time.Duration
	throttleFiring  bool
	onSnapshot      func(collector.Snapshot)
	onAlert         func(fired, resolved []alert.State)
}

func runTUI(
	ctx context.Context,
	cancel context.CancelFunc,
	db *store.DB,
	cfg config.Config,
	snapshots <-chan collector.Snapshot,
	eng *alert.Engine,
	restoredAlerts []alert.State,
	n notify.Notifier,
	mute *notify.Mute,
	mountHandler *alert.MountAlertHandler,
	serviceHandler *alert.ServiceAlertHandler,
) {
	model := tui.New(cfg.Theme, func(name string) {
		if err := db.DismissAlert(name); err != nil {
			log.Printf("dismiss alert: %v", err)
		}
		eng.Dismiss(name)
		mountHandler.Dismiss(name)
		serviceHandler.Dismiss(name)
	}, mute)
	if len(restoredAlerts) > 0 {
		model.SetAlerts(restoredAlerts)
	}

	throttleFiring := false
	for _, a := range restoredAlerts {
		if a.Name == metric.AlertThrottle {
			throttleFiring = true
			break
		}
	}

	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
	)

	done := make(chan struct{})
	go func() {
		defer close(done)
		runLoop(ctx, db, snapshots, eng, n, mountHandler, serviceHandler, loopConfig{
			writeBufCap:     4096,
			containerBufCap: 256,
			flushInterval:   5 * time.Minute,
			purgeInterval:   1 * time.Hour,
			retention:       cfg.Retention.Duration,
			throttleFiring:  throttleFiring,
			onSnapshot: func(snap collector.Snapshot) {
				for i, m := range snap.Mounts {
					if mountHandler.IsUnstable(m.Path) {
						snap.Mounts[i].Unstable = true
					}
				}
				p.Send(tui.SnapshotMsg(snap))
			},
			onAlert: func(fired, resolved []alert.State) {
				p.Send(tui.AlertMsg{Fired: fired, Resolved: resolved})
			},
		})
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tui error: %v\n", err)
		os.Exit(1)
	}
	cancel()
	<-done
}

func runHeadless(
	ctx context.Context,
	db *store.DB,
	cfg config.Config,
	snapshots <-chan collector.Snapshot,
	eng *alert.Engine,
	n notify.Notifier,
	mountHandler *alert.MountAlertHandler,
	serviceHandler *alert.ServiceAlertHandler,
) {
	throttleFiring := false
	if persisted, err := db.ActiveAlerts(); err == nil {
		for _, a := range persisted {
			if a.Name == metric.AlertThrottle {
				throttleFiring = true
				break
			}
		}
	}

	runLoop(ctx, db, snapshots, eng, n, mountHandler, serviceHandler, loopConfig{
		writeBufCap:     256,
		containerBufCap: 64,
		flushInterval:   30 * time.Second,
		purgeInterval:   5 * time.Minute,
		retention:       cfg.Retention.Duration,
		throttleFiring:  throttleFiring,
	})
}

func runLoop(
	ctx context.Context,
	db *store.DB,
	snapshots <-chan collector.Snapshot,
	eng *alert.Engine,
	n notify.Notifier,
	mountHandler *alert.MountAlertHandler,
	serviceHandler *alert.ServiceAlertHandler,
	lc loopConfig,
) {
	writeBuf := make([]store.Reading, 0, lc.writeBufCap)
	containerBuf := make([]store.ContainerReading, 0, lc.containerBufCap)
	serviceBuf := make([]store.ServiceCheckReading, 0, 64)
	var lastCycleTime time.Time
	throttleFiring := lc.throttleFiring

	flushTicker := time.NewTicker(lc.flushInterval)
	purgeTicker := time.NewTicker(lc.purgeInterval)
	defer flushTicker.Stop()
	defer purgeTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			flushReadings(db, writeBuf)
			flushContainerReadings(db, containerBuf)
			flushServiceCheckReadings(db, serviceBuf)
			return

		case snap, ok := <-snapshots:
			if !ok {
				flushReadings(db, writeBuf)
				flushContainerReadings(db, containerBuf)
				flushServiceCheckReadings(db, serviceBuf)
				return
			}

			values := snapshotToValues(snap)
			persistAlerts(ctx, db, eng, values, lc.onAlert, n)
			handleThrottleAlert(ctx, snap, &throttleFiring, db, lc.onAlert, n)
			handleMountAlerts(ctx, snap, mountHandler, db, lc.onAlert, n)

			ct := snap.ServiceCycleTime
			if !ct.IsZero() && ct.After(lastCycleTime) {
				lastCycleTime = ct
				handleServiceAlerts(ctx, snap, serviceHandler, db, lc.onAlert, n)
				for _, s := range snap.Services {
					serviceBuf = append(serviceBuf, store.ServiceCheckReading{
						Name:       s.Name,
						CheckType:  s.CheckType,
						Up:         s.Up,
						StatusCode: s.StatusCode,
						LatencyMs:  s.Latency.Milliseconds(),
						Error:      s.Error,
						Timestamp:  s.CheckedAt,
					})
				}
			}

			if lc.onSnapshot != nil {
				lc.onSnapshot(snap)
			}

			writeBuf = appendReadings(writeBuf, snap)
			containerBuf = appendContainerReadings(containerBuf, snap)

		case <-flushTicker.C:
			flushReadings(db, writeBuf)
			writeBuf = writeBuf[:0]
			flushContainerReadings(db, containerBuf)
			containerBuf = containerBuf[:0]
			flushServiceCheckReadings(db, serviceBuf)
			serviceBuf = serviceBuf[:0]

		case <-purgeTicker.C:
			cutoff := time.Now().Add(-lc.retention)
			if err := db.PurgeOlderThan(cutoff); err != nil {
				log.Printf("purge error: %v", err)
			}
			if err := db.PurgeContainerMetrics(cutoff); err != nil {
				log.Printf("purge container metrics error: %v", err)
			}
			if err := db.PurgeServiceChecks(cutoff); err != nil {
				log.Printf("purge service checks error: %v", err)
			}
		}
	}
}

func buildNotifier(cfg config.Notifications) (notify.Notifier, *notify.Mute, error) {
	var notifiers notify.Multi
	if u := cfg.DiscordWebhook; u != "" {
		if err := notify.ValidateURL(u); err != nil {
			return nil, nil, fmt.Errorf("discord_webhook: %w", err)
		}
		notifiers = append(notifiers, notify.Discord{WebhookURL: u})
		log.Printf("discord notifications enabled")
	}
	if u := cfg.WebhookURL; u != "" {
		if err := notify.ValidateURL(u); err != nil {
			return nil, nil, fmt.Errorf("webhook_url: %w", err)
		}
		notifiers = append(notifiers, notify.Webhook{URL: u})
		log.Printf("webhook notifications enabled")
	}
	if cfg.NtfyTopic != "" {
		if err := ntfy.ValidateTopic(cfg.NtfyTopic); err != nil {
			return nil, nil, fmt.Errorf("ntfy_topic: %w", err)
		}
		if err := ntfy.ValidateServerURL(cfg.NtfyServer); err != nil {
			return nil, nil, fmt.Errorf("ntfy_server: %w", err)
		}
		notifiers = append(notifiers, notify.Ntfy{Server: cfg.NtfyServer, Topic: cfg.NtfyTopic})
		log.Printf("ntfy notifications enabled")
	}
	if len(notifiers) == 0 {
		return nil, nil, nil
	}

	mute := &notify.Mute{}
	var windows []notify.QuietWindow
	if qh := cfg.QuietHours; len(qh) > 0 {
		var err error
		windows, err = notify.ParseQuietHours(qh)
		if err != nil {
			return nil, nil, fmt.Errorf("quiet_hours: %w", err)
		}
		log.Printf("quiet hours enabled: %v", qh)
	}
	return notify.Quiet{Inner: notifiers, Windows: windows, Mute: mute}, mute, nil
}

// persistAlerts evaluates alert rules, writes state changes to the DB,
// sends outbound notifications, and calls onAlert when non-nil.
func persistAlerts(ctx context.Context, db *store.DB, eng *alert.Engine, values map[string]float64, onAlert func(fired, resolved []alert.State), n notify.Notifier) {
	fired := eng.Evaluate(values)
	resolved := eng.Resolved(values)
	persistAlertChanges(ctx, fired, resolved, db, onAlert, n, "alert")
}

// persistAlertChanges writes fired/resolved alert state to the DB, sends
// notifications, and calls onAlert when non-nil.
func persistAlertChanges(
	ctx context.Context,
	fired, resolved []alert.State,
	db *store.DB,
	onAlert func(fired, resolved []alert.State),
	n notify.Notifier,
	logPrefix string,
) {
	if len(fired) > 0 {
		if onAlert != nil {
			onAlert(fired, nil)
		}
		for _, a := range fired {
			if err := db.WriteAlert(store.AlertRecord{
				Name: a.Name, Message: a.Message, FiredAt: a.FiredAt,
			}); err != nil {
				log.Printf("%s write alert: %v", logPrefix, err)
				continue
			}
			if n != nil {
				go func(a alert.State) {
					if err := n.Send(ctx, a, false); err != nil {
						log.Printf("%s notify fired: %v", logPrefix, err)
					}
				}(a)
			}
		}
	}
	if len(resolved) > 0 {
		if onAlert != nil {
			onAlert(nil, resolved)
		}
		for _, a := range resolved {
			if err := db.ResolveAlert(a.Name); err != nil {
				log.Printf("%s resolve alert: %v", logPrefix, err)
				continue
			}
			if n != nil {
				go func(a alert.State) {
					if err := n.Send(ctx, a, true); err != nil {
						log.Printf("%s notify resolved: %v", logPrefix, err)
					}
				}(a)
			}
		}
	}
}

// handleThrottleAlert fires or resolves a hardcoded alert based on active throttle flags.
// It manages its own firing state via the throttleFiring pointer, independent of the alert engine.
func handleThrottleAlert(
	ctx context.Context,
	snap collector.Snapshot,
	firing *bool,
	db *store.DB,
	onAlert func(fired, resolved []alert.State),
	n notify.Notifier,
) {
	if !snap.Throttle.Available {
		return
	}
	active := snap.Throttle.ActiveNow()
	if active && !*firing {
		*firing = true
		s := alert.State{
			Name:    metric.AlertThrottle,
			Message: snap.Throttle.ActiveMessage(),
			FiredAt: time.Now(),
		}
		if err := db.WriteAlert(store.AlertRecord{
			Name: s.Name, Message: s.Message, FiredAt: s.FiredAt,
		}); err != nil {
			log.Printf("write throttle alert: %v", err)
		} else if n != nil {
			go func() {
				if err := n.Send(ctx, s, false); err != nil {
					log.Printf("notify throttle fired: %v", err)
				}
			}()
		}
		if onAlert != nil {
			onAlert([]alert.State{s}, nil)
		}
	} else if !active && *firing {
		*firing = false
		s := alert.State{Name: metric.AlertThrottle, Message: "throttle condition cleared"}
		if err := db.ResolveAlert(metric.AlertThrottle); err != nil {
			log.Printf("resolve throttle alert: %v", err)
		} else if n != nil {
			go func() {
				if err := n.Send(ctx, s, true); err != nil {
					log.Printf("notify throttle resolved: %v", err)
				}
			}()
		}
		if onAlert != nil {
			onAlert(nil, []alert.State{s})
		}
	}
}

func handleMountAlerts(
	ctx context.Context,
	snap collector.Snapshot,
	handler *alert.MountAlertHandler,
	db *store.DB,
	onAlert func(fired, resolved []alert.State),
	n notify.Notifier,
) {
	if len(snap.Mounts) == 0 {
		return
	}
	fired, resolved := handler.Evaluate(snap.Mounts)
	persistAlertChanges(ctx, fired, resolved, db, onAlert, n, "mount")
}

func handleServiceAlerts(
	ctx context.Context,
	snap collector.Snapshot,
	handler *alert.ServiceAlertHandler,
	db *store.DB,
	onAlert func(fired, resolved []alert.State),
	n notify.Notifier,
) {
	if len(snap.Services) == 0 {
		return
	}
	fired, resolved := handler.Evaluate(snap.Services)
	persistAlertChanges(ctx, fired, resolved, db, onAlert, n, "service")
}

func snapshotToValues(snap collector.Snapshot) map[string]float64 {
	return snapshotMetricValues(snap)
}

func snapshotMetricValues(snap collector.Snapshot) map[string]float64 {
	v := map[string]float64{
		metric.MemPercent:  snap.Memory.Percent,
		metric.SwapPercent: snap.Memory.SwapPercent,
		metric.SwapIn:      snap.Memory.SwapInRate,
		metric.SwapOut:     snap.Memory.SwapOutRate,
		metric.Load1:       snap.Load.Load1,
		metric.Load5:       snap.Load.Load5,
		metric.Load15:      snap.Load.Load15,
	}
	if snap.CPU.Ready {
		v[metric.CPUPercent] = snap.CPU.PercentTotal
		v[metric.CPUUser] = snap.CPU.UserPercent
		v[metric.CPUSystem] = snap.CPU.SystemPercent
		v[metric.CPUIowait] = snap.CPU.IOWaitPercent
		v[metric.CPUIdle] = snap.CPU.IdlePercent
	}
	for _, d := range snap.Disks {
		v[metric.PrefixDiskPercent+d.MountPoint] = d.Percent
	}
	trackedDevices := trackedDiskDevices(snap.Disks)
	for _, d := range snap.DiskIO {
		if !isTrackedDiskDevice(d.Device, trackedDevices) {
			continue
		}
		v[metric.PrefixDiskRead+d.Device] = d.ReadRate
		v[metric.PrefixDiskWrite+d.Device] = d.WriteRate
		v[metric.PrefixDiskUtil+d.Device] = d.UtilPercent
		v[metric.PrefixDiskLatency+d.Device] = d.LatencyMs
	}
	for _, s := range snap.SDErrors {
		v[metric.PrefixSDErrors+s.Host] = float64(s.Delta)
	}
	for _, n := range snap.Network {
		v[metric.PrefixNetSend+n.Interface] = n.SendRate
		v[metric.PrefixNetRecv+n.Interface] = n.RecvRate
		v[metric.PrefixNetDrops+n.Interface] = n.DropRate
		v[metric.PrefixNetErrors+n.Interface] = n.ErrRate
	}
	for _, t := range snap.Temperature {
		v[metric.PrefixTemp+t.SensorKey] = t.Celsius
	}
	if snap.Throttle.Available {
		v[metric.ThrottleRaw] = float64(snap.Throttle.Raw)
	}
	return v
}

func appendContainerReadings(buf []store.ContainerReading, snap collector.Snapshot) []store.ContainerReading {
	ts := snap.Timestamp
	for _, c := range snap.Containers {
		buf = append(buf, store.ContainerReading{
			Name:        c.Name,
			ContainerID: c.ID,
			Status:      c.Status,
			CPUPercent:  c.CPUPercent,
			MemUsed:     c.MemUsed,
			MemLimit:    c.MemLimit,
			MemPercent:  c.MemPercent,
			Timestamp:   ts,
		})
	}
	return buf
}

func appendReadings(buf []store.Reading, snap collector.Snapshot) []store.Reading {
	ts := snap.Timestamp
	for k, v := range snapshotMetricValues(snap) {
		buf = append(buf, store.Reading{
			Metric:    k,
			Value:     v,
			Timestamp: ts,
		})
	}
	return buf
}

func trackedDiskDevices(disks []collector.DiskSnapshot) map[string]struct{} {
	out := make(map[string]struct{}, len(disks)*2)
	for _, d := range disks {
		if d.Device == "" {
			continue
		}
		out[d.Device] = struct{}{}
		if parent := collector.ParentBlockDevice(d.Device); parent != "" {
			out[parent] = struct{}{}
		}
	}
	return out
}

func isTrackedDiskDevice(device string, tracked map[string]struct{}) bool {
	if len(tracked) == 0 {
		return false
	}
	if _, ok := tracked[device]; ok {
		return true
	}
	if parent := collector.ParentBlockDevice(device); parent != "" {
		_, ok := tracked[parent]
		return ok
	}
	return false
}

func flushReadings(db *store.DB, buf []store.Reading) {
	if len(buf) == 0 {
		return
	}
	if err := db.WriteReadings(buf); err != nil {
		log.Printf("write readings error: %v", err)
	}
}

func flushContainerReadings(db *store.DB, buf []store.ContainerReading) {
	if len(buf) == 0 {
		return
	}
	if err := db.WriteContainerReadings(buf); err != nil {
		log.Printf("write container readings error: %v", err)
	}
}

func flushServiceCheckReadings(db *store.DB, buf []store.ServiceCheckReading) {
	if len(buf) == 0 {
		return
	}
	if err := db.WriteServiceCheckReadings(buf); err != nil {
		log.Printf("write service check readings error: %v", err)
	}
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
