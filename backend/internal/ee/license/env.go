//go:build mailez_ee

package license

import (
	"os"
	"strconv"
	"sync"
)

var (
	envOnce sync.Once
	envMgr  *Manager
)

// FromEnv loads the manager once from the standard environment variables
// (MAILEZ_LICENSE_FILE / MAILEZ_LICENSE / MAILEZ_LICENSE_REQUIRED). A load
// error is fatal: it panics, refusing startup with an unusable license.
func FromEnv() *Manager {
	envOnce.Do(func() {
		required, _ := strconv.ParseBool(os.Getenv("MAILEZ_LICENSE_REQUIRED"))
		m, err := Load(os.Getenv("MAILEZ_LICENSE_FILE"), os.Getenv("MAILEZ_LICENSE"), required)
		if err != nil {
			panic("license: " + err.Error())
		}
		envMgr = m
	})
	return envMgr
}
