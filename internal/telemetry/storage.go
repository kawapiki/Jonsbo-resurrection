package telemetry

import (
	"fmt"
	"strings"
	"time"
)

// Disk reports total and free bytes available to the calling user, respecting
// filesystem quotas. It does not mix volume-wide free space with a user quota.
// A nil UsagePercent indicates an unavailable reading, including invalid totals.
type Disk struct {
	Path         string   `json:"path"`
	TotalBytes   uint64   `json:"total_bytes"`
	FreeBytes    uint64   `json:"free_bytes"`
	UsagePercent *float64 `json:"usage_percent"`
}

func DiskFrom(path string, total, free uint64) Disk {
	if total == 0 || free > total {
		return Disk{Path: path}
	}
	p := 100 * float64(total-free) / float64(total)
	return Disk{Path: path, TotalBytes: total, FreeBytes: free, UsagePercent: &p}
}

// storageSampler is owned by the serial Collector sampling loop.
type storageSampler struct {
	systemDrive   string
	logicalDrives func() (uint32, error)
	driveType     func(string) uint32
	diskSpace     func(string) (uint64, uint64, error)
	sampled       bool
	last          time.Time
	disks         []Disk
	warnings      []string
}

func (s *storageSampler) sample(now time.Time) ([]Disk, []string) {
	if !s.sampled || now.Sub(s.last) >= 10*time.Second || now.Before(s.last) {
		s.sampled = true
		s.last = now
		s.disks = make([]Disk, 0)
		s.warnings = nil
		mask, err := s.logicalDrives()
		if err != nil {
			s.warnings = append(s.warnings, fmt.Sprintf("Storage enumeration: %v", err))
		} else {
			for i := 0; i < 26; i++ {
				if mask&(uint32(1)<<i) == 0 {
					continue
				}
				path := fmt.Sprintf("%c:\\", 'A'+i)
				if s.driveType(path) != 3 {
					continue
				} // DRIVE_FIXED only.
				total, free, err := s.diskSpace(path)
				disk := Disk{Path: path}
				if err != nil {
					s.warnings = append(s.warnings, fmt.Sprintf("Storage %s: %v", path, err))
				} else {
					disk = DiskFrom(path, total, free)
					if disk.UsagePercent == nil {
						s.warnings = append(s.warnings, fmt.Sprintf("Storage %s: invalid capacity reading", path))
					}
				}
				s.disks = append(s.disks, disk)
			}
			systemRoot := strings.TrimRight(s.systemDrive, "\\/") + "\\"
			for i := range s.disks {
				if strings.EqualFold(s.disks[i].Path, systemRoot) {
					disk := s.disks[i]
					copy(s.disks[1:i+1], s.disks[:i])
					s.disks[0] = disk
					break
				}
			}
		}
	}
	// Returned snapshots cannot mutate the cached slices or percentage pointers.
	disks := make([]Disk, len(s.disks))
	copy(disks, s.disks)
	for i := range disks {
		if disks[i].UsagePercent != nil {
			p := *disks[i].UsagePercent
			disks[i].UsagePercent = &p
		}
	}
	return disks, append([]string(nil), s.warnings...)
}
