//go:build linux

package metrics

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"rockchip-node/models"
)

type procSnap struct {
	utime      uint64
	stime      uint64
	uptime     uint64
	readBytes  uint64
	writeBytes uint64
	timestamp  time.Time
}

var (
	procHistory = make(map[int]procSnap)
	lastUpdate  time.Time
	procMutex   sync.Mutex
)

func GetProcessInfo() ([]models.Process, models.Process) {
	procMutex.Lock()
	defer procMutex.Unlock()

	now := time.Now()
	systemUptime := getSystemUptime()
	
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, models.Process{}
	}

	var allProcs []models.Process
	var self models.Process
	selfPID := os.Getpid()

	newHistory := make(map[int]procSnap)

	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		statPath := filepath.Join("/proc", entry.Name(), "stat")
		content, err := os.ReadFile(statPath)
		if err != nil {
			continue
		}

		fields := strings.Fields(string(content))
		if len(fields) < 22 {
			continue
		}

		name := strings.Trim(fields[1], "()")
		utime, _ := strconv.ParseUint(fields[13], 10, 64)
		stime, _ := strconv.ParseUint(fields[14], 10, 64)
		
		rssPages, _ := strconv.ParseInt(fields[23], 10, 64)
		memMB := float64(rssPages * 4096) / 1024 / 1024

		ioPath := filepath.Join("/proc", entry.Name(), "io")
		ioContent, _ := os.ReadFile(ioPath)
		var readBytes, writeBytes uint64
		if len(ioContent) > 0 {
			for _, line := range strings.Split(string(ioContent), "\n") {
				if strings.HasPrefix(line, "read_bytes:") {
					readBytes, _ = strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "read_bytes:")), 10, 64)
				} else if strings.HasPrefix(line, "write_bytes:") {
					writeBytes, _ = strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "write_bytes:")), 10, 64)
				}
			}
		}

		currentSnap := procSnap{utime: utime, stime: stime, uptime: systemUptime, readBytes: readBytes, writeBytes: writeBytes, timestamp: now}
		newHistory[pid] = currentSnap

		cpuUsage := 0.0
		readMBps := 0.0
		writeMBps := 0.0
		if prev, ok := procHistory[pid]; ok {
			deltaProc := float64((utime + stime) - (prev.utime + prev.stime))
			deltaSys := float64(systemUptime - prev.uptime)
			if deltaSys > 0 {
				cpuUsage = (deltaProc / deltaSys) * 100.0
			}

			dt := now.Sub(prev.timestamp).Seconds()
			if dt > 0 {
				if readBytes >= prev.readBytes {
					readMBps = float64(readBytes-prev.readBytes) / dt / 1024 / 1024
				}
				if writeBytes >= prev.writeBytes {
					writeMBps = float64(writeBytes-prev.writeBytes) / dt / 1024 / 1024
				}
			}
		}

		p := models.Process{
			PID:       pid,
			Name:      name,
			CPU:       roundDecimal(cpuUsage),
			MemMB:     roundDecimal(memMB),
			ReadMBps:  roundDecimal(readMBps),
			WriteMBps: roundDecimal(writeMBps),
		}

		if pid == selfPID {
			self = p
		}
		allProcs = append(allProcs, p)
	}

	procHistory = newHistory
	lastUpdate = now

	return allProcs, self
}

func getSystemUptime() uint64 {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if sc.Scan() {
		fields := strings.Fields(sc.Text())
		var total uint64
		for i := 1; i < len(fields); i++ {
			v, _ := strconv.ParseUint(fields[i], 10, 64)
			total += v
		}
		return total
	}
	return 0
}

func roundDecimal(f float64) float64 {
	return float64(int(f*10)) / 10
}

func GetTopProcesses() (cpu []models.Process, mem []models.Process, io []models.Process, self models.Process) {
	all, selfProc := GetProcessInfo()
	
	memList := make([]models.Process, len(all))
	copy(memList, all)
	
	ioList := make([]models.Process, len(all))
	copy(ioList, all)

	sort.Slice(all, func(i, j int) bool {
		return all[i].CPU > all[j].CPU
	})
	
	sort.Slice(memList, func(i, j int) bool {
		return memList[i].MemMB > memList[j].MemMB
	})

	sort.Slice(ioList, func(i, j int) bool {
		return (ioList[i].ReadMBps + ioList[i].WriteMBps) > (ioList[j].ReadMBps + ioList[j].WriteMBps)
	})

	limitCPU := 10
	if len(all) < 10 { limitCPU = len(all) }
	
	limitMem := 10
	if len(memList) < 10 { limitMem = len(memList) }

	limitIO := 10
	if len(ioList) < 10 { limitIO = len(ioList) }

	return all[:limitCPU], memList[:limitMem], ioList[:limitIO], selfProc
}
