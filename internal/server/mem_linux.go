//go:build linux

package server

import (
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func totalPhysicalBytes() uint64 {
	// cgroup v2
	if v := cgroupMemLimit("/sys/fs/cgroup/memory.max"); v > 0 {
		return v
	}
	// cgroup v1
	if v := cgroupMemLimit("/sys/fs/cgroup/memory/memory.limit_in_bytes"); v > 0 {
		return v
	}
	// fallback: host RAM via sysinfo
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return 0
	}
	return uint64(info.Totalram) * uint64(info.Unit)
}

func cgroupMemLimit(path string) uint64 {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	s := strings.TrimSpace(string(b))
	if s == "max" {
		return 0
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil || v == 0 {
		return 0
	}
	// cgroup v1 uses 9223372036854771712 as "unlimited" sentinel
	const unlimitedV1 = uint64(1<<63 - 4096)
	if v >= unlimitedV1 {
		return 0
	}
	return v
}
