package process

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// CopyToClipboard copies text to the system clipboard.
func CopyToClipboard(text string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "windows":
		cmd = exec.Command("clip")
	default:
		// Linux - check if WSL
		if isWSL() {
			cmd = exec.Command("clip.exe")
		} else {
			// Try xclip first
			cmd = exec.Command("xclip", "-selection", "clipboard")
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err == nil {
				return nil
			}
			// Fall back to xsel
			cmd = exec.Command("xsel", "--clipboard", "--input")
		}
	}

	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func isWSL() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), "microsoft")
}
