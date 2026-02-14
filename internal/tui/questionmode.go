package tui

import tea "github.com/charmbracelet/bubbletea"

// QuestionMode holds state for an interactive prompt.
type QuestionMode struct {
	Prompt   string
	Input    string
	Cursor   int
	callback func(string)
}

// handleQuestionKeypress processes key events in question mode.
func (m Model) handleQuestionKeypress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	q := m.questionMode

	switch {
	case isKey(msg, "enter"):
		answer := q.Input
		cb := q.callback
		m.questionMode = nil
		cb(answer)
		// Pop next question from queue if available
		if m.questionMode == nil && len(m.questionQueue) > 0 {
			next := m.questionQueue[0]
			m.questionQueue = m.questionQueue[1:]
			m.questionMode = &QuestionMode{
				Prompt:   next.prompt,
				Input:    "",
				Cursor:   0,
				callback: next.callback,
			}
		}
		return m, nil

	case isKey(msg, "esc"):
		cb := q.callback
		m.questionMode = nil
		cb("") // empty string signals cancel
		// Pop next question from queue if available
		if m.questionMode == nil && len(m.questionQueue) > 0 {
			next := m.questionQueue[0]
			m.questionQueue = m.questionQueue[1:]
			m.questionMode = &QuestionMode{
				Prompt:   next.prompt,
				Input:    "",
				Cursor:   0,
				callback: next.callback,
			}
		}
		return m, nil

	case isKey(msg, "backspace"):
		if q.Cursor > 0 {
			q.Input = q.Input[:q.Cursor-1] + q.Input[q.Cursor:]
			q.Cursor--
		}
		return m, nil

	case isKey(msg, "left"):
		if q.Cursor > 0 {
			q.Cursor--
		}
		return m, nil

	case isKey(msg, "right"):
		if q.Cursor < len(q.Input) {
			q.Cursor++
		}
		return m, nil

	case isKey(msg, "home") || isCtrl(msg, "a"):
		q.Cursor = 0
		return m, nil

	case isKey(msg, "end") || isCtrl(msg, "e"):
		q.Cursor = len(q.Input)
		return m, nil

	case isCtrl(msg, "u"):
		q.Input = ""
		q.Cursor = 0
		return m, nil
	}

	// Regular character input
	if msg.Type == tea.KeyRunes {
		ch := string(msg.Runes)
		q.Input = q.Input[:q.Cursor] + ch + q.Input[q.Cursor:]
		q.Cursor += len(ch)
		return m, nil
	}

	return m, nil
}

// askQuestion starts an interactive question prompt.
// If a question is already active, the new question is queued.
func (m *Model) askQuestion(prompt string, callback func(string)) {
	if m.questionMode != nil {
		m.questionQueue = append(m.questionQueue, struct {
			prompt   string
			callback func(string)
		}{prompt, callback})
		return
	}
	m.questionMode = &QuestionMode{
		Prompt:   prompt,
		Input:    "",
		Cursor:   0,
		callback: callback,
	}
}
