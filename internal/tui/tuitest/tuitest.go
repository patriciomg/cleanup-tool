// Package tuitest provides small helpers for testing Bubble Tea models and
// sub-models. It reduces the boilerplate of constructing key messages and
// performing type-safe updates.
package tuitest

import tea "github.com/charmbracelet/bubbletea"

// Key returns a key message for a single rune.
func Key(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// Esc returns an Esc key message.
func Esc() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEsc}
}

// Send delivers a message to a model and returns the updated concrete model.
// It panics if the model's Update returns a different concrete type than M, so
// callers must ensure the model returns the same concrete type from Update.
func Send[M tea.Model](m M, msg tea.Msg) (M, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(M), cmd
}

// SendKey sends a single-character key to the model and returns the updated
// concrete model.
func SendKey[M tea.Model](m M, r rune) (M, tea.Cmd) {
	return Send(m, Key(r))
}
