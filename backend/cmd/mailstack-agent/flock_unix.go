//go:build unix

package main

import (
	"os"
	"syscall"
)

// flockRemoveStaleMasterPID removes /queue/pid/master.pid only when no other
// process holds an exclusive lock on it, i.e. postfix is not running. Mirrors
// the legacy launcher's "flock -n /queue/pid/master.pid rm /queue/pid/master.pid".
func flockRemoveStaleMasterPID() {
	f, err := os.OpenFile("/queue/pid/master.pid", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return // locked by a running postfix; keep the file
	}
	_ = os.Remove("/queue/pid/master.pid")
}

