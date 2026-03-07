package main

import (
	"testing"
	"time"

	"fiatjaf.com/nostr"
)

func TestAppendMessage(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		msgs := appendMessage(nil, ChatMessage{Content: "a", Timestamp: 100}, 10)
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
	})

	t.Run("timestamp ordering - insert at end", func(t *testing.T) {
		msgs := []ChatMessage{
			{Content: "first", Timestamp: 100},
			{Content: "second", Timestamp: 200},
		}
		msgs = appendMessage(msgs, ChatMessage{Content: "third", Timestamp: 300}, 10)
		if msgs[2].Content != "third" {
			t.Errorf("expected 'third' at end, got %q", msgs[2].Content)
		}
	})

	t.Run("timestamp ordering - insert at beginning", func(t *testing.T) {
		msgs := []ChatMessage{
			{Content: "second", Timestamp: 200},
			{Content: "third", Timestamp: 300},
		}
		msgs = appendMessage(msgs, ChatMessage{Content: "first", Timestamp: 100}, 10)
		if msgs[0].Content != "first" {
			t.Errorf("expected 'first' at beginning, got %q", msgs[0].Content)
		}
	})

	t.Run("timestamp ordering - insert in middle", func(t *testing.T) {
		msgs := []ChatMessage{
			{Content: "first", Timestamp: 100},
			{Content: "third", Timestamp: 300},
		}
		msgs = appendMessage(msgs, ChatMessage{Content: "second", Timestamp: 200}, 10)
		if msgs[1].Content != "second" {
			t.Errorf("expected 'second' in middle, got %q", msgs[1].Content)
		}
	})

	t.Run("max cap enforced", func(t *testing.T) {
		var msgs []ChatMessage
		for i := 0; i < 15; i++ {
			msgs = appendMessage(msgs, ChatMessage{
				Content:   "msg",
				Timestamp: nostr.Timestamp(i),
			}, 10)
		}
		if len(msgs) != 10 {
			t.Errorf("expected max 10 messages, got %d", len(msgs))
		}
		// Oldest messages should be trimmed.
		if msgs[0].Timestamp != 5 {
			t.Errorf("expected oldest timestamp 5, got %d", msgs[0].Timestamp)
		}
	})

	t.Run("equal timestamps preserve insertion order", func(t *testing.T) {
		msgs := []ChatMessage{
			{Content: "a", Timestamp: 100},
		}
		msgs = appendMessage(msgs, ChatMessage{Content: "b", Timestamp: 100}, 10)
		if len(msgs) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(msgs))
		}
		if msgs[1].Content != "b" {
			t.Errorf("expected 'b' after 'a' for equal timestamps, got %q", msgs[1].Content)
		}
	})
}

func newTestModel(channels int, groups int, dmPeers int) *model {
	m := &model{
		activeItem: 0,
	}
	for i := 0; i < channels; i++ {
		m.sidebar = append(m.sidebar, ChannelItem{Channel: Channel{ID: "ch" + string(rune('0'+i)), Name: "chan" + string(rune('0'+i))}})
	}
	for i := 0; i < groups; i++ {
		m.sidebar = append(m.sidebar, GroupItem{Group: Group{RelayURL: "wss://r", GroupID: "g" + string(rune('0'+i)), Name: "grp" + string(rune('0'+i))}})
	}
	for i := 0; i < dmPeers; i++ {
		m.sidebar = append(m.sidebar, DMItem{PubKey: "pk" + string(rune('0'+i)), Name: "pk" + string(rune('0'+i))})
	}
	return m
}

func TestIsChannelSelected(t *testing.T) {
	m := newTestModel(2, 2, 2) // channels: 0-1, groups: 2-3, DMs: 4-5

	m.activeItem = 0
	if !m.isChannelSelected() {
		t.Error("activeItem=0 should be a channel")
	}

	m.activeItem = 1
	if !m.isChannelSelected() {
		t.Error("activeItem=1 should be a channel")
	}

	m.activeItem = 2
	if m.isChannelSelected() {
		t.Error("activeItem=2 should NOT be a channel")
	}

	m.activeItem = 4
	if m.isChannelSelected() {
		t.Error("activeItem=4 should NOT be a channel")
	}
}

func TestIsGroupSelected(t *testing.T) {
	m := newTestModel(2, 2, 2)

	m.activeItem = 0
	if m.isGroupSelected() {
		t.Error("activeItem=0 should NOT be a group")
	}

	m.activeItem = 2
	if !m.isGroupSelected() {
		t.Error("activeItem=2 should be a group")
	}

	m.activeItem = 3
	if !m.isGroupSelected() {
		t.Error("activeItem=3 should be a group")
	}

	m.activeItem = 4
	if m.isGroupSelected() {
		t.Error("activeItem=4 should NOT be a group")
	}
}

