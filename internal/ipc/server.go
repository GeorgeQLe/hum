package ipc

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/georgele/devctl/internal/panicutil"
)

var socketDir = filepath.Join(os.TempDir(), "devctl-sockets")

const ipcRequestBufferSize = 16

// Request represents an IPC request from a client.
type Request struct {
	Action    string          `json:"action"`
	App       json.RawMessage `json:"app,omitempty"`
	Cwd       string          `json:"cwd,omitempty"`
	AutoStart bool            `json:"autoStart,omitempty"`
	Message   string          `json:"message,omitempty"`
	Target    string          `json:"target,omitempty"` // app name or "all"
}

// Response represents an IPC response to a client.
type Response struct {
	OK      bool            `json:"ok"`
	Error   string          `json:"error,omitempty"`
	Name    string          `json:"name,omitempty"`
	Message string          `json:"message,omitempty"`
	PID     int             `json:"pid,omitempty"`
	Project string          `json:"project,omitempty"`
	Apps    json.RawMessage `json:"apps,omitempty"`
}

// IPCRequestMsg is sent to the TUI update loop for processing.
type IPCRequestMsg struct {
	Request    Request
	ResponseCh chan Response
}

// Server listens on a Unix socket for IPC commands.
type Server struct {
	socketPath string
	listener   net.Listener
	requestCh  chan IPCRequestMsg
	stopCh     chan struct{}
	closeOnce  sync.Once
}

// SocketPath returns the socket path for a project root.
func SocketPath(projectRoot string) string {
	hash := sha256.Sum256([]byte(projectRoot))
	socketName := fmt.Sprintf("devctl-%x.sock", hash[:8])
	return filepath.Join(socketDir, socketName)
}

// NewServer creates a new IPC server for the given project root.
func NewServer(projectRoot string) (*Server, error) {
	socketPath := SocketPath(projectRoot)

	// Ensure socket directory exists
	if err := os.MkdirAll(socketDir, 0700); err != nil {
		return nil, err
	}

	// Check for stale socket
	if _, err := os.Stat(socketPath); err == nil {
		conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
		if err == nil {
			// Another instance is running
			conn.Close()
			return nil, fmt.Errorf("another devctl instance is running for this project")
		}
		// Stale socket, remove it
		os.Remove(socketPath)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}

	// Set permissions
	if err := os.Chmod(socketPath, 0600); err != nil {
		log.Printf("warning: could not set socket permissions: %v", err)
	}

	return &Server{
		socketPath: socketPath,
		listener:   listener,
		requestCh:  make(chan IPCRequestMsg, ipcRequestBufferSize),
		stopCh:     make(chan struct{}),
	}, nil
}

// Requests returns the channel for incoming IPC requests.
func (s *Server) Requests() <-chan IPCRequestMsg {
	return s.requestCh
}

// Start begins accepting connections in a background goroutine.
func (s *Server) Start() {
	go func() {
		defer panicutil.Recover("ipc accept loop")
		for {
			conn, err := s.listener.Accept()
			if err != nil {
				select {
				case <-s.stopCh:
					return
				default:
					continue
				}
			}
			go func() {
				defer panicutil.Recover("ipc connection handler")
				s.handleConnection(conn)
			}()
		}
	}()
}

// Stop closes the server and removes the socket file. Safe to call multiple times.
func (s *Server) Stop() {
	s.closeOnce.Do(func() {
		close(s.stopCh)
		s.listener.Close()
		os.Remove(s.socketPath)
		close(s.requestCh)
	})
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Set read timeout
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return
	}

	line := scanner.Text()
	var req Request
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		resp := Response{OK: false, Error: "Invalid JSON"}
		data, err := json.Marshal(resp)
		if err != nil {
			log.Printf("ipc: failed to marshal error response: %v", err)
			return
		}
		conn.Write(append(data, '\n'))
		return
	}

	// Send request to TUI and wait for response
	responseCh := make(chan Response, 1)
	select {
	case s.requestCh <- IPCRequestMsg{Request: req, ResponseCh: responseCh}:
	case <-time.After(5 * time.Second):
		resp := Response{OK: false, Error: "Request timeout"}
		data, err := json.Marshal(resp)
		if err != nil {
			log.Printf("ipc: failed to marshal timeout response: %v", err)
			return
		}
		conn.Write(append(data, '\n'))
		return
	}

	// Wait for response from TUI
	select {
	case resp := <-responseCh:
		data, err := json.Marshal(resp)
		if err != nil {
			log.Printf("ipc: failed to marshal response: %v", err)
			return
		}
		conn.Write(append(data, '\n'))
	case <-time.After(10 * time.Second):
		resp := Response{OK: false, Error: "Response timeout"}
		data, err := json.Marshal(resp)
		if err != nil {
			log.Printf("ipc: failed to marshal timeout response: %v", err)
			return
		}
		conn.Write(append(data, '\n'))
	}
}

// Cleanup removes the IPC socket file for a project root.
// Safe to call even if no server is running.
func Cleanup(projectRoot string) {
	os.Remove(SocketPath(projectRoot))
}
