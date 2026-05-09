package metrics

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"rockchip-node/models"
)

// ─── Temperatures ─────────────────────────────────────────────────────────────

var knownZoneNames = map[string]string{
	"thermal_zone0": "SoC (Geral)",
	"thermal_zone1": "CPU",
	"thermal_zone2": "GPU",
	"thermal_zone3": "NPU",
	"thermal_zone4": "Board",
}

func ReadThermals() []models.ThermalZone {
	var zones []models.ThermalZone
	basePath := "/sys/class/thermal"
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return zones
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "thermal_zone") {
			continue
		}
		typePath := filepath.Join(basePath, name, "type")
		tempPath := filepath.Join(basePath, name, "temp")

		zoneName := knownZoneNames[name]
		if rawType, err := os.ReadFile(typePath); err == nil {
			t := strings.TrimSpace(string(rawType))
			if zoneName == "" {
				zoneName = t
			} else {
				zoneName = fmt.Sprintf("%s (%s)", zoneName, t)
			}
		} else if zoneName == "" {
			zoneName = name
		}

		rawTemp, err := os.ReadFile(tempPath)
		if err != nil {
			continue
		}
		milliC, err := strconv.ParseInt(strings.TrimSpace(string(rawTemp)), 10, 64)
		if err != nil {
			continue
		}
		tempC := float64(milliC) / 1000.0
		zones = append(zones, models.ThermalZone{
			Name:  zoneName,
			TempC: math.Round(tempC*10) / 10,
		})
	}
	return zones
}

// ─── GPU / NPU / VPU / DMC ────────────────────────────────────────────────────

type devKind string

const (
	devUnknown devKind = ""
	devGPU     devKind = "gpu"
	devNPU     devKind = "npu"
	devVEnc    devKind = "venc"
	devVDec    devKind = "vdec"
	devDMC     devKind = "dmc"
)

func ReadGPU() models.GPUStats {
	var stats models.GPUStats

	devfreqPath := "/sys/class/devfreq"
	entries, err := os.ReadDir(devfreqPath)
	if err != nil {
		return stats
	}

	for _, entry := range entries {
		entryName := entry.Name()
		base := filepath.Join(devfreqPath, entryName)

		kind, _ := detectDevfreqKind(base, entryName)
		if kind == devUnknown {
			continue
		}

		loadPath := filepath.Join(base, "load")
		raw, err := os.ReadFile(loadPath)
		if err != nil {
			continue
		}

		rawStr := strings.TrimSpace(string(raw))
		val, ok := parseDevfreqLoad(rawStr)
		if !ok {
			continue
		}

		switch kind {
		case devGPU:
			stats.GPULoad = val
		case devNPU:
			stats.NPULoad = val
		case devVEnc:
			stats.VEncLoad = val
		case devVDec:
			stats.VDecLoad = val
		case devDMC:
			stats.DMCLoad = val
		}
	}

	return stats
}

func detectDevfreqKind(basePath, entryName string) (devKind, string) {
	candidates := []string{
		filepath.Join(basePath, "name"),
		filepath.Join(basePath, "device", "of_node", "name"),
		filepath.Join(basePath, "device", "of_node", "full_name"),
	}

	names := []string{entryName}

	for _, p := range candidates {
		if raw, err := os.ReadFile(p); err == nil {
			s := strings.TrimSpace(string(raw))
			if s != "" {
				names = append(names, s)
			}
		}
	}

	joined := strings.ToLower(strings.Join(names, " | "))

	switch {
	case strings.Contains(joined, "mali") || strings.Contains(joined, "gpu"):
		return devGPU, joined
	case strings.Contains(joined, "rknpu") || strings.Contains(joined, "npu"):
		return devNPU, joined
	case strings.Contains(joined, "rkvenc") || strings.Contains(joined, "venc"):
		return devVEnc, joined
	case strings.Contains(joined, "rkvdec") || strings.Contains(joined, "vdec"):
		return devVDec, joined
	case strings.Contains(joined, "dmc"):
		return devDMC, joined
	default:
		return devUnknown, joined
	}
}

func parseDevfreqLoad(raw string) (float64, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false
	}

	if idx := strings.Index(s, "@"); idx != -1 {
		left := strings.TrimSpace(s[:idx])
		if v, ok := parseLeadingNumber(left); ok {
			return clampPercent(v), true
		}
	}

	if v, ok := parseLeadingNumber(s); ok {
		return clampPercent(v), true
	}

	var buf strings.Builder
	started := false
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' {
			buf.WriteRune(r)
			started = true
			continue
		}
		if started {
			break
		}
	}

	if buf.Len() == 0 {
		return 0, false
	}

	v, err := strconv.ParseFloat(buf.String(), 64)
	if err != nil {
		return 0, false
	}

	return clampPercent(v), true
}

func parseLeadingNumber(s string) (float64, bool) {
	s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func clampPercent(v float64) float64 {
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return math.Round(v*10) / 10
}

// ─── CPU Stats ────────────────────────────────────────────────────────────────

type cpuSnap struct {
	user, nice, system, idle, iowait, irq, softirq uint64
}

func snapCPU() (cpuSnap, int) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuSnap{}, 0
	}
	defer f.Close()

	var s cpuSnap
	cores := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) >= 8 {
				s.user, _ = parseU(fields[1])
				s.nice, _ = parseU(fields[2])
				s.system, _ = parseU(fields[3])
				s.idle, _ = parseU(fields[4])
				s.iowait, _ = parseU(fields[5])
				s.irq, _ = parseU(fields[6])
				s.softirq, _ = parseU(fields[7])
			}
		} else if strings.HasPrefix(line, "cpu") {
			cores++
		}
	}
	return s, cores
}

func CPUUsage() (float64, int) {
	t1, cores := snapCPU()
	time.Sleep(200 * time.Millisecond) // Snapshot reduzido para resposta rápida
	t2, _ := snapCPU()

	idle1 := t1.idle + t1.iowait
	idle2 := t2.idle + t2.iowait
	total1 := t1.user + t1.nice + t1.system + idle1 + t1.irq + t1.softirq
	total2 := t2.user + t2.nice + t2.system + idle2 + t2.irq + t2.softirq

	diffTotal := float64(total2 - total1)
	diffIdle := float64(idle2 - idle1)
	if diffTotal == 0 {
		return 0, cores
	}
	usage := 100.0 * (diffTotal - diffIdle) / diffTotal
	return math.Round(usage*10) / 10, cores
}

func parseU(s string) (uint64, error) {
	return strconv.ParseUint(strings.TrimSpace(s), 10, 64)
}

// ─── Memory Stats ─────────────────────────────────────────────────────────────

func ReadMem() models.MemStats {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return models.MemStats{}
	}
	defer f.Close()

	data := map[string]uint64{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.Fields(sc.Text())
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		val, _ := parseU(parts[1])
		data[key] = val
	}

	totalKB := data["MemTotal"]
	freeKB := data["MemFree"]
	availableKB := data["MemAvailable"]
	cachedKB := data["Cached"] + data["Buffers"] + data["SReclaimable"]
	usedKB := totalKB - freeKB - cachedKB

	var usedPct float64
	if totalKB > 0 {
		usedPct = math.Round(float64(usedKB)/float64(totalKB)*1000) / 10
	}
	return models.MemStats{
		TotalMB:     totalKB / 1024,
		UsedMB:      usedKB / 1024,
		FreeMB:      freeKB / 1024,
		CachedMB:    cachedKB / 1024,
		AvailableMB: availableKB / 1024,
		UsedPct:     usedPct,
	}
}
