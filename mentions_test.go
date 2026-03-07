package main

import (
	"strings"
	"testing"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
)

func testProfiles() map[string]string {
	return map[string]string{
		"aaaa000000000000000000000000000000000000000000000000000000000001": "alice",
		"bbbb000000000000000000000000000000000000000000000000000000000002": "bob",
	}
}

func TestResolveMentions_SingleMention(t *testing.T) {
	profiles := testProfiles()
	content, pks := resolveMentions("hello @alice", profiles)

	alicePK, _ := nostr.PubKeyFromHex("aaaa000000000000000000000000000000000000000000000000000000000001")
	npub := nip19.EncodeNpub(alicePK)
	if !strings.Contains(content, "nostr:"+npub) {
		t.Errorf("expected nostr:%s in content, got %q", npub, content)
	}
	if len(pks) != 1 || pks[0] != alicePK.Hex() {
		t.Errorf("expected [%s], got %v", alicePK.Hex(), pks)
	}
}

func TestResolveMentions_MultipleMentions(t *testing.T) {
	profiles := testProfiles()
	content, pks := resolveMentions("hey @alice and @bob", profiles)

	if !strings.Contains(content, "nostr:npub1") {
		t.Errorf("expected nostr:npub references in content, got %q", content)
	}
	if strings.Contains(content, "@alice") || strings.Contains(content, "@bob") {
		t.Errorf("expected @mentions to be replaced, got %q", content)
	}
	if len(pks) != 2 {
		t.Errorf("expected 2 pubkeys, got %d", len(pks))
	}
}

func TestResolveMentions_UnknownUser(t *testing.T) {
	profiles := testProfiles()
	content, pks := resolveMentions("hello @unknown", profiles)

	if content != "hello @unknown" {
		t.Errorf("expected unchanged content, got %q", content)
	}
	if len(pks) != 0 {
		t.Errorf("expected no pubkeys, got %v", pks)
	}
}

func TestResolveMentions_CaseInsensitive(t *testing.T) {
	profiles := testProfiles()
	content, pks := resolveMentions("hello @Alice", profiles)

	if strings.Contains(content, "@Alice") {
		t.Errorf("expected @Alice to be replaced, got %q", content)
	}
	if len(pks) != 1 {
		t.Errorf("expected 1 pubkey, got %d", len(pks))
	}
}

func TestResolveMentions_DuplicateMentions(t *testing.T) {
	profiles := testProfiles()
	_, pks := resolveMentions("@alice hello @alice", profiles)

	if len(pks) != 1 {
		t.Errorf("expected 1 deduplicated pubkey, got %d: %v", len(pks), pks)
	}
}

func TestResolveMentions_EmailIgnored(t *testing.T) {
	profiles := map[string]string{
		"aaaa000000000000000000000000000000000000000000000000000000000001": "example",
	}
	content, pks := resolveMentions("email user@example.com", profiles)

	if content != "email user@example.com" {
		t.Errorf("expected unchanged content, got %q", content)
	}
	if len(pks) != 0 {
		t.Errorf("expected no pubkeys for email, got %v", pks)
	}
}

func TestResolveMentions_AtStartOfString(t *testing.T) {
	profiles := testProfiles()
	content, pks := resolveMentions("@alice how are you?", profiles)

	if strings.Contains(content, "@alice") {
		t.Errorf("expected @alice to be replaced, got %q", content)
	}
	if len(pks) != 1 {
		t.Errorf("expected 1 pubkey, got %d", len(pks))
	}
}

func TestResolveMentions_HyphenatedName(t *testing.T) {
	profiles := map[string]string{
		"aaaa000000000000000000000000000000000000000000000000000000000001": "pinpox",
		"bbbb000000000000000000000000000000000000000000000000000000000002": "pinpox-mobile",
	}

	// Should resolve pinpox-mobile, not pinpox
	content, pks := resolveMentions("hey @pinpox-mobile", profiles)
	if strings.Contains(content, "@pinpox-mobile") {
		t.Errorf("expected @pinpox-mobile to be replaced, got %q", content)
	}
	if len(pks) != 1 || pks[0] != "bbbb000000000000000000000000000000000000000000000000000000000002" {
		t.Errorf("expected pinpox-mobile pubkey, got %v", pks)
	}

	// Should resolve pinpox only
	content2, pks2 := resolveMentions("hey @pinpox", profiles)
	if strings.Contains(content2, "@pinpox") && !strings.Contains(content2, "nostr:") {
		t.Errorf("expected @pinpox to be replaced, got %q", content2)
	}
	if len(pks2) != 1 || pks2[0] != "aaaa000000000000000000000000000000000000000000000000000000000001" {
		t.Errorf("expected pinpox pubkey, got %v", pks2)
	}
}

