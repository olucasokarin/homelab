//go:build !linux

package metrics

import "rockchip-node/models"

func GetIOHealth() models.IOHealth {
	// Dummy data for Windows testing
	return models.IOHealth{
		Devices: []models.IODeviceStats{
			{Device: "C:", ReadS: 10.5, WriteS: 5.2, AquSz: 0.1, RAwait: 2.0, WAwait: 3.5, UtilPct: 15.0},
		},
		TopIO: []models.IOProcess{
			{PID: "1024", User: "System", Read: "1.50 M/s", Write: "0.00 B/s", IODelay: "0.50 %", Command: "dummy.exe"},
		},
	}
}
