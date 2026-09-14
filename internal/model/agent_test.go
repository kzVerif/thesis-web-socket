package model

import "testing"

func TestPerformanceSampleValidation(t *testing.T) {
	valid := PerformanceSample{CPUUsage: 18.42, RAMTotalGB: 15.87, RAMUsedGB: 8.31, RAMUsage: 52.36, DiskTotalGB: 475.82, DiskUsedGB: 201.54, DiskFreeGB: 274.28, DiskUsage: 42.36}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid sample rejected: %v", err)
	}
	invalid := valid
	invalid.CPUUsage = 101
	if err := invalid.Validate(); err == nil {
		t.Fatal("out-of-range CPU usage should be rejected")
	}
}

func TestProcessValidation(t *testing.T) {
	if err := ValidateProcesses([]ProcessInfo{{PID: 1324, Name: "systemd"}, {PID: 8120, Name: "node"}}); err != nil {
		t.Fatalf("valid process list rejected: %v", err)
	}
	if err := ValidateProcesses([]ProcessInfo{{PID: -1, Name: "bad"}}); err == nil {
		t.Fatal("negative PID should be rejected")
	}
	if err := ValidateProcesses([]ProcessInfo{{PID: 1}}); err == nil {
		t.Fatal("empty process name should be rejected")
	}
}
