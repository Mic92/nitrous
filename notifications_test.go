package main

import (
	"testing"

	"fiatjaf.com/nostr"
	"fiatjaf.com/nostr/nip19"
)

func boolPtr(b bool) *bool { return &b }

func TestDesktopNotificationsEnabled(t *testing.T) {
	tests := []struct {
		name string
		val  *bool
		want bool
	}{
		{"nil defaults to true", nil, true},
		{"explicit true", boolPtr(true), true},
		{"explicit false", boolPtr(false), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{NotificationsDesktop: tt.val}
			if got := cfg.DesktopNotificationsEnabled(); got != tt.want {
				t.Errorf("DesktopNotificationsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBellNotificationsEnabled(t *testing.T) {
	tests := []struct {
		name string
		val  *bool
		want bool
	}{
		{"nil defaults to true", nil, true},
		{"explicit true", boolPtr(true), true},
		{"explicit false", boolPtr(false), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{NotificationsBell: tt.val}
			if got := cfg.BellNotificationsEnabled(); got != tt.want {
				t.Errorf("BellNotificationsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMentionsUser_PTag(t *testing.T) {
	userPK := "aaaa000000000000000000000000000000000000000000000000000000000001"
	tags := nostr.Tags{{"p", userPK}}
	if !mentionsUser("hello", tags, userPK) {
		t.Error("expected mentionsUser to return true for p tag match")
	}
}

func TestMentionsUser_PTagNoMatch(t *testing.T) {
	userPK := "aaaa000000000000000000000000000000000000000000000000000000000001"
	otherPK := "bbbb000000000000000000000000000000000000000000000000000000000002"
	tags := nostr.Tags{{"p", otherPK}}
	if mentionsUser("hello", tags, userPK) {
		t.Error("expected mentionsUser to return false for non-matching p tag")
	}
}

func TestMentionsUser_NpubInContent(t *testing.T) {
	userPK := "aaaa000000000000000000000000000000000000000000000000000000000001"
	pk, _ := nostr.PubKeyFromHex(userPK)
	npub := nip19.EncodeNpub(pk)
	content := "hey nostr:" + npub + " check this"
	if !mentionsUser(content, nil, userPK) {
		t.Error("expected mentionsUser to return true for npub in content")
	}
}

func TestMentionsUser_NprofileInContent(t *testing.T) {
	userPK := "aaaa000000000000000000000000000000000000000000000000000000000001"
	pk, _ := nostr.PubKeyFromHex(userPK)
	nprofile := nip19.EncodeNprofile(pk, []string{"wss://relay.example.com"})
	content := "hey nostr:" + nprofile + " check this"
	if !mentionsUser(content, nil, userPK) {
		t.Error("expected mentionsUser to return true for nprofile in content")
	}
}

func TestMentionsUser_NoMatch(t *testing.T) {
	userPK := "aaaa000000000000000000000000000000000000000000000000000000000001"
	otherPK := "bbbb000000000000000000000000000000000000000000000000000000000002"
	pk, _ := nostr.PubKeyFromHex(otherPK)
	npub := nip19.EncodeNpub(pk)
	content := "hey nostr:" + npub
	if mentionsUser(content, nil, userPK) {
		t.Error("expected mentionsUser to return false for non-matching npub")
	}
}

func TestMentionsUser_EmptyContent(t *testing.T) {
	userPK := "aaaa000000000000000000000000000000000000000000000000000000000001"
	if mentionsUser("", nil, userPK) {
		t.Error("expected mentionsUser to return false for empty content")
	}
}
