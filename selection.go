package main

import (
	"log"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// applySelectionHighlight overlays reverse-video on the selected region
// of the viewport output.
func (m *model) applySelectionHighlight(vp string) string {
	vpLines := strings.Split(vp, "\n")

	sw := m.sidebarWidth() + sidebarBorder
	titleHeight := lipgloss.Height(m.renderTitleBar())

	// Normalize selection coordinates to viewport-local.
	sy, ey := m.selectFrom[1]-titleHeight, m.selectTo[1]-titleHeight
	sx, ex := m.selectFrom[0]-sw, m.selectTo[0]-sw
	if sy > ey || (sy == ey && sx > ex) {
		sy, ey = ey, sy
		sx, ex = ex, sx
	}

	for y := range vpLines {
		if y < sy || y > ey {
			continue
		}
		plain := ansi.Strip(vpLines[y])
		width := stringColumnWidth(plain)
		var from, to int
		if sy == ey {
			from, to = sx, ex
		} else if y == sy {
			from, to = sx, width
		} else if y == ey {
			from, to = 0, ex
		} else {
			from, to = 0, width
		}
		mid := sliceByColumns(plain, from, to)
		if mid == "" {
			continue
		}
		// Rebuild line: prefix + highlighted + suffix (using plain text
		// to avoid ANSI nesting issues).
		pre := sliceByColumns(plain, 0, from)
		suf := sliceByColumns(plain, to, width)
		vpLines[y] = pre + m.theme.Selection.Render(mid) + suf
	}
	return strings.Join(vpLines, "\n")
}

// extractSelectedText extracts plain text from the viewport between
// the selection start and end screen coordinates.
func (m *model) extractSelectedText() string {
	content := m.viewport.View()
	vpLines := strings.Split(content, "\n")

	sw := m.sidebarWidth() + sidebarBorder
	titleHeight := lipgloss.Height(m.renderTitleBar())

	// Convert screen Y to viewport line index.
	startY := m.selectFrom[1] - titleHeight
	endY := m.selectTo[1] - titleHeight
	startX := m.selectFrom[0] - sw
	endX := m.selectTo[0] - sw

	// Normalize: start should be before end.
	if startY > endY || (startY == endY && startX > endX) {
		startY, endY = endY, startY
		startX, endX = endX, startX
	}

	if startY < 0 {
		startY = 0
		startX = 0
	}
	if endY >= len(vpLines) {
		endY = len(vpLines) - 1
	}
	if startY > endY {
		return ""
	}

	var selected []string
	for y := startY; y <= endY; y++ {
		if y < 0 || y >= len(vpLines) {
			continue
		}
		line := ansi.Strip(vpLines[y])
		width := stringColumnWidth(line)

		if startY == endY {
			// Single line selection.
			if s := sliceByColumns(line, startX, endX); s != "" {
				selected = append(selected, s)
			}
		} else if y == startY {
			selected = append(selected, sliceByColumns(line, startX, width))
		} else if y == endY {
			selected = append(selected, sliceByColumns(line, 0, endX))
		} else {
			selected = append(selected, line)
		}
	}

	return strings.Join(selected, "\n")
}

// sliceByColumns returns the substring of s spanning terminal columns
// [fromCol, toCol). Unlike s[from:to], this maps display columns to byte
// offsets so multi-byte runes (é, CJK, emoji) are not split mid-sequence.
//
// Out-of-range columns are clamped. If a column boundary falls inside a
// wide rune (e.g. column 1 of a 2-column CJK char), both fromCol and toCol
// snap right to the next rune boundary. Snapping the same direction ensures
// sliceByColumns(s, 0, k) + sliceByColumns(s, k, width) == s for all k,
// which applySelectionHighlight relies on to reassemble lines.
//
// Limitation: operates at the rune level using runewidth. Grapheme
// clusters that span multiple runes (flag emoji, ZWJ sequences, combining
// marks) may still be split at rune boundaries. This is acceptable for
// terminal selection — the output remains valid UTF-8 even if a cluster
// is visually broken.
func sliceByColumns(s string, fromCol, toCol int) string {
	if fromCol < 0 {
		fromCol = 0
	}
	if toCol <= fromCol {
		return ""
	}

	var (
		col      int  // current column position
		fromByte = -1 // byte offset where fromCol lands
		toByte   = len(s)
	)
	for i, r := range s {
		if fromByte < 0 && col >= fromCol {
			fromByte = i
		}
		if col >= toCol {
			toByte = i
			break
		}
		col += runewidth.RuneWidth(r)
	}
	if fromByte < 0 {
		// fromCol is at or past the end of the string.
		return ""
	}
	return s[fromByte:toByte]
}

// stringColumnWidth returns the display width of s in terminal columns.
func stringColumnWidth(s string) int {
	return runewidth.StringWidth(s)
}

// copyToClipboard copies text to the system clipboard.
// Tries wl-copy (Wayland), then xclip (X11), then OSC 52 escape sequence.
func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		// Try wl-copy first (Wayland).
		if path, err := exec.LookPath("wl-copy"); err == nil {
			cmd := exec.Command(path)
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err == nil {
				log.Printf("clipboard: copied %d bytes via wl-copy", len(text))
				return clipboardCopiedMsg{}
			}
		}

		// Try xclip (X11).
		if path, err := exec.LookPath("xclip"); err == nil {
			cmd := exec.Command(path, "-selection", "clipboard")
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err == nil {
				log.Printf("clipboard: copied %d bytes via xclip", len(text))
				return clipboardCopiedMsg{}
			}
		}

		// Try xsel (X11).
		if path, err := exec.LookPath("xsel"); err == nil {
			cmd := exec.Command(path, "--clipboard", "--input")
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err == nil {
				log.Printf("clipboard: copied %d bytes via xsel", len(text))
				return clipboardCopiedMsg{}
			}
		}

		// Fallback: OSC 52 (terminal clipboard escape sequence).
		//
		// We write directly to os.Stdout because bubbletea v1 has
		// no sanctioned way to emit raw OSC sequences: tea.Printf
		// appends a newline and is suppressed under altscreen.
		// Clipboard support landed in bubbletea v2 (beta).
		// TODO: switch to tea.SetClipboard once we move to v2.
		_, _ = os.Stdout.WriteString(osc52Sequence(text))
		log.Printf("clipboard: sent %d bytes via OSC 52", len(text))
		return clipboardCopiedMsg{}
	}
}

// osc52Sequence builds an OSC 52 escape sequence for setting the
// system clipboard. The payload is base64-encoded so that BEL, ESC
// or non-ASCII bytes in text cannot terminate the sequence early or
// confuse the terminal parser. Uses BEL as the terminator (broadly
// compatible; matches charmbracelet/x/ansi).
func osc52Sequence(text string) string {
	return ansi.SetSystemClipboard(text)
}

type clipboardCopiedMsg struct{}
