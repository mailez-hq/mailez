//go:build !linux

package admin

// diskCheck is linux-only: other platforms report unknown instead of
// guessing (production containers are linux; dev hosts may not be).
func diskCheck(path string) CheckItem {
	return unknown("disk", "linux only")
}

// memCheck mirrors diskCheck for the memory probe.
func memCheck() CheckItem {
	return unknown("memory", "linux only")
}
