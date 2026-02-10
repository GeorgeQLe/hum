package tui

import "github.com/charmbracelet/bubbletea"

// Key binding helpers for the TUI.

func isKey(msg tea.KeyMsg, keys ...string) bool {
	for _, k := range keys {
		if msg.String() == k {
			return true
		}
	}
	return false
}

func isCtrl(msg tea.KeyMsg, key string) bool {
	return msg.String() == "ctrl+"+key
}

func isRune(msg tea.KeyMsg, r rune) bool {
	if msg.Type != tea.KeyRunes {
		return false
	}
	runes := msg.Runes
	return len(runes) == 1 && runes[0] == r
}
