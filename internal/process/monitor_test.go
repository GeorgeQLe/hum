package process

import (
	"testing"
	"time"
)

func TestAppMonitorCircularBuffer(t *testing.T) {
	am := newAppMonitor(ThresholdConfig{})

	// Empty buffer
	if s := am.getLatest(); s != nil {
		t.Fatal("expected nil for empty buffer")
	}
	if h := am.getHistory(10); h != nil {
		t.Fatal("expected nil for empty buffer history")
	}

	// Add samples
	for i := 0; i < 5; i++ {
		am.addSample(ResourceSample{
			Timestamp:  time.Now(),
			CPUPercent: float64(i + 1),
			MemoryRSS:  int64((i + 1) * 1024 * 1024),
		})
	}

	// Latest should be last added
	latest := am.getLatest()
	if latest == nil || latest.CPUPercent != 5.0 {
		t.Fatalf("expected latest CPU=5.0, got %v", latest)
	}

	// History with limit
	h := am.getHistory(3)
	if len(h) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(h))
	}
	// Should be most recent 3, oldest first
	if h[0].CPUPercent != 3.0 {
		t.Fatalf("expected first history sample CPU=3.0, got %.1f", h[0].CPUPercent)
	}
	if h[2].CPUPercent != 5.0 {
		t.Fatalf("expected last history sample CPU=5.0, got %.1f", h[2].CPUPercent)
	}

	// Full history
	h = am.getHistory(0)
	if len(h) != 5 {
		t.Fatalf("expected 5 samples with max=0, got %d", len(h))
	}
}

func TestAppMonitorOverwrite(t *testing.T) {
	am := newAppMonitor(ThresholdConfig{})

	// Fill beyond capacity
	for i := 0; i < maxSamples+100; i++ {
		am.addSample(ResourceSample{
			Timestamp:  time.Now(),
			CPUPercent: float64(i),
			MemoryRSS:  int64(i * 1024),
		})
	}

	if am.count != maxSamples {
		t.Fatalf("expected count=%d, got %d", maxSamples, am.count)
	}

	// Latest should be the most recently written
	latest := am.getLatest()
	expected := float64(maxSamples + 100 - 1)
	if latest == nil || latest.CPUPercent != expected {
		t.Fatalf("expected latest CPU=%.0f, got %v", expected, latest)
	}

	// Oldest should be sample number 100 (the first 100 were overwritten)
	h := am.getHistory(0)
	if h[0].CPUPercent != 100.0 {
		t.Fatalf("expected oldest CPU=100.0, got %.0f", h[0].CPUPercent)
	}
}

func TestStatsComputation(t *testing.T) {
	am := newAppMonitor(ThresholdConfig{})

	now := time.Now()
	samples := []ResourceSample{
		{Timestamp: now, CPUPercent: 10.0, MemoryRSS: 100 * 1024 * 1024},
		{Timestamp: now.Add(2 * time.Second), CPUPercent: 20.0, MemoryRSS: 200 * 1024 * 1024},
		{Timestamp: now.Add(4 * time.Second), CPUPercent: 30.0, MemoryRSS: 150 * 1024 * 1024},
	}
	for _, s := range samples {
		am.addSample(s)
	}

	stats := am.computeStats()
	if stats == nil {
		t.Fatal("expected non-nil stats")
	}

	if stats.SampleCount != 3 {
		t.Fatalf("expected 3 samples, got %d", stats.SampleCount)
	}

	// CPU stats
	if stats.MinCPU != 10.0 {
		t.Fatalf("expected MinCPU=10.0, got %.1f", stats.MinCPU)
	}
	if stats.MaxCPU != 30.0 {
		t.Fatalf("expected MaxCPU=30.0, got %.1f", stats.MaxCPU)
	}
	expectedAvgCPU := 20.0
	if stats.AvgCPU != expectedAvgCPU {
		t.Fatalf("expected AvgCPU=%.1f, got %.1f", expectedAvgCPU, stats.AvgCPU)
	}

	// Memory stats
	if stats.MinMemory != 100*1024*1024 {
		t.Fatalf("expected MinMemory=100MB, got %d", stats.MinMemory)
	}
	if stats.MaxMemory != 200*1024*1024 {
		t.Fatalf("expected MaxMemory=200MB, got %d", stats.MaxMemory)
	}

	// Current should be the latest sample
	if stats.Current.CPUPercent != 30.0 {
		t.Fatalf("expected current CPU=30.0, got %.1f", stats.Current.CPUPercent)
	}

	// Duration
	if stats.Duration != 4*time.Second {
		t.Fatalf("expected duration=4s, got %v", stats.Duration)
	}
}

