//go:build !linux

package metrics

import "rockchip-node/models"

func ReadDisks() []models.DiskStats {
	return []models.DiskStats{}
}
