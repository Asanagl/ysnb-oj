//go:build !linux

package sysload

// Snapshot is one point-in-time host load reading.
type Snapshot struct {
	Load1      float64 `json:"load1"`
	MemUsedMB  float64 `json:"mem_used_mb"`
	MemTotalMB float64 `json:"mem_total_mb"`
}

// Read returns zeros off-Linux; dashboards treat zeros as "not reported".
func Read() Snapshot { return Snapshot{} }