func TestIsDMSelected(t *testing.T) {
	m := newTestModel(2, 2, 2)

	m.activeItem = 0
	if m.isDMSelected() {
		t.Error("activeItem=0 should NOT be a DM")
	}

	m.activeItem = 3
	if m.isDMSelected() {
		t.Error("activeItem=3 should NOT be a DM")
	}

	m.activeItem = 4
	if !m.isDMSelected() {
		t.Error("activeItem=4 should be a DM")
	}

	m.activeItem = 5
	if !m.isDMSelected() {
		t.Error("activeItem=5 should be a DM")
	}
}

func TestActiveChannelID(t *testing.T) {
	m := newTestModel(2, 2, 2)

	m.activeItem = 0
	if id := m.activeChannelID(); id != "ch0" {
		t.Errorf("expected ch0, got %q", id)
	}

	m.activeItem = 1
	if id := m.activeChannelID(); id != "ch1" {
		t.Errorf("expected ch1, got %q", id)
	}

	m.activeItem = 2 // group
	if id := m.activeChannelID(); id != "" {
		t.Errorf("expected empty for group, got %q", id)
	}

	m.activeItem = 4 // DM
	if id := m.activeChannelID(); id != "" {
		t.Errorf("expected empty for DM, got %q", id)
	}
}

func TestActiveGroupKey(t *testing.T) {
	m := newTestModel(2, 2, 2)

	m.activeItem = 2
	expected0 := groupKey("wss://r", "g0")
	if gk := m.activeGroupKey(); gk != expected0 {
		t.Errorf("expected %q, got %q", expected0, gk)
	}

	m.activeItem = 3
	expected1 := groupKey("wss://r", "g1")
	if gk := m.activeGroupKey(); gk != expected1 {
		t.Errorf("expected %q, got %q", expected1, gk)
	}

	m.activeItem = 0 // channel
	if gk := m.activeGroupKey(); gk != "" {
		t.Errorf("expected empty for channel, got %q", gk)
	}
}

func TestActiveDMPeerPK(t *testing.T) {
	m := newTestModel(2, 2, 2)

	m.activeItem = 4
	if pk := m.activeDMPeerPK(); pk != "pk0" {
		t.Errorf("expected pk0, got %q", pk)
	}

	m.activeItem = 5
	if pk := m.activeDMPeerPK(); pk != "pk1" {
		t.Errorf("expected pk1, got %q", pk)
	}

	m.activeItem = 3 // group
	if pk := m.activeDMPeerPK(); pk != "" {
		t.Errorf("expected empty for group, got %q", pk)
	}
}

func TestSidebarTotal(t *testing.T) {
	tests := []struct {
		channels, groups, dms int
		want                  int
	}{
		{0, 0, 0, 0},
		{3, 0, 0, 3},
		{0, 2, 0, 2},
		{0, 0, 4, 4},
		{2, 3, 5, 10},
	}
	for _, tt := range tests {
		m := newTestModel(tt.channels, tt.groups, tt.dms)
		got := m.sidebarTotal()
		if got != tt.want {
			t.Errorf("sidebarTotal(%d, %d, %d) = %d, want %d",
				tt.channels, tt.groups, tt.dms, got, tt.want)
		}
	}
}

func TestResolveAuthor(t *testing.T) {
	m := &model{
		profiles: map[string]string{
			"abcdef1234567890": "Alice",
		},
	}

	t.Run("cached profile name", func(t *testing.T) {
		got := m.resolveAuthor("abcdef1234567890")
		if got != "Alice" {
			t.Errorf("expected 'Alice', got %q", got)
		}
	})

	t.Run("fallback to shortPK", func(t *testing.T) {
		got := m.resolveAuthor("ffffffffffffffff0000000000000000ffffffffffffffff0000000000000000")
		want := "ffffffff"
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})

	t.Run("empty key", func(t *testing.T) {
		got := m.resolveAuthor("")
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})
}

