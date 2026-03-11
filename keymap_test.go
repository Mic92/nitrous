package main

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

func TestParseKeyMap(t *testing.T) {
	tomlData := []byte(`
prev_room = ["alt+k", "ctrl+up"]
next_room = ["alt+j"]
`)

	km, err := ParseKeyMap(tomlData)
	if err != nil {
		t.Fatalf("ParseKeyMap: %v", err)
	}

	// Multiple keys per action should all match.
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}, Alt: true}
	if !key.Matches(msg, km.PrevRoom) {
		t.Error("PrevRoom should match alt+k after override")
	}
	msg = tea.KeyMsg{Type: tea.KeyCtrlUp}
	if !key.Matches(msg, km.PrevRoom) {
		t.Error("PrevRoom should also match ctrl+up (multiple keys)")
	}

	// Non-overridden bindings should keep defaults.
	msg = tea.KeyMsg{Type: tea.KeyCtrlC}
	if !key.Matches(msg, km.Quit) {
		t.Error("Quit should still match ctrl+c (not overridden)")
	}
}