func TestThresholdExceeded(t *testing.T) {
	am := newAppMonitor(ThresholdConfig{
		MaxCPUPercent: 50.0,
		MaxMemoryMB:   100,
	})

	// Below threshold
	am.addSample(ResourceSample{
		Timestamp:  time.Now(),
		CPUPercent: 30.0,
		MemoryRSS:  50 * 1024 * 1024,
	})
	if am.isExceeded() {
		t.Fatal("should not be exceeded")
	}

	// CPU exceeded
	am.addSample(ResourceSample{
		Timestamp:  time.Now(),
		CPUPercent: 60.0,
		MemoryRSS:  50 * 1024 * 1024,
	})
	if !am.isExceeded() {
		t.Fatal("should be exceeded (CPU)")
	}

	// Memory exceeded
	am.addSample(ResourceSample{
		Timestamp:  time.Now(),
		CPUPercent: 10.0,
		MemoryRSS:  150 * 1024 * 1024,
	})
	if !am.isExceeded() {
		t.Fatal("should be exceeded (Memory)")
	}

	// Back to normal
	am.addSample(ResourceSample{
		Timestamp:  time.Now(),
		CPUPercent: 10.0,
		MemoryRSS:  50 * 1024 * 1024,
	})
	if am.isExceeded() {
		t.Fatal("should not be exceeded anymore")
	}
}

func TestThresholdAlertCooldown(t *testing.T) {
	pidCalled := 0
	rm := NewResourceMonitor(func(name string) int {
		pidCalled++
		return 0 // no PID, won't actually sample
	})

	am := newAppMonitor(ThresholdConfig{MaxCPUPercent: 50.0})
	rm.mu.Lock()
	rm.apps["test"] = am
	rm.mu.Unlock()

	// First alert should fire
	sample := ResourceSample{
		Timestamp:  time.Now(),
		CPUPercent: 80.0,
		MemoryRSS:  0,
	}
	am.addSample(sample)
	rm.checkThresholds("test", am, sample)

	select {
	case alert := <-rm.alertCh:
		if alert.Type != AlertCPU {
			t.Fatalf("expected CPU alert, got %s", alert.Type)
		}
		if alert.Value != 80.0 {
			t.Fatalf("expected value=80.0, got %.1f", alert.Value)
		}
	default:
		t.Fatal("expected an alert")
	}

	// Immediate second alert should be suppressed (cooldown)
	rm.checkThresholds("test", am, sample)
	select {
	case <-rm.alertCh:
		t.Fatal("expected no alert during cooldown")
	default:
		// expected
	}

	rm.StopAll()
}

func TestEmptyStats(t *testing.T) {
	am := newAppMonitor(ThresholdConfig{})
	if stats := am.computeStats(); stats != nil {
		t.Fatal("expected nil stats for empty monitor")
	}
}

func TestResourceMonitorRegisterUnregister(t *testing.T) {
	rm := NewResourceMonitor(func(name string) int { return 0 })

	rm.Register("app1", ThresholdConfig{})
	rm.Register("app2", ThresholdConfig{})

	rm.mu.Lock()
	count := len(rm.apps)
	rm.mu.Unlock()
	if count != 2 {
		t.Fatalf("expected 2 apps, got %d", count)
	}

	rm.Unregister("app1")
	rm.mu.Lock()
	count = len(rm.apps)
	rm.mu.Unlock()
	if count != 1 {
		t.Fatalf("expected 1 app after unregister, got %d", count)
	}

	rm.StopAll()
	rm.mu.Lock()
	count = len(rm.apps)
	rm.mu.Unlock()
	if count != 0 {
		t.Fatalf("expected 0 apps after StopAll, got %d", count)
	}
}

func TestGetLatestEmpty(t *testing.T) {
	rm := NewResourceMonitor(func(name string) int { return 0 })
	if s := rm.GetLatest("nonexistent"); s != nil {
		t.Fatal("expected nil for nonexistent app")
	}
	if s := rm.GetStats("nonexistent"); s != nil {
		t.Fatal("expected nil stats for nonexistent app")
	}
	if h := rm.GetHistory("nonexistent", 10); h != nil {
		t.Fatal("expected nil history for nonexistent app")
	}
	if rm.IsExceeded("nonexistent") {
		t.Fatal("expected not exceeded for nonexistent app")
	}
}
