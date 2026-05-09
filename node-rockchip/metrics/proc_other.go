//go:build !linux

package metrics

import "rockchip-node/models"

func GetTopProcesses() ([]models.Process, []models.Process, []models.Process, models.Process) {
	return []models.Process{}, []models.Process{}, []models.Process{}, models.Process{}
}

func GetProcessInfo() ([]models.Process, models.Process) {
	return []models.Process{}, models.Process{}
}
