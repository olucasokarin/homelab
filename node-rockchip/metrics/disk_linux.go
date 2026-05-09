//go:build linux

package metrics

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"rockchip-node/models"
)

var mountPaths = []string{"/", "/mnt/storage_blue", "/mnt/storage_wdblue", "/mnt/storage_samsung", "/mnt/storage"}

var diskLabels = map[string]string{
	"/":                    "SSD Principal (NVMe)",
	"/mnt/storage_blue":    "HD Externo (Blue)",
	"/mnt/storage_wdblue":  "HD Externo (WdBlue)",
	"/mnt/storage_samsung": "HD Externo (Samsung)",
	"/mnt/storage":         "Disco Principal (Pool)",
}

// ─── I/O Snapshot ─────────────────────────────────────────────────────────────

type ioSnap struct {
	readSectors  uint64
	writeSectors uint64
	ts           time.Time
}

var (
	ioMu   sync.Mutex
	ioLast = map[string]ioSnap{}
)

func readDiskstats(devName string) (uint64, uint64, bool) {
	f, err := os.Open("/proc/diskstats")
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 14 {
			continue
		}
		if fields[2] != devName {
			continue
		}
		reads, _ := strconv.ParseUint(fields[5], 10, 64)
		writes, _ := strconv.ParseUint(fields[9], 10, 64)
		return reads, writes, true
	}
	return 0, 0, false
}

func getIOSpeed(devName string) (float64, float64) {
	r2, w2, ok := readDiskstats(devName)
	now := time.Now()
	if !ok {
		return 0, 0
	}

	ioMu.Lock()
	prev, hasPrev := ioLast[devName]
	ioLast[devName] = ioSnap{readSectors: r2, writeSectors: w2, ts: now}
	ioMu.Unlock()

	if !hasPrev {
		return 0, 0
	}

	elapsed := now.Sub(prev.ts).Seconds()
	if elapsed <= 0 {
		return 0, 0
	}

	readMB := float64(r2-prev.readSectors) * 512 / 1048576 / elapsed
	writeMB := float64(w2-prev.writeSectors) * 512 / 1048576 / elapsed

	return math.Round(readMB*100) / 100, math.Round(writeMB*100) / 100
}

func toGB(bytes uint64) float64 {
	return math.Round(float64(bytes)/1073741824*100) / 100
}

func ReadDisks() []models.DiskStats {
	seen := map[string]bool{}
	var disks []models.DiskStats

	for _, path := range mountPaths {
		var stat syscall.Statfs_t
		if err := syscall.Statfs(path, &stat); err != nil {
			continue
		}

		total := stat.Blocks * uint64(stat.Bsize)
		free := stat.Bfree * uint64(stat.Bsize)
		used := total - free

		key := fmt.Sprintf("%v", stat.Fsid)
		if seen[key] {
			continue
		}
		seen[key] = true

		var usedPct float64
		if total > 0 {
			usedPct = math.Round(float64(used)/float64(total)*1000) / 10
		}

		label := diskLabels[path]
		if label == "" {
			label = path
		}

		device := getDeviceFromMount(path)
		devBase := device[strings.LastIndex(device, "/")+1:]
		readMBps, writeMBps := getIOSpeed(devBase)

		disks = append(disks, models.DiskStats{
			Path:       path,
			DeviceName: devBase,
			Label:      label,
			TotalGB:    toGB(total),
			UsedGB:     toGB(used),
			FreeGB:     toGB(free),
			UsedPct:    usedPct,
			TempC:      getDiskTemp(device),
			ReadMBps:   readMBps,
			WriteMBps:  writeMBps,
		})
	}

	return disks
}