func TestResolveMentions_HyphenatedNameUnknown(t *testing.T) {
	profiles := map[string]string{
		"aaaa000000000000000000000000000000000000000000000000000000000001": "pinpox",
	}

	// @pinpox-mobile doesn't exist — should NOT partially match @pinpox
	content, pks := resolveMentions("hey @pinpox-mobile", profiles)
	if content != "hey @pinpox-mobile" {
		t.Errorf("expected unchanged content, got %q", content)
	}
	if len(pks) != 0 {
		t.Errorf("expected no pubkeys, got %v", pks)
	}
}

func TestRenderMentions_KnownNpub(t *testing.T) {
	profiles := testProfiles()
	alicePK, _ := nostr.PubKeyFromHex("aaaa000000000000000000000000000000000000000000000000000000000001")
	npub := nip19.EncodeNpub(alicePK)

	content, resolved := renderMentions("hello nostr:"+npub, profiles)
	if !strings.Contains(content, "@alice") {
		t.Errorf("expected @alice in rendered content, got %q", content)
	}
	if strings.Contains(content, "nostr:") {
		t.Errorf("expected nostr: reference to be replaced, got %q", content)
	}
	if len(resolved) != 1 || resolved[0] != "@alice" {
		t.Errorf("expected resolved=[@alice], got %v", resolved)
	}
}

func TestRenderMentions_UnknownNpub(t *testing.T) {
	profiles := testProfiles()
	unknownPK, _ := nostr.PubKeyFromHex("cccc000000000000000000000000000000000000000000000000000000000003")
	npub := nip19.EncodeNpub(unknownPK)

	content, resolved := renderMentions("hello nostr:"+npub, profiles)
	if strings.Contains(content, "nostr:") {
		t.Errorf("expected nostr: reference to be replaced, got %q", content)
	}
	if !strings.Contains(content, "@npub1") {
		t.Errorf("expected truncated @npub1... fallback, got %q", content)
	}
	if !strings.Contains(content, "...") {
		t.Errorf("expected ... truncation, got %q", content)
	}
	if len(resolved) != 0 {
		t.Errorf("expected no resolved mentions for unknown npub, got %v", resolved)
	}
}

func TestRenderMentions_KnownNprofile(t *testing.T) {
	profiles := testProfiles()
	alicePK, _ := nostr.PubKeyFromHex("aaaa000000000000000000000000000000000000000000000000000000000001")
	nprofile := nip19.EncodeNprofile(alicePK, []string{"wss://relay.example.com"})

	content, resolved := renderMentions("hello nostr:"+nprofile, profiles)
	if !strings.Contains(content, "@alice") {
		t.Errorf("expected @alice in rendered content, got %q", content)
	}
	if strings.Contains(content, "nostr:") {
		t.Errorf("expected nostr: reference to be replaced, got %q", content)
	}
	if len(resolved) != 1 || resolved[0] != "@alice" {
		t.Errorf("expected resolved=[@alice], got %v", resolved)
	}
}

func TestRenderMentions_UnknownNprofile(t *testing.T) {
	profiles := testProfiles()
	unknownPK, _ := nostr.PubKeyFromHex("cccc000000000000000000000000000000000000000000000000000000000003")
	nprofile := nip19.EncodeNprofile(unknownPK, nil)

	content, resolved := renderMentions("hello nostr:"+nprofile, profiles)
	if strings.Contains(content, "nostr:") {
		t.Errorf("expected nostr: reference to be replaced, got %q", content)
	}
	if !strings.Contains(content, "@nprofile1") {
		t.Errorf("expected truncated @nprofile1... fallback, got %q", content)
	}
	if len(resolved) != 0 {
		t.Errorf("expected no resolved mentions for unknown nprofile, got %v", resolved)
	}
}

func TestRenderMentions_MultipleNpubs(t *testing.T) {
	profiles := testProfiles()
	alicePK, _ := nostr.PubKeyFromHex("aaaa000000000000000000000000000000000000000000000000000000000001")
	bobPK, _ := nostr.PubKeyFromHex("bbbb000000000000000000000000000000000000000000000000000000000002")
	npubAlice := nip19.EncodeNpub(alicePK)
	npubBob := nip19.EncodeNpub(bobPK)

	content, resolved := renderMentions("nostr:"+npubAlice+" and nostr:"+npubBob, profiles)
	if !strings.Contains(content, "@alice") {
		t.Errorf("expected @alice, got %q", content)
	}
	if !strings.Contains(content, "@bob") {
		t.Errorf("expected @bob, got %q", content)
	}
	if len(resolved) != 2 {
		t.Errorf("expected 2 resolved mentions, got %v", resolved)
	}
}
