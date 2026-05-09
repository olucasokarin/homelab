//go:build linux

package metrics

import (
	"bufio"
	"bytes"
	"os/exec"
	"rockchip-node/models"
	"strconv"
	"strings"
)

func GetIOHealth() models.IOHealth {
	var health models.IOHealth

	// 1. iostat -xy 1 1
	// Output format usually has a header, then empty line, then average-cpu, then empty line, then Device: r/s w/s rkB/s wkB/s rrqm/s wrqm/s %rrqm %wrqm r_await w_await aqu-sz rareq-sz wareq-sz svctm %util
	cmdIostat := exec.Command("iostat", "-xy", "1", "1")
	outIostat, err := cmdIostat.Output()
	if err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(outIostat))
		deviceSection := false
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "Device") {
				deviceSection = true
				continue
			}
			if deviceSection {
				fields := strings.Fields(line)
				// The number of fields can vary slightly depending on sysstat version, 
				// but usually Device is 0, r/s is 1, w/s is 2, %util is the last one.
				// We'll try to extract common ones based on typical indices if len >= 14
				// For extended iostat (sysstat 12+): Device r/s w/s rkB/s wkB/s rrqm/s wrqm/s %rrqm %wrqm r_await w_await aqu-sz rareq-sz wareq-sz svctm %util (16 fields)
				// sysstat 11: Device r/s w/s rkB/s wkB/s rrqm/s wrqm/s %rrqm %wrqm r_await w_await aqu-sz rareq-sz wareq-sz svctm %util
				// Let's rely on standard positions for sysstat 11/12
				
				if len(fields) >= 14 {
					device := fields[0]
					// Filter out loop devices
					if strings.HasPrefix(device, "loop") {
						continue
					}

					rs, _ := strconv.ParseFloat(strings.ReplaceAll(fields[1], ",", "."), 64)
					ws, _ := strconv.ParseFloat(strings.ReplaceAll(fields[2], ",", "."), 64)

					// Parse from the end to be safer with different sysstat versions
					// Last is %util
					utilPct, _ := strconv.ParseFloat(strings.ReplaceAll(fields[len(fields)-1], ",", "."), 64)
					
					// Typically: r_await, w_await, aqu-sz are somewhere before svctm and %util
					// For sysstat 12: %util is -1, svctm is -2, wareq-sz is -3, rareq-sz is -4, aqu-sz is -5, w_await is -6, r_await is -7
					var aquSz, rAwait, wAwait float64
					if len(fields) >= 16 {
						aquSz, _ = strconv.ParseFloat(strings.ReplaceAll(fields[len(fields)-5], ",", "."), 64)
						wAwait, _ = strconv.ParseFloat(strings.ReplaceAll(fields[len(fields)-6], ",", "."), 64)
						rAwait, _ = strconv.ParseFloat(strings.ReplaceAll(fields[len(fields)-7], ",", "."), 64)
					} else if len(fields) >= 14 {
						// Older format might be slightly different, let's just grab what we can
						aquSz, _ = strconv.ParseFloat(strings.ReplaceAll(fields[len(fields)-4], ",", "."), 64)
						wAwait, _ = strconv.ParseFloat(strings.ReplaceAll(fields[len(fields)-5], ",", "."), 64)
						rAwait, _ = strconv.ParseFloat(strings.ReplaceAll(fields[len(fields)-6], ",", "."), 64)
					}

					health.Devices = append(health.Devices, models.IODeviceStats{
						Device:  device,
						ReadS:   rs,
						WriteS:  ws,
						AquSz:   aquSz,
						RAwait:  rAwait,
						WAwait:  wAwait,
						UtilPct: utilPct,
					})
				}
			}
		}
	}

	// 2. sudo iotop -b -n 1 -oP
	// Output: 
	// Total DISK READ :       0.00 B/s | Total DISK WRITE :       0.00 B/s
	// Actual DISK READ:       0.00 B/s | Actual DISK WRITE:       0.00 B/s
	//   PID  PRIO  USER     DISK READ  DISK WRITE  SWAPIN     IO>    COMMAND
	//  1234 be/4 root        0.00 B/s    0.00 B/s  0.00 %  0.00 %  [jbd2/nvme0n1-8]
	cmdIotop := exec.Command("sudo", "iotop", "-b", "-n", "1", "-o", "-P")
	outIotop, err := cmdIotop.Output()
	if err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(outIotop))
		started := false
		count := 0
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			// Skip headers
			if strings.Contains(line, "Total DISK READ") || strings.Contains(line, "Actual DISK READ") {
				continue
			}
			if strings.Contains(line, "PID") && strings.Contains(line, "USER") && strings.Contains(line, "COMMAND") {
				started = true
				continue
			}

			if started {
				fields := strings.Fields(line)
				// Expected fields: PID (0), PRIO (1), USER (2), ReadVal (3), ReadUnit (4), WriteVal (5), WriteUnit (6)
				if len(fields) >= 8 {
					pid := fields[0]
					user := fields[2]
					read := fields[3] + " " + fields[4]
					write := fields[5] + " " + fields[6]
					
					idx := 7
					ioDelay := "N/A"

					// Skip SWAPIN: could be "?unavailable?" or "X.XX %"
					if idx < len(fields) {
						if fields[idx] == "?unavailable?" {
							idx++
						} else if idx+1 < len(fields) && fields[idx+1] == "%" {
							idx += 2
						}
					}

					// Read or Skip IO>: could be "?unavailable?" or "X.XX %"
					if idx < len(fields) {
						if fields[idx] == "?unavailable?" {
							idx++
						} else if idx+1 < len(fields) && fields[idx+1] == "%" {
							ioDelay = fields[idx] + " %"
							idx += 2
						}
					}

					// Any remaining "?unavailable?" prefix
					for idx < len(fields) && fields[idx] == "?unavailable?" {
						idx++
					}

					command := ""
					if idx < len(fields) {
						command = strings.Join(fields[idx:], " ")
					}

					health.TopIO = append(health.TopIO, models.IOProcess{
						PID:     pid,
						User:    user,
						Read:    read,
						Write:   write,
						IODelay: ioDelay,
						Command: command,
					})
					count++
					if count >= 10 { // Limit to top 10
						break
					}
				}
			}
		}
	}

	return health
}