func getDiskTemp(device string) float64 {
	if device == "" || !strings.HasPrefix(device, "/dev/") {
		return 0
	}

	types := []string{"sat", "nvme", "sntrealtek", "auto", "scsi", "uas", "sat,12", "sat,16"}

	for _, t := range types {
		args := []string{"-a", "-n", "standby"}
		if t != "" {
			args = append(args, "-d", t)
		}
		args = append(args, device)

		out, _ := exec.Command("sudo", append([]string{"smartctl"}, args...)...).CombinedOutput()
		if len(out) == 0 {
			continue
		}

		scanner := bufio.NewScanner(strings.NewReader(string(out)))
		for scanner.Scan() {
			line := scanner.Text()
			lower := strings.ToLower(line)

			// Exclude common threshold/limit lines that cause stuck values
			if strings.Contains(lower, "threshold") || strings.Contains(lower, "limit") ||
				strings.Contains(lower, "alarm") || strings.Contains(lower, "critical") ||
				strings.Contains(lower, "warning") {
				continue
			}

			// Clean line: remove anything in parentheses to avoid Min/Max confusion
			cleanLine := line
			if idx := strings.Index(line, "("); idx >= 0 {
				cleanLine = line[:idx]
			}
			fields := strings.Fields(cleanLine)
			if len(fields) == 0 {
				continue
			}

			// SMART Attributes: ID 194 (Temperature_Celsius) or 190 (Airflow_Temperature_Cel)
			// We check if the FIRST field is exactly the ID to avoid false positives in values
			isSmartTemp := fields[0] == "194" || fields[0] == "190"

			// Direct "Temperature" labels (NVMe, etc.)
			isLabelTemp := strings.Contains(lower, "temperature") || strings.Contains(lower, "temp")

			if isSmartTemp || isLabelTemp {
				// Work backwards to find the first plausible temperature
				for i := len(fields) - 1; i >= 0; i-- {
					f := fields[i]

					// Basic parsing for digits only
					cleanVal := ""
					for _, r := range f {
						if (r >= '0' && r <= '9') || r == '.' {
							cleanVal += string(r)
						} else if len(cleanVal) > 0 {
							break
						}
					}

					if temp, err := strconv.ParseFloat(cleanVal, 64); err == nil {
						if temp > 10 && temp < 95 {
							return temp
						}
					}
				}
			}
		}

		// Fallback regex for non-standard outputs
		re := regexp.MustCompile(`(?i)(Temperature|Temp)\D+(\d{2,3})`)
		matches := re.FindAllStringSubmatch(string(out), -1)
		if len(matches) > 0 {
			last := matches[len(matches)-1]
			if temp, err := strconv.ParseFloat(last[2], 64); err == nil {
				if temp > 10 && temp < 95 {
					return temp
				}
			}
		}
	}

	return getEmmcTemp(device)
}

func getDeviceFromMount(mountPath string) string {
	out, err := exec.Command("df", mountPath).Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) < 2 {
		return ""
	}
	fields := strings.Fields(lines[1])
	if len(fields) == 0 {
		return ""
	}
	device := fields[0]

	if strings.Contains(device, "mmcblk") || strings.Contains(device, "nvme") {
		if idx := strings.LastIndex(device, "p"); idx > 0 {
			suffix := device[idx+1:]
			allDigits := true
			for _, r := range suffix {
				if r < '0' || r > '9' {
					allDigits = false
					break
				}
			}
			if allDigits && len(suffix) > 0 {
				device = device[:idx]
			}
		}
	} else {
		device = strings.TrimRight(device, "0123456789")
	}

	return device
}

func getEmmcTemp(device string) float64 {
	if !strings.Contains(device, "mmcblk") {
		return 0
	}
	baseName := device[strings.LastIndex(device, "/")+1:]
	hwmonPath := fmt.Sprintf("/sys/class/block/%s/device/hwmon", baseName)
	entries, err := os.ReadDir(hwmonPath)
	if err != nil || len(entries) == 0 {
		return 0
	}
	tempFile := fmt.Sprintf("%s/%s/temp1_input", hwmonPath, entries[0].Name())
	raw, err := os.ReadFile(tempFile)
	if err != nil {
		return 0
	}
	milliC, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
	if err != nil {
		return 0
	}
	return math.Round(milliC/100) / 10
}
