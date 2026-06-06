//go:build !windows && !linux

package server

func totalPhysicalBytes() uint64 { return 0 }
