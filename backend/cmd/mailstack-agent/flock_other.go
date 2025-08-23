//go:build !unix

package main

// flockRemoveStaleMasterPID is a no-op outside Linux containers.
func flockRemoveStaleMasterPID() {}
