package tuitest

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type testModel struct {
	value int
}

func (m testModel) Init() tea.Cmd { return nil }

func (m testModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && k.Type == tea.KeyRunes && len(k.Runes) == 1 && k.Runes[0] == 'a' {
		m.value++
	}
	return m, nil
}

func (m testModel) View() string { return "" }

func TestKey(t *testing.T) {
	msg := Key('a')
	if msg.Type != tea.KeyRunes || len(msg.Runes) != 1 || msg.Runes[0] != 'a' {
		t.Fatalf("expected rune key 'a', got %+v", msg)
	}
}

func TestEsc(t *testing.T) {
	msg := Esc()
	if msg.Type != tea.KeyEsc {
		t.Fatalf("expected Esc key, got %+v", msg)
	}
}

func TestSend(t *testing.T) {
	m, _ := Send(testModel{}, Key('a'))
	if m.value != 1 {
		t.Fatalf("expected value 1, got %d", m.value)
	}
}

func TestSendKey(t *testing.T) {
	m, _ := SendKey(testModel{}, 'a')
	if m.value != 1 {
		t.Fatalf("expected value 1, got %d", m.value)
	}
}