func TestBoundaryConditions(t *testing.T) {
	t.Run("no channels", func(t *testing.T) {
		m := newTestModel(0, 2, 1)
		m.activeItem = 0
		if m.isChannelSelected() {
			t.Error("should not be channel when no channels exist")
		}
		if !m.isGroupSelected() {
			t.Error("activeItem=0 with 0 channels should be group")
		}
	})

	t.Run("no groups", func(t *testing.T) {
		m := newTestModel(2, 0, 1)
		m.activeItem = 2
		if m.isGroupSelected() {
			t.Error("should not be group when no groups exist")
		}
		if !m.isDMSelected() {
			t.Error("activeItem=2 with 0 groups should be DM")
		}
	})

	t.Run("only DMs", func(t *testing.T) {
		m := newTestModel(0, 0, 3)
		m.activeItem = 0
		if !m.isDMSelected() {
			t.Error("activeItem=0 with only DMs should be DM")
		}
		if pk := m.activeDMPeerPK(); pk != "pk0" {
			t.Errorf("expected DM peer pk0, got %q", pk)
		}
	})
}

func TestSidebarHelpers(t *testing.T) {
	t.Run("channelCount/groupCount/dmCount", func(t *testing.T) {
		m := newTestModel(2, 3, 4)
		if got := m.channelCount(); got != 2 {
			t.Errorf("channelCount() = %d, want 2", got)
		}
		if got := m.groupCount(); got != 3 {
			t.Errorf("groupCount() = %d, want 3", got)
		}
		if got := m.dmCount(); got != 4 {
			t.Errorf("dmCount() = %d, want 4", got)
		}
	})

	t.Run("channelEndIdx/groupEndIdx", func(t *testing.T) {
		m := newTestModel(2, 3, 4)
		if got := m.channelEndIdx(); got != 2 {
			t.Errorf("channelEndIdx() = %d, want 2", got)
		}
		if got := m.groupEndIdx(); got != 5 {
			t.Errorf("groupEndIdx() = %d, want 5", got)
		}
	})

	t.Run("appendChannelItem inserts in channel section", func(t *testing.T) {
		m := newTestModel(1, 1, 1)
		idx := m.appendChannelItem(Channel{ID: "new", Name: "new-chan"})
		if idx != 1 {
			t.Errorf("appendChannelItem returned %d, want 1", idx)
		}
		if m.channelCount() != 2 {
			t.Errorf("channelCount() = %d, want 2", m.channelCount())
		}
		if m.sidebarTotal() != 4 {
			t.Errorf("sidebarTotal() = %d, want 4", m.sidebarTotal())
		}
		// Verify order: channels, groups, DMs
		if m.sidebar[0].Kind() != SidebarChannel || m.sidebar[1].Kind() != SidebarChannel {
			t.Error("channels should be at positions 0-1")
		}
		if m.sidebar[2].Kind() != SidebarGroup {
			t.Error("group should be at position 2")
		}
		if m.sidebar[3].Kind() != SidebarDM {
			t.Error("DM should be at position 3")
		}
	})

	t.Run("appendGroupItem inserts in group section", func(t *testing.T) {
		m := newTestModel(1, 1, 1)
		idx := m.appendGroupItem(Group{RelayURL: "wss://r", GroupID: "new", Name: "new-grp"})
		if idx != 2 {
			t.Errorf("appendGroupItem returned %d, want 2", idx)
		}
		if m.groupCount() != 2 {
			t.Errorf("groupCount() = %d, want 2", m.groupCount())
		}
	})

	t.Run("appendDMItem appends at end", func(t *testing.T) {
		m := newTestModel(1, 1, 1)
		idx := m.appendDMItem("newpk", "New Peer")
		if idx != 3 {
			t.Errorf("appendDMItem returned %d, want 3", idx)
		}
		if m.dmCount() != 2 {
			t.Errorf("dmCount() = %d, want 2", m.dmCount())
		}
	})

	t.Run("findChannelIdx", func(t *testing.T) {
		m := newTestModel(2, 1, 1)
		if idx := m.findChannelIdx("ch1"); idx != 1 {
			t.Errorf("findChannelIdx(ch1) = %d, want 1", idx)
		}
		if idx := m.findChannelIdx("nonexistent"); idx != -1 {
			t.Errorf("findChannelIdx(nonexistent) = %d, want -1", idx)
		}
	})

	t.Run("findGroupIdx", func(t *testing.T) {
		m := newTestModel(1, 2, 1)
		if idx := m.findGroupIdx("wss://r", "g1"); idx != 2 {
			t.Errorf("findGroupIdx(g1) = %d, want 2", idx)
		}
		if idx := m.findGroupIdx("wss://r", "nonexistent"); idx != -1 {
			t.Errorf("findGroupIdx(nonexistent) = %d, want -1", idx)
		}
	})

	t.Run("findDMPeerIdx", func(t *testing.T) {
		m := newTestModel(1, 1, 2)
		if idx := m.findDMPeerIdx("pk0"); idx != 2 {
			t.Errorf("findDMPeerIdx(pk0) = %d, want 2", idx)
		}
		if idx := m.findDMPeerIdx("nonexistent"); idx != -1 {
			t.Errorf("findDMPeerIdx(nonexistent) = %d, want -1", idx)
		}
	})

	t.Run("containsDMPeer", func(t *testing.T) {
		m := newTestModel(0, 0, 2)
		if !m.containsDMPeer("pk0") {
			t.Error("containsDMPeer(pk0) should be true")
		}
		if m.containsDMPeer("nonexistent") {
			t.Error("containsDMPeer(nonexistent) should be false")
		}
	})

	t.Run("removeSidebarItem", func(t *testing.T) {
		m := newTestModel(2, 2, 2)
		m.removeSidebarItem(0) // Remove first channel
		if m.sidebarTotal() != 5 {
			t.Errorf("sidebarTotal() = %d, want 5", m.sidebarTotal())
		}
		if m.channelCount() != 1 {
			t.Errorf("channelCount() = %d, want 1", m.channelCount())
		}
	})

	t.Run("allChannels", func(t *testing.T) {
		m := newTestModel(2, 1, 1)
		channels := m.allChannels()
		if len(channels) != 2 {
			t.Errorf("allChannels() returned %d, want 2", len(channels))
		}
	})

	t.Run("allGroups", func(t *testing.T) {
		m := newTestModel(1, 3, 1)
		groups := m.allGroups()
		if len(groups) != 3 {
			t.Errorf("allGroups() returned %d, want 3", len(groups))
		}
	})

	t.Run("allDMPeers", func(t *testing.T) {
		m := newTestModel(1, 1, 2)
		peers := m.allDMPeers()
		if len(peers) != 2 {
			t.Errorf("allDMPeers() returned %d, want 2", len(peers))
		}
	})

	t.Run("updateDMItemName", func(t *testing.T) {
		m := newTestModel(0, 0, 1)
		m.updateDMItemName("pk0", "Alice")
		di := m.sidebar[0].(DMItem)
		if di.Name != "Alice" {
			t.Errorf("expected Alice, got %q", di.Name)
		}
	})

	t.Run("replaceChannels", func(t *testing.T) {
		m := newTestModel(2, 1, 1)
		m.replaceChannels([]Channel{{ID: "new1", Name: "New1"}, {ID: "new2", Name: "New2"}, {ID: "new3", Name: "New3"}})
		if m.channelCount() != 3 {
			t.Errorf("channelCount() = %d, want 3", m.channelCount())
		}
		if m.groupCount() != 1 {
			t.Errorf("groupCount() = %d, want 1", m.groupCount())
		}
		if m.dmCount() != 1 {
			t.Errorf("dmCount() = %d, want 1", m.dmCount())
		}
	})

	t.Run("replaceGroups", func(t *testing.T) {
		m := newTestModel(1, 2, 1)
		m.replaceGroups([]Group{{RelayURL: "wss://r", GroupID: "new1", Name: "New1"}})
		if m.groupCount() != 1 {
			t.Errorf("groupCount() = %d, want 1", m.groupCount())
		}
		if m.channelCount() != 1 {
			t.Errorf("channelCount() = %d, want 1", m.channelCount())
		}
		if m.dmCount() != 1 {
			t.Errorf("dmCount() = %d, want 1", m.dmCount())
		}
	})

	t.Run("replaceDMPeers", func(t *testing.T) {
		m := newTestModel(1, 1, 2)
		m.replaceDMPeers([]Contact{{PubKey: "newpk", Name: "NewPeer"}})
		if m.dmCount() != 1 {
			t.Errorf("dmCount() = %d, want 1", m.dmCount())
		}
		if m.channelCount() != 1 {
			t.Errorf("channelCount() = %d, want 1", m.channelCount())
		}
		if m.groupCount() != 1 {
			t.Errorf("groupCount() = %d, want 1", m.groupCount())
		}
	})
}

