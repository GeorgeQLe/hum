package health

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/georgele/hum/internal/panicutil"
)

// Status represents the health status of an app.
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusUnknown   Status = "unknown"

	defaultTimeout = 5 * time.Second
)

// StatusChange is emitted when an app's health status changes.
type StatusChange struct {
	AppName   string
	OldStatus Status
	NewStatus Status
}

type appChecker struct {
	url      string
	interval time.Duration
	status   Status
	stopCh   chan struct{}
}

// Checker manages health check goroutines for registered apps.
type Checker struct {
	mu       sync.Mutex
	apps     map[string]*appChecker
	changeCh chan StatusChange
	client   *http.Client
}

// NewChecker creates a new health checker.
func NewChecker() *Checker {
	return &Checker{
		apps:     make(map[string]*appChecker),
		changeCh: make(chan StatusChange, 64),
		client: &http.Client{
			Timeout: defaultTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Changes returns a channel that emits health status changes.
func (c *Checker) Changes() <-chan StatusChange {
	return c.changeCh
}

// ValidateHealthURL checks that a health check URL uses http/https and points
// to a loopback address. This prevents SSRF attacks where a malicious config
// could probe internal network services.
func ValidateHealthURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid health check URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("health check URL must use http or https scheme, got %q", u.Scheme)
	}
	host := u.Hostname()
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("health check URL must point to localhost, got %q", host)
	}
	return nil
}

// Register starts health checking for an app. Returns an error if the URL
// fails validation.
func (c *Checker) Register(appName, rawURL string, intervalMs int) error {
	if err := ValidateHealthURL(rawURL); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Stop existing checker if any
	if existing, ok := c.apps[appName]; ok {
		close(existing.stopCh)
	}

	interval := time.Duration(intervalMs) * time.Millisecond
	if interval < 1*time.Second {
		interval = 5 * time.Second
	}

	ac := &appChecker{
		url:      rawURL,
		interval: interval,
		status:   StatusUnknown,
		stopCh:   make(chan struct{}),
	}
	c.apps[appName] = ac

	go func() {
		defer panicutil.Recover("health poll")
		c.poll(appName, ac)
	}()

	return nil
}

// Unregister stops health checking for an app.
func (c *Checker) Unregister(appName string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ac, ok := c.apps[appName]; ok {
		close(ac.stopCh)
		delete(c.apps, appName)
	}
}

// GetStatus returns the current health status for an app.
func (c *Checker) GetStatus(appName string) Status {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ac, ok := c.apps[appName]; ok {
		return ac.status
	}
	return StatusUnknown
}

// HasCheck returns true if the app has health checking configured.
func (c *Checker) HasCheck(appName string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.apps[appName]
	return ok
}

// StopAll stops all health checkers.
func (c *Checker) StopAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for name, ac := range c.apps {
		close(ac.stopCh)
		delete(c.apps, name)
	}
}

func (c *Checker) poll(appName string, ac *appChecker) {
	// Do an initial check immediately
	c.doCheck(appName, ac)

	ticker := time.NewTicker(ac.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ac.stopCh:
			return
		case <-ticker.C:
			c.doCheck(appName, ac)
		}
	}
}

func (c *Checker) doCheck(appName string, ac *appChecker) {
	resp, err := c.client.Get(ac.url)
	var newStatus Status
	if err != nil {
		newStatus = StatusUnhealthy
	} else {
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			newStatus = StatusHealthy
		} else {
			newStatus = StatusUnhealthy
		}
	}

	c.mu.Lock()
	oldStatus := ac.status
	ac.status = newStatus
	c.mu.Unlock()

	if oldStatus != newStatus {
		select {
		case c.changeCh <- StatusChange{
			AppName:   appName,
			OldStatus: oldStatus,
			NewStatus: newStatus,
		}:
		default:
			log.Printf("health: status update dropped for %s (channel full)", appName)
		}
	}
}
