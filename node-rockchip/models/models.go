package models

import "time"

type ThermalZone struct {
	Name  string  `json:"name"`
	TempC float64 `json:"temp_c"`
}

type CPUStats struct {
	UsagePercent float64 `json:"usage_percent"`
	CoreCount    int     `json:"core_count"`
}

type GPUStats struct {
	GPULoad  float64 `json:"gpu_load"`
	NPULoad  float64 `json:"npu_load"`
	VEncLoad float64 `json:"venc_load"`
	VDecLoad float64 `json:"vdec_load"`
	DMCLoad  float64 `json:"dmc_load"`
}

type DiskStats struct {
	Path       string  `json:"path"`
	DeviceName string  `json:"device_name"`
	Label      string  `json:"label"`
	TotalGB    float64 `json:"total_gb"`
	UsedGB     float64 `json:"used_gb"`
	FreeGB     float64 `json:"free_gb"`
	UsedPct    float64 `json:"used_pct"`
	TempC      float64 `json:"temp_c"`
	ReadMBps   float64 `json:"read_mbps"`
	WriteMBps  float64 `json:"write_mbps"`
}

type MemStats struct {
	TotalMB     uint64  `json:"total_mb"`
	UsedMB      uint64  `json:"used_mb"`
	FreeMB      uint64  `json:"free_mb"`
	CachedMB    uint64  `json:"cached_mb"`
	AvailableMB uint64  `json:"available_mb"`
	UsedPct     float64 `json:"used_pct"`
}

type Process struct {
	PID       int     `json:"pid"`
	Name      string  `json:"name"`
	CPU       float64 `json:"cpu"`
	MemMB     float64 `json:"mem_mb"`
	ReadMBps  float64 `json:"read_mbps"`
	WriteMBps float64 `json:"write_mbps"`
}

type Metrics struct {
	Version   string        `json:"version"`
	Timestamp string        `json:"timestamp"`
	Thermals  []ThermalZone `json:"thermals"`
	CPU       CPUStats      `json:"cpu"`
	GPU       GPUStats      `json:"gpu"`
	Disks     []DiskStats   `json:"disks"`
	Memory    MemStats      `json:"memory"`
	TopCPU    []Process     `json:"top_cpu"`
	TopMem    []Process     `json:"top_mem"`
	TopIO     []Process     `json:"top_io"`
	Self      Process       `json:"self"`
	Bot       Process       `json:"bot"`
	Sniffer   Process       `json:"sniffer"`
}

type IODeviceStats struct {
	Device   string  `json:"device"`
	ReadS    float64 `json:"r_s"`
	WriteS   float64 `json:"w_s"`
	AquSz    float64 `json:"aqu_sz"`
	RAwait   float64 `json:"r_await"`
	WAwait   float64 `json:"w_await"`
	UtilPct  float64 `json:"util_pct"`
}

type IOProcess struct {
	PID     string `json:"pid"`
	User    string `json:"user"`
	Read    string `json:"read"`
	Write   string `json:"write"`
	IODelay string `json:"io_delay"`
	Command string `json:"command"`
}

type IOHealth struct {
	Devices []IODeviceStats `json:"devices"`
	TopIO   []IOProcess     `json:"top_io"`
}

func GetTimestamp() string {
	return time.Now().Format(time.RFC3339)
}
