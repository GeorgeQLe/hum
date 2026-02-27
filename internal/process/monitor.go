package process

import (
	"sync"
	"time"

	"github.com/georgele/hum/internal/panicutil"
)

const (
	defaultPollInterval = 2 * time.Second
	maxSamples          = 1800 // 1hr at 2s intervals
	alertCooldown       = 30 * time.Second
)

// ResourceSample is a single point-in-time measurement.
type ResourceSample struct {
	Timestamp  time.Time
	CPUPercent float64
	MemoryRSS  int64 // bytes
}

// ResourceStats holds aggregated statistics computed from a sample buffer.
type ResourceStats struct {
	Current    ResourceSample
	AvgCPU     float64
	MinCPU     float64
	MaxCPU     float64
	AvgMemory  int64
	MinMemory  int64
	MaxMemory  int64
	SampleCount int
	Duration   time.Duration // time span of samples
}

// ThresholdConfig defines resource limits for an app.
type ThresholdConfig struct {
	MaxCPUPercent float64
	MaxMemoryMB   int64
}

// AlertType identifies the kind of threshold breach.
type AlertType string

const (
	AlertCPU    AlertType = "cpu"
	AlertMemory AlertType = "memory"
)

// ThresholdAlert is emitted when a resource threshold is breached.
type ThresholdAlert struct {
	AppName   string
	Type      AlertType
	Value     float64 // current value (CPU% or MB)
	Threshold float64 // configured limit
	Timestamp time.Time
}

// appMonitor tracks samples and thresholds for a single app.
type appMonitor struct {
	samples   []ResourceSample
	writeIdx  int
	count     int
	threshold ThresholdConfig
	lastAlert time.Time
	stopCh    chan struct{}
	mu        sync.Mutex
}

func newAppMonitor(cfg ThresholdConfig) *appMonitor {
	return &appMonitor{
		samples:   make([]ResourceSample, maxSamples),
		threshold: cfg,
		stopCh:    make(chan struct{}),
	}
}

func (am *appMonitor) addSample(s ResourceSample) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.samples[am.writeIdx] = s
	am.writeIdx = (am.writeIdx + 1) % maxSamples
	if am.count < maxSamples {
		am.count++
	}
}

func (am *appMonitor) getLatest() *ResourceSample {
	am.mu.Lock()
	defer am.mu.Unlock()
	if am.count == 0 {
		return nil
	}
	idx := (am.writeIdx - 1 + maxSamples) % maxSamples
	s := am.samples[idx]
	return &s
}

func (am *appMonitor) getHistory(max int) []ResourceSample {
	am.mu.Lock()
	defer am.mu.Unlock()
	if am.count == 0 {
		return nil
	}
	n := am.count
	if max > 0 && max < n {
		n = max
	}
	result := make([]ResourceSample, n)
	// Read from oldest to newest
	start := (am.writeIdx - am.count + maxSamples) % maxSamples
	offset := am.count - n
	for i := 0; i < n; i++ {
		result[i] = am.samples[(start+offset+i)%maxSamples]
	}
	return result
}

func (am *appMonitor) computeStats() *ResourceStats {
	am.mu.Lock()
	defer am.mu.Unlock()
	if am.count == 0 {
		return nil
	}

	start := (am.writeIdx - am.count + maxSamples) % maxSamples
	first := am.samples[start]
	latest := am.samples[(am.writeIdx-1+maxSamples)%maxSamples]

	stats := &ResourceStats{
		Current:     latest,
		MinCPU:      first.CPUPercent,
		MaxCPU:      first.CPUPercent,
		MinMemory:   first.MemoryRSS,
		MaxMemory:   first.MemoryRSS,
		SampleCount: am.count,
		Duration:    latest.Timestamp.Sub(first.Timestamp),
	}

	var totalCPU float64
	var totalMem int64
	for i := 0; i < am.count; i++ {
		s := am.samples[(start+i)%maxSamples]
		totalCPU += s.CPUPercent
		totalMem += s.MemoryRSS
		if s.CPUPercent < stats.MinCPU {
			stats.MinCPU = s.CPUPercent
		}
		if s.CPUPercent > stats.MaxCPU {
			stats.MaxCPU = s.CPUPercent
		}
		if s.MemoryRSS < stats.MinMemory {
			stats.MinMemory = s.MemoryRSS
		}
		if s.MemoryRSS > stats.MaxMemory {
			stats.MaxMemory = s.MemoryRSS
		}
	}
	stats.AvgCPU = totalCPU / float64(am.count)
	stats.AvgMemory = totalMem / int64(am.count)

	return stats
}

func (am *appMonitor) isExceeded() bool {
	am.mu.Lock()
	defer am.mu.Unlock()
	if am.count == 0 {
		return false
	}
	latest := am.samples[(am.writeIdx-1+maxSamples)%maxSamples]
	if am.threshold.MaxCPUPercent > 0 && latest.CPUPercent > am.threshold.MaxCPUPercent {
		return true
	}
	if am.threshold.MaxMemoryMB > 0 && latest.MemoryRSS > am.threshold.MaxMemoryMB*1024*1024 {
		return true
	}
	return false
}

