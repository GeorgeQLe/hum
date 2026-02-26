package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ApprovalRule defines how an action is handled.
type ApprovalRule string

const (
	ApprovalRequired ApprovalRule = "required"
	ApprovalAuto     ApprovalRule = "auto"
)

// ApprovalConfig holds the approval system configuration.
type ApprovalConfig struct {
	TimeoutSeconds int                     `json:"timeout_seconds"`
	Rules          map[string]ApprovalRule `json:"rules"`
}

// DevctlConfig represents ~/.devctl/config.json.
type DevctlConfig struct {
	Approval ApprovalConfig `json:"approval"`
}

// DefaultConfig returns the default approval configuration.
func DefaultConfig() DevctlConfig {
	return DevctlConfig{
		Approval: ApprovalConfig{
			TimeoutSeconds: 60,
			Rules: map[string]ApprovalRule{
				"register": ApprovalRequired,
				"remove":   ApprovalRequired,
				"start":    ApprovalRequired,
				"stop":     ApprovalRequired,
				"restart":  ApprovalRequired,
				"scan":     ApprovalRequired,
			},
		},
	}
}

// LoadDevctlConfig loads ~/.devctl/config.json, creating it with defaults if missing.
func LoadDevctlConfig() DevctlConfig {
	path := filepath.Join(GlobalDir(), "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultConfig()
	}
	var cfg DevctlConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig()
	}
	// Fill defaults for missing rules
	def := DefaultConfig()
	if cfg.Approval.TimeoutSeconds <= 0 {
		cfg.Approval.TimeoutSeconds = def.Approval.TimeoutSeconds
	}
	if cfg.Approval.Rules == nil {
		cfg.Approval.Rules = def.Approval.Rules
	}
	for k, v := range def.Approval.Rules {
		if _, ok := cfg.Approval.Rules[k]; !ok {
			cfg.Approval.Rules[k] = v
		}
	}
	return cfg
}

// SaveDevctlConfig writes the config to ~/.devctl/config.json.
func SaveDevctlConfig(cfg DevctlConfig) error {
	dir := GlobalDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(dir, "config.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ApprovalDecision is the human's response to an approval request.
type ApprovalDecision int

const (
	DecisionPending ApprovalDecision = iota
	DecisionApproved
	DecisionDenied
	DecisionTimeout
	DecisionSkipped
)

// ApprovalRequest represents a pending approval request.
type ApprovalRequest struct {
	ID         string    `json:"id"`
	Action     string    `json:"action"`
	AppName    string    `json:"app_name,omitempty"`
	ClientName string    `json:"client_name"`
	Detail     string    `json:"detail,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	Deadline   time.Time `json:"deadline"`

	decision chan ApprovalDecision
}

// ApprovalQueue manages pending approval requests for TUI display.
type ApprovalQueue struct {
	mu       sync.Mutex
	pending  []*ApprovalRequest
	config   ApprovalConfig
	notifyCh chan struct{} // signaled when a new request is added
}

// NewApprovalQueue creates an approval queue with the given configuration.
func NewApprovalQueue(cfg ApprovalConfig) *ApprovalQueue {
	return &ApprovalQueue{
		config:   cfg,
		notifyCh: make(chan struct{}, 16),
	}
}

// NeedsApproval checks if the given action requires human approval.
func (q *ApprovalQueue) NeedsApproval(action string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	rule, ok := q.config.Rules[action]
	if !ok {
		return true // default to required
	}
	return rule == ApprovalRequired
}

// Submit adds a request to the queue and blocks until a decision is made or timeout.
// Returns the decision.
func (q *ApprovalQueue) Submit(id, action, appName, clientName, detail string) ApprovalDecision {
	q.mu.Lock()

	// Dedup: reuse pending request with same ID
	for _, req := range q.pending {
		if req.ID == id {
			ch := req.decision
			q.mu.Unlock()
			return <-ch
		}
	}

	timeout := time.Duration(q.config.TimeoutSeconds) * time.Second
	req := &ApprovalRequest{
		ID:         id,
		Action:     action,
		AppName:    appName,
		ClientName: clientName,
		Detail:     detail,
		CreatedAt:  time.Now(),
		Deadline:   time.Now().Add(timeout),
		decision:   make(chan ApprovalDecision, 1),
	}
	q.pending = append(q.pending, req)
	q.mu.Unlock()

	// Notify TUI
	select {
	case q.notifyCh <- struct{}{}:
	default:
	}

	// Wait for decision or timeout
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case d := <-req.decision:
		return d
	case <-timer.C:
		q.remove(id)
		return DecisionTimeout
	}
}

// Notify returns the channel that signals new pending requests.
func (q *ApprovalQueue) Notify() <-chan struct{} {
	return q.notifyCh
}

// Pending returns a snapshot of all pending requests.
func (q *ApprovalQueue) Pending() []ApprovalRequest {
	q.mu.Lock()
	defer q.mu.Unlock()
	result := make([]ApprovalRequest, len(q.pending))
	for i, r := range q.pending {
		result[i] = *r
	}
	return result
}

// PendingCount returns the number of pending requests.
func (q *ApprovalQueue) PendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

// Decide resolves the first pending request with the given decision.
func (q *ApprovalQueue) Decide(decision ApprovalDecision) {
	q.mu.Lock()
	if len(q.pending) == 0 {
		q.mu.Unlock()
		return
	}
	req := q.pending[0]
	q.pending = q.pending[1:]
	q.mu.Unlock()

	select {
	case req.decision <- decision:
	default:
	}
}

// DecideByID resolves a specific request by ID.
func (q *ApprovalQueue) DecideByID(id string, decision ApprovalDecision) bool {
	q.mu.Lock()
	var found *ApprovalRequest
	newPending := q.pending[:0]
	for _, req := range q.pending {
		if req.ID == id {
			found = req
		} else {
			newPending = append(newPending, req)
		}
	}
	q.pending = newPending
	q.mu.Unlock()

	if found == nil {
		return false
	}
	select {
	case found.decision <- decision:
	default:
	}
	return true
}

// DenyAll denies all pending requests (used during shutdown).
func (q *ApprovalQueue) DenyAll() {
	q.mu.Lock()
	pending := q.pending
	q.pending = nil
	q.mu.Unlock()

	for _, req := range pending {
		select {
		case req.decision <- DecisionDenied:
		default:
		}
	}
}

func (q *ApprovalQueue) remove(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, r := range q.pending {
		if r.ID == id {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			return
		}
	}
}

// FormatDecision returns a human-readable string for a decision.
func FormatDecision(d ApprovalDecision) string {
	switch d {
	case DecisionApproved:
		return "approved"
	case DecisionDenied:
		return "denied"
	case DecisionTimeout:
		return "timeout"
	case DecisionSkipped:
		return "skipped"
	default:
		return fmt.Sprintf("unknown(%d)", d)
	}
}
