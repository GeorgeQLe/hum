package process

import (
	"os/exec"
	"runtime"
	"strings"
)

// SendNotification sends a desktop notification.
// Uses osascript on macOS and notify-send on Linux.
func SendNotification(title, message string) error {
	switch runtime.GOOS {
	case "darwin":
		script := `display notification "` + escapeAppleScript(message) + `" with title "` + escapeAppleScript(title) + `"`
		return exec.Command("osascript", "-e", script).Run()
	case "linux":
		return exec.Command("notify-send", title, message).Run()
	default:
		return nil // silently ignore unsupported platforms
	}
}

func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	// Strip other control characters
	var b strings.Builder
	for _, r := range s {
		if r >= 32 || r == '\t' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
