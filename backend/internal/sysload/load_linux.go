//go:build linux

package sysload

import (
	"os"
	"strconv"
	"strings"
)

// Snapshot is one point-in-time host load reading.
type Snapshot struct {
	Load1      float64 `json:"load1"`
	MemUsedMB  float64 `json:"mem_used_mb"`
	MemTotalMB float64 `json:"mem_total_mb"`
}

// Read collects load-average and memory usage from /proc. Errors degrade to
// a zero snapshot — dashboards treat zeros as "not reported".
func Read() Snapshot {
	var s Snapshot
	if raw, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(raw))
		if len(fields) > 0 {
			s.Load1, _ = strconv.ParseFloat(fields[0], 64)
		}
	}
	if raw, err := os.ReadFile("/proc/meminfo"); err == nil {
		var total, avail float64
		for _, line := range strings.Split(string(raw), "\n") {
			switch {
			case strings.HasPrefix(line, "MemTotal:"):
				total = meminfoKB(line)
			case strings.HasPrefix(line, "MemAvailable:"):
				avail = meminfoKB(line)
			}
		}
		if total > 0 {
			s.MemTotalMB = total / 1024
			s.MemUsedMB = (total - avail) / 1024
		}
	}
	return s
}

func meminfoKB(line string) float64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	v, _ := strconv.ParseFloat(fields[1], 64)
	return v
}
