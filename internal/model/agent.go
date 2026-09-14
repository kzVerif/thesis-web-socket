package model

import (
	"fmt"
	"math"
	"time"
)

const (
	StatusOnline  = "ONLINE"
	StatusOffline = "OFFLINE"
)

type OSInfo struct {
	Name    string `json:"name"`
	Edition string `json:"edition"`
	Version string `json:"version"`
}

type AgentInfo struct {
	ID         string `json:"id"`
	RoomID     string `json:"room_id,omitempty"`
	Hostname   string `json:"hostname"`
	OSInfo     OSInfo `json:"os_info"`
	IPAddress  string `json:"ip_address"`
	MACAddress string `json:"mac_address"`
}

type PerformanceSample struct {
	CPUUsage    float64 `json:"cpu_usage"`
	RAMTotalGB  float64 `json:"ram_total_gb"`
	RAMUsedGB   float64 `json:"ram_used_gb"`
	RAMUsage    float64 `json:"ram_usage"`
	DiskTotalGB float64 `json:"disk_total_gb"`
	DiskUsedGB  float64 `json:"disk_used_gb"`
	DiskFreeGB  float64 `json:"disk_free_gb"`
	DiskUsage   float64 `json:"disk_usage"`
}

type PerformanceEvent struct {
	Type       string            `json:"type"`
	AgentID    string            `json:"agent_id"`
	Data       PerformanceSample `json:"data"`
	ReceivedAt time.Time         `json:"received_at"`
}

type ProcessInfo struct {
	PID  int64  `json:"pid"`
	Name string `json:"name"`
}

type ProcessEvent struct {
	Type       string        `json:"type"`
	AgentID    string        `json:"agent_id"`
	Data       []ProcessInfo `json:"data"`
	ReceivedAt time.Time     `json:"received_at"`
}

type ProcessKillResult struct {
	Type    string `json:"type"`
	Action  string `json:"action"`
	AgentID string `json:"agent_id,omitempty"`
	PID     int64  `json:"pid"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func ValidateProcesses(processes []ProcessInfo) error {
	if len(processes) > 100_000 {
		return fmt.Errorf("process list exceeds 100000 entries")
	}
	for index, process := range processes {
		if process.PID < 0 {
			return fmt.Errorf("process[%d].pid must not be negative", index)
		}
		if process.Name == "" {
			return fmt.Errorf("process[%d].name is required", index)
		}
	}
	return nil
}

func (sample PerformanceSample) Validate() error {
	percentages := map[string]float64{
		"cpu_usage":  sample.CPUUsage,
		"ram_usage":  sample.RAMUsage,
		"disk_usage": sample.DiskUsage,
	}
	for name, value := range percentages {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
			return fmt.Errorf("%s must be between 0 and 100", name)
		}
	}
	capacities := map[string]float64{
		"ram_total_gb":  sample.RAMTotalGB,
		"ram_used_gb":   sample.RAMUsedGB,
		"disk_total_gb": sample.DiskTotalGB,
		"disk_used_gb":  sample.DiskUsedGB,
		"disk_free_gb":  sample.DiskFreeGB,
	}
	for name, value := range capacities {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return fmt.Errorf("%s must not be negative", name)
		}
	}
	return nil
}
