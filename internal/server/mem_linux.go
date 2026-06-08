//go:build linux

package server

import (
	"bufio"
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
	// /proc/meminfo — reflects container limit under Proxmox LXC when cgroup reports "max"
	if v := procMemTotal(); v > 0 {
		return v
	}
	// fallback: host RAM via sysinfo
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return 0
	}
	return uint64(info.Totalram) * uint64(info.Unit)
}

func procMemTotal() uint64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		// format: "MemTotal:        2097152 kB"
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[2] != "kB" {
			return 0
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return kb * 1024
	}
	return 0
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
