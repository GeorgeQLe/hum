package process

import (
	"os/exec"
	"runtime"
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
	// Escape backslashes and double quotes for AppleScript strings
	var result []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' || s[i] == '"' {
			result = append(result, '\\')
		}
		result = append(result, s[i])
	}
	return string(result)
}
