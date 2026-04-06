package main

import (
	"encoding/base64"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSliceByColumns(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		fromCol int
		toCol   int
		want    string
	}{
		// ASCII baseline.
		{"ascii full", "hello", 0, 5, "hello"},
		{"ascii mid", "hello", 1, 4, "ell"},
		{"ascii empty", "hello", 2, 2, ""},

		// Latin-1: é is 2 bytes but 1 column.
		{"latin1 full", "héllo", 0, 5, "héllo"},
		{"latin1 after accent", "héllo", 2, 5, "llo"},
		{"latin1 accent only", "héllo", 1, 2, "é"},

		// CJK: each char is 3 bytes, 2 columns.
		{"cjk full", "日本語", 0, 6, "日本語"},
		{"cjk middle", "abc日本語def", 3, 9, "日本語"},
		{"cjk prefix", "abc日本語def", 0, 3, "abc"},
		{"cjk suffix", "abc日本語def", 9, 12, "def"},
		{"cjk one char", "日本語", 2, 4, "本"},

		// Emoji: 👋 is 4 bytes, 2 columns.
		{"emoji wide", "👋hello", 0, 2, "👋"},
		{"emoji after", "👋hello", 2, 7, "hello"},
		{"emoji mid", "a👋b", 1, 3, "👋"},

		// Clamping: out-of-range columns.
		{"clamp negative", "hello", -3, 2, "he"},
		{"clamp overflow", "hello", 3, 99, "lo"},
		{"clamp both", "héllo", -1, 99, "héllo"},
		{"clamp cjk overflow", "日本", 0, 99, "日本"},

		// Edge: column lands inside a wide char — snap right to the next
		// rune boundary. Column 1 is the right half of 日 (cols 0–1).
		// Snapping right consistently ensures slice(s,0,k) + slice(s,k,w)
		// == s with no gaps or duplicated runes, which applySelectionHighlight
		// relies on when reassembling pre + mid + suf.
		{"mid-wide from", "日x", 1, 3, "x"},
		{"mid-wide to", "x日", 0, 2, "x日"},
		{"mid-wide reassembly pre", "a日b", 0, 2, "a日"},
		{"mid-wide reassembly suf", "a日b", 2, 4, "b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sliceByColumns(tt.input, tt.fromCol, tt.toCol)
			if got != tt.want {
				t.Errorf("sliceByColumns(%q, %d, %d) = %q, want %q",
					tt.input, tt.fromCol, tt.toCol, got, tt.want)
			}
		})
	}
}

func TestSliceByColumnsValidUTF8(t *testing.T) {
	// Regression: byte-slicing produced invalid UTF-8 (rendered as �).
	// Ensure every output is valid regardless of column alignment.
	inputs := []string{"日本語hello", "👋world", "héllo", "abc日本語def"}
	for _, s := range inputs {
		w := stringColumnWidth(s)
		for from := -1; from <= w+1; from++ {
			for to := from; to <= w+1; to++ {
				got := sliceByColumns(s, from, to)
				if !utf8.ValidString(got) {
					t.Errorf("sliceByColumns(%q, %d, %d) = %q: invalid UTF-8",
						s, from, to, got)
				}
			}
		}
	}
}

func TestOSC52Sequence(t *testing.T) {
	t.Run("plain ASCII round-trips through base64", func(t *testing.T) {
		input := "hello world"
		seq := osc52Sequence(input)

		// Expected exact format: ESC ] 52 ; c ; <base64> BEL
		// We use BEL (\a) as the terminator rather than ST (ESC \)
		// because it is a single byte and matches what
		// charmbracelet/x/ansi emits — broadly compatible with
		// xterm, kitty, iTerm2, alacritty, foot, wezterm.
		wantPrefix := "\x1b]52;c;"
		wantSuffix := "\a"

		if !strings.HasPrefix(seq, wantPrefix) {
			t.Fatalf("missing OSC 52 prefix: got %q", seq)
		}
		if !strings.HasSuffix(seq, wantSuffix) {
			t.Fatalf("missing BEL terminator: got %q", seq)
		}

		payload := seq[len(wantPrefix) : len(seq)-len(wantSuffix)]
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			t.Fatalf("payload is not valid base64: %v (payload=%q)", err, payload)
		}
		if string(decoded) != input {
			t.Errorf("round-trip mismatch: got %q want %q", decoded, input)
		}
	})

	t.Run("control bytes in input do not leak into the framing", func(t *testing.T) {
		// BEL and ESC inside the text would terminate the OSC
		// sequence early or start a new one if emitted raw.
		input := "foo\abar\x1bbaz"
		seq := osc52Sequence(input)

		// Strip the single allowed ESC (prefix) and single BEL
		// (terminator); the remainder must be pure base64.
		inner := strings.TrimPrefix(seq, "\x1b]52;c;")
		inner = strings.TrimSuffix(inner, "\a")

		if strings.ContainsAny(inner, "\a\x1b") {
			t.Errorf("raw control bytes leaked into payload: %q", inner)
		}

		decoded, err := base64.StdEncoding.DecodeString(inner)
		if err != nil {
			t.Fatalf("payload is not valid base64: %v", err)
		}
		if string(decoded) != input {
			t.Errorf("round-trip mismatch: got %q want %q", decoded, input)
		}
	})
}
