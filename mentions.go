package main

import (
	"regexp"
	"strings"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
	"github.com/charmbracelet/lipgloss"
)

// mentionPattern matches @username at word boundaries. The @ must be at the
// start of the string or preceded by whitespace (avoids matching emails like
// user@example.com). Usernames may contain word chars, hyphens, and dots.
var mentionPattern = regexp.MustCompile(`(?:^|(?:\s))@([\w.-]+)`)

// nostrMentionPattern matches nostr:npub1... and nostr:nprofile1... references in content.
var nostrMentionPattern = regexp.MustCompile(`nostr:((?:npub1|nprofile1)[a-z0-9]+)`)

// resolveMentions replaces @username patterns in content with nostr:npub1...
// references using the profiles map (pubkey hex → display name). Returns the
// rewritten content and a deduplicated list of mentioned hex pubkeys.
func resolveMentions(content string, profiles map[string]string) (string, []string) {
	// Build reverse map: lowercase display name → hex pubkey.
	nameToKey := make(map[string]string, len(profiles))
	for pk, name := range profiles {
		lower := strings.ToLower(name)
		if _, exists := nameToKey[lower]; !exists {
			nameToKey[lower] = pk
		}
	}

	seen := make(map[string]bool)
	var mentionedPKs []string

	result := mentionPattern.ReplaceAllStringFunc(content, func(match string) string {
		// Preserve leading whitespace if present.
		prefix := ""
		trimmed := match
		if strings.HasPrefix(match, " ") || strings.HasPrefix(match, "\t") || strings.HasPrefix(match, "\n") {
			prefix = match[:1]
			trimmed = match[1:]
		}

		username := strings.TrimPrefix(trimmed, "@")
		hexPK, ok := nameToKey[strings.ToLower(username)]
		if !ok {
			return match // unknown user, leave as-is
		}

		pk, err := nostr.PubKeyFromHex(hexPK)
		if err != nil {
			return match
		}

		npub := nip19.EncodeNpub(pk)
		if !seen[hexPK] {
			seen[hexPK] = true
			mentionedPKs = append(mentionedPKs, hexPK)
		}

		return prefix + "nostr:" + npub
	})

	return result, mentionedPKs
}

// renderMentions replaces nostr:npub1... and nostr:nprofile1... references in
// content with @displayname for readability. Falls back to @npub1<8chars>...
// if the pubkey is not in the profiles map. Returns the rewritten content and
// a list of resolved @displayname strings (for post-render styling).
func renderMentions(content string, profiles map[string]string) (string, []string) {
	seen := make(map[string]bool)
	var resolved []string

	out := nostrMentionPattern.ReplaceAllStringFunc(content, func(match string) string {
		// Extract the bech32 part after "nostr:".
		bech32str := strings.TrimPrefix(match, "nostr:")

		prefix, value, err := nip19.Decode(bech32str)
		if err != nil {
			return match
		}

		var hexPK string
		switch prefix {
		case "npub":
			pk, ok := value.(nostr.PubKey)
			if !ok {
				return match
			}
			hexPK = pk.Hex()
		case "nprofile":
			pp, ok := value.(nostr.ProfilePointer)
			if !ok {
				return match
			}
			hexPK = pp.PublicKey.Hex()
		default:
			return match
		}

		if name, found := profiles[hexPK]; found {
			mention := "@" + name
			if !seen[mention] {
				seen[mention] = true
				resolved = append(resolved, mention)
			}
			return mention
		}

		// Truncated fallback: @npub1<8chars>... or @nprofile1<8chars>...
		if len(bech32str) > 13 {
			return "@" + bech32str[:13] + "..."
		}
		return "@" + bech32str
	})

	return out, resolved
}

// styleMentions applies ANSI styling to resolved @displayname strings in
// already-rendered content. Call this after markdown rendering.
func styleMentions(content string, mentions []string, style lipgloss.Style) string {
	for _, m := range mentions {
		styled := style.Render(m)
		content = strings.ReplaceAll(content, m, styled)
	}
	return content
}
