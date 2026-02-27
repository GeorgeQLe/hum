package tui

import (
	"regexp"
	"strings"

	"github.com/georgele/hum/internal/process"
)

// SearchMode holds the state for log search.
type SearchMode struct {
	pattern  string
	matches  []SearchMatch
	matchIdx int
	regex    *regexp.Regexp
}

// SearchMatch records a match location in the log buffer.
type SearchMatch struct {
	LineIdx int
	Start   int
	End     int
}

func newSearchMode() *SearchMode {
	return &SearchMode{
		matchIdx: -1,
	}
}

func (sm *SearchMode) updateMatches(logBuf *process.LogBuffer) {
	sm.matches = nil
	sm.matchIdx = -1

	if sm.pattern == "" {
		sm.regex = nil
		return
	}

	// Always escape user input to prevent ReDoS from pathological patterns.
	// This gives safe substring matching while remaining fast on any input.
	escaped := regexp.QuoteMeta(sm.pattern)
	re, _ := regexp.Compile("(?i)" + escaped)
	sm.regex = re

	if re == nil {
		return
	}

	lines, _, _ := logBuf.Snapshot()
	for i, line := range lines {
		stripped := process.StripAnsi(line.Text)
		locs := re.FindAllStringIndex(stripped, -1)
		for _, loc := range locs {
			sm.matches = append(sm.matches, SearchMatch{
				LineIdx: i,
				Start:   loc[0],
				End:     loc[1],
			})
		}
	}

	if len(sm.matches) > 0 {
		sm.matchIdx = 0
	}
}

func (sm *SearchMode) navigate(delta int) {
	if len(sm.matches) == 0 {
		return
	}
	sm.matchIdx += delta
	if sm.matchIdx < 0 {
		sm.matchIdx = len(sm.matches) - 1
	} else if sm.matchIdx >= len(sm.matches) {
		sm.matchIdx = 0
	}
}

func (sm *SearchMode) currentMatch() *SearchMatch {
	if sm.matchIdx < 0 || sm.matchIdx >= len(sm.matches) {
		return nil
	}
	return &sm.matches[sm.matchIdx]
}

// highlightSearch applies search highlighting to a log line.
func (m *Model) highlightSearch(line string, lineIdx int) string {
	sm := m.searchMode
	if sm == nil || sm.regex == nil || len(sm.matches) == 0 {
		return line
	}

	// Find matches on this line
	var lineMatches []SearchMatch
	for _, match := range sm.matches {
		if match.LineIdx == lineIdx {
			lineMatches = append(lineMatches, match)
		}
	}
	if len(lineMatches) == 0 {
		return line
	}

	stripped := process.StripAnsi(line)
	var result strings.Builder
	lastEnd := 0

	for _, match := range lineMatches {
		if match.Start > lastEnd {
			result.WriteString(stripped[lastEnd:match.Start])
		}

		isCurrent := sm.matchIdx >= 0 &&
			sm.matches[sm.matchIdx].LineIdx == lineIdx &&
			sm.matches[sm.matchIdx].Start == match.Start

		matchText := stripped[match.Start:match.End]
		if isCurrent {
			// Magenta background for current match
			result.WriteString("\x1b[45m\x1b[37m" + matchText + "\x1b[0m")
		} else {
			// Yellow background for other matches
			result.WriteString("\x1b[43m\x1b[30m" + matchText + "\x1b[0m")
		}
		lastEnd = match.End
	}

	if lastEnd < len(stripped) {
		result.WriteString(stripped[lastEnd:])
	}

	return result.String()
}
