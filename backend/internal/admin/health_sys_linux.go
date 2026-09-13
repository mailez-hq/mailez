//go:build linux

package admin

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// diskCheck reports filesystem usage for the path backing uploads/blobs.
func diskCheck(path string) CheckItem {
	if path == "" {
		path = "/data"
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		// Fall back to the container root when the configured path is not
		// mounted in this namespace (dev runs on a host filesystem).
		if err2 := syscall.Statfs("/", &st); err2 != nil {
			return unknown("disk", path+": "+err.Error())
		}
		path = "/"
	}
	total := uint64(st.Blocks) * uint64(st.Bsize)
	avail := uint64(st.Bavail) * uint64(st.Bsize)
	if total == 0 {
		return unknown("disk", path)
	}
	pct := int((total - avail) * 100 / total)
	detail := path + " " + strconv.Itoa(pct) + "% used"
	switch {
	case pct >= 95:
		return fail("disk", detail)
	case pct >= 85:
		return warn("disk", detail)
	default:
		return ok("disk", detail)
	}
}

// memCheck reports available memory from /proc/meminfo.
func memCheck() CheckItem {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return unknown("memory", "/proc/meminfo unavailable")
	}
	var total, avail int64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		kb := parseInt64(fields[1])
		switch fields[0] {
		case "MemTotal:":
			total = kb
		case "MemAvailable:":
			avail = kb
		}
	}
	if total == 0 {
		return unknown("memory", "no MemTotal")
	}
	pctAvail := int(avail * 100 / total)
	detail := strconv.Itoa(pctAvail) + "% available"
	switch {
	case pctAvail < 5:
		return fail("memory", detail)
	case pctAvail < 10:
		return warn("memory", detail)
	default:
		return ok("memory", detail)
	}
}

func parseInt64(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
