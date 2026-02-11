package tui

import (
	"strings"
)

// Commands available in the command line.
var commandNames = []string{
	"start", "stop", "restart", "status",
	"ports", "scan", "add", "remove", "reload",
	"autorestart", "clear-errors", "export",
	"pin", "unpin", "run",
	"list", "help", "quit",
}

// Commands that accept an app name argument.
var commandsWithName = map[string]bool{
	"start": true, "stop": true, "restart": true,
	"status": true, "remove": true, "autorestart": true,
	"clear-errors": true, "export": true,
	"pin": true, "unpin": true, "run": true,
}

// Commands that also accept "all".
var commandsWithAll = map[string]bool{
	"start": true, "stop": true, "restart": true,
	"clear-errors": true,
}

// complete returns tab completion candidates for the current input.
func (m *Model) complete(input string) (matches []string, partial string) {
	parts := strings.Fields(input)
	hasTrailingSpace := len(input) > 0 && input[len(input)-1] == ' '

	// Command completion (no parts, or one part without trailing space)
	if len(parts) == 0 || (len(parts) == 1 && !hasTrailingSpace) {
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

	// Argument completion (D4: trailing space triggers argument completion)
	cmd := parts[0]
	p := ""
	if !hasTrailingSpace {
		p = parts[len(parts)-1]
	}

	if commandsWithName[cmd] {
		// For "run", if we already have an app name, complete command types
		if cmd == "run" && len(parts) >= 3 || (cmd == "run" && len(parts) == 2 && hasTrailingSpace) {
			appName := parts[1]
			app := m.findApp(appName)
			if app != nil && len(app.Commands) > 0 {
				for cmdType := range app.Commands {
					if strings.HasPrefix(cmdType, p) {
						matches = append(matches, cmdType)
					}
				}
				return matches, p
			}
			return nil, p
		}

		// @group completion
		if strings.HasPrefix(p, "@") {
			groupPrefix := p[1:]
			seen := make(map[string]bool)
			for _, app := range m.apps {
				if app.Group != "" && !seen[app.Group] && strings.HasPrefix(app.Group, groupPrefix) {
					matches = append(matches, "@"+app.Group)
					seen[app.Group] = true
				}
			}
			return matches, p
		}

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
