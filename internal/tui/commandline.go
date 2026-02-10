package tui

import (
	"strings"
)

// Commands available in the command line.
var commandNames = []string{
	"start", "stop", "restart", "status",
	"ports", "scan", "add", "remove", "reload",
	"autorestart", "clear-errors", "list", "help", "quit",
}

// Commands that accept an app name argument.
var commandsWithName = map[string]bool{
	"start": true, "stop": true, "restart": true,
	"status": true, "remove": true, "autorestart": true,
	"clear-errors": true,
}

// Commands that also accept "all".
var commandsWithAll = map[string]bool{
	"start": true, "stop": true, "restart": true,
	"clear-errors": true,
}

// complete returns tab completion candidates for the current input.
func (m *Model) complete(input string) (matches []string, partial string) {
	parts := strings.Fields(input)
	trimmed := strings.TrimLeft(input, " ")

	// Command completion
	if len(parts) <= 1 {
		p := ""
		if len(parts) == 1 {
			p = parts[0]
		}
		for _, cmd := range commandNames {
			if strings.HasPrefix(cmd, p) {
				matches = append(matches, cmd)
			}
		}
		return matches, p
	}

	// Argument completion
	cmd := parts[0]
	_ = trimmed
	p := ""
	if len(parts) > 1 {
		p = parts[len(parts)-1]
	}

	if commandsWithName[cmd] {
		for _, app := range m.apps {
			if strings.HasPrefix(app.Name, p) {
				matches = append(matches, app.Name)
			}
		}
		if commandsWithAll[cmd] && strings.HasPrefix("all", p) {
			matches = append(matches, "all")
		}
	}

	return matches, p
}

// commonPrefix finds the longest common prefix among strings.
func commonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	prefix := strs[0]
	for _, s := range strs[1:] {
		for !strings.HasPrefix(s, prefix) {
			prefix = prefix[:len(prefix)-1]
			if prefix == "" {
				return ""
			}
		}
	}
	return prefix
}

// parseCommand splits "command args" into (command, args).
func parseCommand(line string) (cmd, args string) {
	line = strings.TrimSpace(line)
	idx := strings.IndexByte(line, ' ')
	if idx == -1 {
		return line, ""
	}
	return line[:idx], strings.TrimSpace(line[idx+1:])
}
