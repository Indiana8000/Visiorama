package util

import (
	"context"
	"net"
	"os"
	"strconv"
	"time"
)

// Notify sends a state string to the systemd notify socket (sd_notify(3)
// protocol). It is a no-op when NOTIFY_SOCKET is unset, e.g. when not
// running under systemd (OpenRC, dev, Docker).
func Notify(state string) error {
	sockPath := os.Getenv("NOTIFY_SOCKET")
	if sockPath == "" {
		return nil
	}
	conn, err := net.Dial("unixgram", sockPath)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write([]byte(state))
	return err
}

// watchdogInterval returns the interval systemd expects WATCHDOG=1 pings at
// (half of WatchdogSec), and whether a watchdog is configured at all.
func watchdogInterval() (time.Duration, bool) {
	usec, err := strconv.ParseInt(os.Getenv("WATCHDOG_USEC"), 10, 64)
	if err != nil || usec <= 0 {
		return 0, false
	}
	return time.Duration(usec) * time.Microsecond / 2, true
}

// RunWatchdog signals READY=1 and, if the service unit set WatchdogSec,
// pings WATCHDOG=1 on a ticker until ctx is done. A missed ping tells
// systemd the process is hung (not just crashed) and triggers a restart.
// Safe to call unconditionally; it is a no-op outside systemd.
func RunWatchdog(ctx context.Context) {
	_ = Notify("READY=1")

	interval, ok := watchdogInterval()
	if !ok {
		return
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = Notify("WATCHDOG=1")
			}
		}
	}()
}
