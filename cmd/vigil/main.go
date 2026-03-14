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
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
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

	// Build notifier chain from config.
	var notifiers notify.Multi
	if u := cfg.Notifications.DiscordWebhook; u != "" {
		if err := notify.ValidateURL(u); err != nil {
			log.Fatalf("discord_webhook: %v", err)
		}
		notifiers = append(notifiers, notify.Discord{WebhookURL: u})
		log.Printf("discord notifications enabled")
	}
	if u := cfg.Notifications.WebhookURL; u != "" {
		if err := notify.ValidateURL(u); err != nil {
			log.Fatalf("webhook_url: %v", err)
		}
		notifiers = append(notifiers, notify.Webhook{URL: u})
		log.Printf("webhook notifications enabled")
	}
	var notifier notify.Notifier
	var mute *notify.Mute
	if len(notifiers) > 0 {
		mute = &notify.Mute{}
		var windows []notify.QuietWindow
		if qh := cfg.Notifications.QuietHours; len(qh) > 0 {
			var err error
			windows, err = notify.ParseQuietHours(qh)
			if err != nil {
				log.Fatalf("quiet_hours: %v", err)
			}
			log.Printf("quiet hours enabled: %v", qh)
		}
		notifier = notify.Quiet{Inner: notifiers, Windows: windows, Mute: mute}
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
		runHeadless(ctx, db, cfg, snapshots, alertEngine, notifier, mountHandler, serviceHandler, svcChecker)
		return
	}

	// Check if stdout is a TTY; fall back to headless if not.
	if !isTerminal(os.Stdout) {
		log.Println("stdout is not a TTY, running in headless mode")
		runHeadless(ctx, db, cfg, snapshots, alertEngine, notifier, mountHandler, serviceHandler, svcChecker)
		return
	}

	runTUI(ctx, cancel, db, cfg, snapshots, alertEngine, restoredAlerts, notifier, mute, mountHandler, serviceHandler, svcChecker)
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
	svcChecker *checker.ServiceChecker,
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
		runLoop(ctx, db, snapshots, eng, n, mountHandler, serviceHandler, svcChecker, loopConfig{
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
	svcChecker *checker.ServiceChecker,
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

	runLoop(ctx, db, snapshots, eng, n, mountHandler, serviceHandler, svcChecker, loopConfig{
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
	svcChecker *checker.ServiceChecker,
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
				return
			}

			values := snapshotToValues(snap)
			persistAlerts(ctx, db, eng, values, lc.onAlert, n)
			handleThrottleAlert(ctx, snap, &throttleFiring, db, lc.onAlert, n)
			handleMountAlerts(ctx, snap, mountHandler, db, lc.onAlert, n)

			if svcChecker != nil {
				ct := svcChecker.CycleTime()
				if ct.After(lastCycleTime) {
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
	v := map[string]float64{
		metric.MemPercent:  snap.Memory.Percent,
		metric.SwapPercent: snap.Memory.SwapPercent,
		metric.Load1:       snap.Load.Load1,
		metric.Load5:       snap.Load.Load5,
		metric.Load15:      snap.Load.Load15,
	}
	if snap.CPU.Ready {
		v[metric.CPUPercent] = snap.CPU.PercentTotal
		v[metric.CPUIowait] = snap.CPU.IOWaitPercent
	}
	for _, d := range snap.Disks {
		v[metric.PrefixDiskPercent+d.MountPoint] = d.Percent
	}
	for _, n := range snap.Network {
		v[metric.PrefixNetDrops+n.Interface] = n.DropRate
		v[metric.PrefixNetErrors+n.Interface] = n.ErrRate
	}
	for _, t := range snap.Temperature {
		v[t.SensorKey] = t.Celsius
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
	if snap.CPU.Ready {
		buf = append(buf,
			store.Reading{Metric: metric.CPUPercent, Value: snap.CPU.PercentTotal, Timestamp: ts},
			store.Reading{Metric: metric.CPUUser, Value: snap.CPU.UserPercent, Timestamp: ts},
			store.Reading{Metric: metric.CPUSystem, Value: snap.CPU.SystemPercent, Timestamp: ts},
			store.Reading{Metric: metric.CPUIowait, Value: snap.CPU.IOWaitPercent, Timestamp: ts},
			store.Reading{Metric: metric.CPUIdle, Value: snap.CPU.IdlePercent, Timestamp: ts},
		)
	}
	buf = append(buf,
		store.Reading{Metric: metric.MemPercent, Value: snap.Memory.Percent, Timestamp: ts},
		store.Reading{Metric: metric.SwapPercent, Value: snap.Memory.SwapPercent, Timestamp: ts},
		store.Reading{Metric: metric.Load1, Value: snap.Load.Load1, Timestamp: ts},
		store.Reading{Metric: metric.Load5, Value: snap.Load.Load5, Timestamp: ts},
		store.Reading{Metric: metric.Load15, Value: snap.Load.Load15, Timestamp: ts},
	)
	for _, d := range snap.Disks {
		buf = append(buf, store.Reading{
			Metric: metric.PrefixDiskPercent + d.MountPoint, Value: d.Percent, Timestamp: ts,
		})
	}
	for _, d := range snap.DiskIO {
		buf = append(buf,
			store.Reading{Metric: metric.PrefixDiskRead + d.Device, Value: d.ReadRate, Timestamp: ts},
			store.Reading{Metric: metric.PrefixDiskWrite + d.Device, Value: d.WriteRate, Timestamp: ts},
		)
	}
	for _, n := range snap.Network {
		buf = append(buf,
			store.Reading{Metric: metric.PrefixNetSend + n.Interface, Value: n.SendRate, Timestamp: ts},
			store.Reading{Metric: metric.PrefixNetRecv + n.Interface, Value: n.RecvRate, Timestamp: ts},
			store.Reading{Metric: metric.PrefixNetDrops + n.Interface, Value: n.DropRate, Timestamp: ts},
			store.Reading{Metric: metric.PrefixNetErrors + n.Interface, Value: n.ErrRate, Timestamp: ts},
		)
	}
	for _, t := range snap.Temperature {
		buf = append(buf, store.Reading{
			Metric: metric.PrefixTemp + t.SensorKey, Value: t.Celsius, Timestamp: ts,
		})
	}
	if snap.Throttle.Available {
		buf = append(buf, store.Reading{
			Metric: metric.ThrottleRaw, Value: float64(snap.Throttle.Raw), Timestamp: ts,
		})
	}
	return buf
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
