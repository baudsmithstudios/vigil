//go:build demo

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"

	"vigil/internal/alert"
	"vigil/internal/collector"
	"vigil/internal/config"
	"vigil/internal/notify"
	"vigil/internal/store"
)

var demoFlag = flag.Bool("demo", false, "run a scripted demo on synthetic data; never reads config or sends notifications")

func init() {
	demo = func() bool {
		if !*demoFlag {
			return false
		}
		if err := runDemo(); err != nil {
			log.Fatalf("demo: %v", err)
		}
		return true
	}
}

// runDemo plays the scripted Pi through the real alert pipeline and TUI. It
// builds its own settings instead of loading config.toml, and passes no
// notifier, so it can't touch a real database or send a notification.
func runDemo() error {
	db, cleanup, err := openDemoDB()
	if err != nil {
		return err
	}
	defer cleanup()

	cfg := config.Defaults()
	cfg.Theme = "dark"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	snapshots := make(chan collector.Snapshot)
	go playDemo(ctx, snapshots)

	runTUI(ctx, cancel, db, cfg, snapshots, demoAlerting(cfg), nil, nil, &notify.Mute{})
	return nil
}

func demoAlerting(cfg config.Config) alerting {
	return alerting{
		engine:   alert.New(cfg.Alerts),
		mounts:   alert.NewMountHandler(),
		services: alert.NewServiceHandler(cfg.Services.FailuresBeforeAlert),
		throttle: &alert.ThrottleAlertHandler{},
	}
}

// openDemoDB opens a database in a fresh temporary directory; cleanup closes it
// and removes the directory.
func openDemoDB() (db *store.DB, cleanup func(), err error) {
	dir, err := os.MkdirTemp("", "vigil-demo-")
	if err != nil {
		return nil, nil, err
	}
	db, err = store.Open(filepath.Join(dir, "vigil.db"))
	if err != nil {
		os.RemoveAll(dir)
		return nil, nil, err
	}
	return db, func() {
		db.Close()
		os.RemoveAll(dir)
	}, nil
}