// TestContactListWipeOnRestart reproduces the bug where restarting nitrous
// causes contacts to be silently dropped from the NIP-51 contacts list.
//
// Scenario:
//  1. NIP-51 fetch loads contacts A, B, C into the sidebar.
//  2. A replayed DM arrives from peer D (timestamp <= dmSeenAtStart).
//     The replay guard skips adding D to the sidebar — correct behaviour.
//  3. A genuinely new DM arrives from peer E (timestamp > dmSeenAtStart).
//     E is added to the sidebar and triggers a contacts list publish.
//  4. BUG: the published list is built solely from allDMPeers() (the sidebar),
//     which now contains [A, B, C, E]. If any of A/B/C had been filtered out
//     of the sidebar for any reason (e.g. replaceDMPeers race), they would be
//     permanently lost.
//
// More critically: if a contact was on the relay's NIP-51 list but never
// appeared in the sidebar (e.g. due to DM replay guard filtering), publishing
// via contactsFromModel(allDMPeers(), profiles) will drop that contact.
func TestContactListWipeOnRestart(t *testing.T) {
	// Simulate startup: dmSeenAtStart is set to 1000 (the last-seen timestamp).
	m := &model{
		activeItem:      0,
		msgs:            make(map[string][]ChatMessage),
		unread:          make(map[string]bool),
		localDMEchoes:   make(map[string]time.Time),
		profiles:        make(map[string]string),
		profilePending:  make(map[string]bool),
		seenEvents:      make(map[string]time.Time),
		seenEventsClean: time.Now(),
		dmSeenAtStart:   1000,
		lastDMSeen:      1000,
		cfg:             Config{MaxMessages: 100},
	}

	// Step 1: NIP-51 fetch returns contacts: alice, bob, charlie.
	m.handleNIP51ListsFetched(nip51ListsFetchedMsg{
		contacts: []Contact{
			{PubKey: "pk_alice", Name: "alice"},
			{PubKey: "pk_bob", Name: "bob"},
			{PubKey: "pk_charlie", Name: "charlie"},
		},
		contactsTS: 999,
	})

	// Verify all three are in the sidebar.
	if got := m.dmCount(); got != 3 {
		t.Fatalf("after NIP-51 fetch: dmCount() = %d, want 3", got)
	}

	// Step 2: A replayed DM arrives from a peer (pk_dave) that was previously
	// removed by the user. Timestamp <= dmSeenAtStart, so replay guard kicks in.
	m.handleDMEvent(dmEventMsg{
		PubKey:    "pk_dave",
		Content:   "old message",
		Timestamp: 900, // <= dmSeenAtStart (1000)
		EventID:   "evt_dave_old",
		Author:    "dave",
	})

	// pk_dave should NOT be in the sidebar (replay guard).
	if m.containsDMPeer("pk_dave") {
		t.Fatal("pk_dave should not be in sidebar after replayed DM")
	}

	// Step 3: Now simulate that pk_bob was removed from the sidebar.
	// This can happen if replaceDMPeers is called again with a subset,
	// or through other sidebar manipulation. For this test, we directly
	// remove bob to simulate the race condition.
	for i, it := range m.sidebar {
		if di, ok := it.(DMItem); ok && di.PubKey == "pk_bob" {
			m.sidebar = append(m.sidebar[:i], m.sidebar[i+1:]...)
			break
		}
	}

	// Step 4: A genuinely new DM arrives from pk_eve (timestamp > dmSeenAtStart).
	// This triggers a contacts list publish.
	m.handleDMEvent(dmEventMsg{
		PubKey:    "pk_eve",
		Content:   "hello!",
		Timestamp: 1500, // > dmSeenAtStart (1000)
		EventID:   "evt_eve_new",
		Author:    "eve",
	})

	// pk_eve should now be in the sidebar.
	if !m.containsDMPeer("pk_eve") {
		t.Fatal("pk_eve should be in sidebar after new DM")
	}

	// Step 5: Check what contactsFromModel would publish.
	// On master, this only uses allDMPeers() — the sidebar contents.
	published := contactsFromModel(m.allDMPeers(), m.profiles)
	publishedPKs := make(map[string]bool)
	for _, c := range published {
		publishedPKs[c.PubKey] = true
	}

	// BUG: pk_bob was on the relay's NIP-51 list but is no longer in the sidebar.
	// The published list will NOT contain pk_bob, causing data loss.
	if !publishedPKs["pk_bob"] {
		t.Errorf("BUG CONFIRMED: pk_bob was in the NIP-51 contacts list from the relay " +
			"but is missing from the published list — contact silently dropped!")
	}

	// Verify the others are present.
	for _, pk := range []string{"pk_alice", "pk_charlie", "pk_eve"} {
		if !publishedPKs[pk] {
			t.Errorf("expected %s in published contacts", pk)
		}
	}
}
