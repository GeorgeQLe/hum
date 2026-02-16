package tui

import (
	"strings"
)

func (m *Model) scrollLog(delta int) {
	bufName := m.getSelectedBufName()
	logBuf := m.procManager.GetLogBuffer(bufName)
	logBuf.ScrollBy(delta, m.logViewHeight())
}

func (m *Model) scrollToSearchMatch() {
	if m.searchMode == nil {
		return
	}
	match := m.searchMode.currentMatch()
	if match == nil {
		return
	}
	bufName := m.getSelectedBufName()
	logBuf := m.procManager.GetLogBuffer(bufName)
	viewHeight := m.logViewHeight()

	if match.LineIdx < logBuf.ScrollPos {
		logBuf.ScrollTo(match.LineIdx, viewHeight)
	} else if match.LineIdx >= logBuf.ScrollPos+viewHeight {
		logBuf.ScrollTo(match.LineIdx-viewHeight+1, viewHeight)
	}
}

func (m *Model) navigateHistory(dir int) {
	if len(m.cmdHistory) == 0 {
		return
	}
	if dir < 0 {
		if m.historyIdx == -1 {
			m.historyTmp = m.cmdInput
			m.historyIdx = len(m.cmdHistory) - 1
		} else if m.historyIdx > 0 {
			m.historyIdx--
		} else {
			return
		}
		m.cmdInput = m.cmdHistory[m.historyIdx]
	} else {
		if m.historyIdx == -1 {
			return
		}
		if m.historyIdx < len(m.cmdHistory)-1 {
			m.historyIdx++
			m.cmdInput = m.cmdHistory[m.historyIdx]
		} else {
			m.historyIdx = -1
			m.cmdInput = m.historyTmp
		}
	}
	m.cmdCursor = len(m.cmdInput)
}

func (m *Model) handleTabCompletion() {
	if m.tabMatches == nil {
		matches, partial := m.complete(m.cmdInput[:m.cmdCursor])
		if len(matches) == 0 {
			return
		}

		if len(matches) == 1 {
			before := m.cmdInput[:m.cmdCursor-len(partial)]
			after := m.cmdInput[m.cmdCursor:]
			m.cmdInput = before + matches[0] + " " + after
			m.cmdCursor = len(before) + len(matches[0]) + 1
			return
		}

		cp := commonPrefix(matches)
		if len(cp) > len(partial) {
			before := m.cmdInput[:m.cmdCursor-len(partial)]
			after := m.cmdInput[m.cmdCursor:]
			m.cmdInput = before + cp + after
			m.cmdCursor = len(before) + len(cp)
		}

		m.tabMatches = matches
		m.tabIdx = 0
		m.tabPartial = cp
		m.tabOrig = m.cmdInput
		m.systemLog("Completions: " + strings.Join(matches, "  "))
		return
	}

	// Cycle through matches
	m.tabIdx = (m.tabIdx + 1) % len(m.tabMatches)
	match := m.tabMatches[m.tabIdx]
	before := m.tabOrig[:len(m.tabOrig)-len(m.tabPartial)]
	m.cmdInput = before + match
	m.cmdCursor = len(m.cmdInput)
}