// ResourceMonitor manages per-app resource polling goroutines.
type ResourceMonitor struct {
	mu      sync.Mutex
	apps    map[string]*appMonitor
	alertCh chan ThresholdAlert
	pidFunc func(string) int
}

// NewResourceMonitor creates a monitor. pidFunc should return the PID for an app name (e.g. Manager.PID).
func NewResourceMonitor(pidFunc func(string) int) *ResourceMonitor {
	return &ResourceMonitor{
		apps:    make(map[string]*appMonitor),
		alertCh: make(chan ThresholdAlert, 64),
		pidFunc: pidFunc,
	}
}

// Register starts polling for the named app with the given thresholds.
func (rm *ResourceMonitor) Register(appName string, cfg ThresholdConfig) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Stop existing monitor if any
	if existing, ok := rm.apps[appName]; ok {
		close(existing.stopCh)
	}

	am := newAppMonitor(cfg)
	rm.apps[appName] = am

	go func() {
		defer panicutil.Recover("resource monitor")
		rm.poll(appName, am)
	}()
}

// Unregister stops polling for the named app.
func (rm *ResourceMonitor) Unregister(appName string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if am, ok := rm.apps[appName]; ok {
		close(am.stopCh)
		delete(rm.apps, appName)
	}
}

// GetStats returns aggregated resource statistics for an app.
func (rm *ResourceMonitor) GetStats(appName string) *ResourceStats {
	rm.mu.Lock()
	am, ok := rm.apps[appName]
	rm.mu.Unlock()
	if !ok {
		return nil
	}
	return am.computeStats()
}

// GetLatest returns the most recent sample for an app.
func (rm *ResourceMonitor) GetLatest(appName string) *ResourceSample {
	rm.mu.Lock()
	am, ok := rm.apps[appName]
	rm.mu.Unlock()
	if !ok {
		return nil
	}
	return am.getLatest()
}

// GetHistory returns up to maxSamples historical samples (oldest first).
func (rm *ResourceMonitor) GetHistory(appName string, max int) []ResourceSample {
	rm.mu.Lock()
	am, ok := rm.apps[appName]
	rm.mu.Unlock()
	if !ok {
		return nil
	}
	return am.getHistory(max)
}

// IsExceeded returns true if the app's latest sample exceeds its thresholds.
func (rm *ResourceMonitor) IsExceeded(appName string) bool {
	rm.mu.Lock()
	am, ok := rm.apps[appName]
	rm.mu.Unlock()
	if !ok {
		return false
	}
	return am.isExceeded()
}

// Alerts returns a channel that emits threshold breach alerts.
func (rm *ResourceMonitor) Alerts() <-chan ThresholdAlert {
	return rm.alertCh
}

// StopAll stops all polling goroutines.
func (rm *ResourceMonitor) StopAll() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	for name, am := range rm.apps {
		close(am.stopCh)
		delete(rm.apps, name)
	}
}

func (rm *ResourceMonitor) poll(appName string, am *appMonitor) {
	// Initial sample immediately
	rm.doSample(appName, am)

	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-am.stopCh:
			return
		case <-ticker.C:
			rm.doSample(appName, am)
		}
	}
}

func (rm *ResourceMonitor) doSample(appName string, am *appMonitor) {
	pid := rm.pidFunc(appName)
	if pid == 0 {
		return
	}

	usage, err := GetResourceUsage(pid)
	if err != nil {
		return
	}

	sample := ResourceSample{
		Timestamp:  time.Now(),
		CPUPercent: usage.CPUPercent,
		MemoryRSS:  usage.MemoryRSS,
	}
	am.addSample(sample)

	// Check thresholds with cooldown
	rm.checkThresholds(appName, am, sample)
}

func (rm *ResourceMonitor) checkThresholds(appName string, am *appMonitor, s ResourceSample) {
	am.mu.Lock()
	lastAlert := am.lastAlert
	threshold := am.threshold
	am.mu.Unlock()

	if time.Since(lastAlert) < alertCooldown {
		return
	}

	var alerts []ThresholdAlert

	if threshold.MaxCPUPercent > 0 && s.CPUPercent > threshold.MaxCPUPercent {
		alerts = append(alerts, ThresholdAlert{
			AppName:   appName,
			Type:      AlertCPU,
			Value:     s.CPUPercent,
			Threshold: threshold.MaxCPUPercent,
			Timestamp: s.Timestamp,
		})
	}

	if threshold.MaxMemoryMB > 0 && s.MemoryRSS > threshold.MaxMemoryMB*1024*1024 {
		memMB := float64(s.MemoryRSS) / (1024 * 1024)
		alerts = append(alerts, ThresholdAlert{
			AppName:   appName,
			Type:      AlertMemory,
			Value:     memMB,
			Threshold: float64(threshold.MaxMemoryMB),
			Timestamp: s.Timestamp,
		})
	}

	if len(alerts) > 0 {
		am.mu.Lock()
		am.lastAlert = time.Now()
		am.mu.Unlock()

		for _, alert := range alerts {
			select {
			case rm.alertCh <- alert:
			default:
			}
		}
	}
}
