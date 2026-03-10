package ai_adapters

import (
	"context"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// SessionProvider
// ---------------------------------------------------------------------------

func TestSessionProvider(t *testing.T) {
	m := newMockAdapter()
	m.sessionID = "sess-abc-123"

	var sp SessionProvider = m
	if sp.SessionID() != "sess-abc-123" {
		t.Errorf("SessionID: got %q", sp.SessionID())
	}
}

// ---------------------------------------------------------------------------
// HistoryClearer
// ---------------------------------------------------------------------------

func TestHistoryClearer(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()
	m.Start(ctx, AdapterConfig{Name: "test"})

	// Add some messages
	m.Send(ctx, Message{ID: "1", Role: RoleUser, Content: TextContent("a")})
	m.Send(ctx, Message{ID: "2", Role: RoleUser, Content: TextContent("b")})

	hist, _ := m.GetHistory(ctx)
	if len(hist) != 2 {
		t.Fatalf("pre-clear: expected 2 messages, got %d", len(hist))
	}

	var hc HistoryClearer = m
	if err := hc.ClearHistory(ctx); err != nil {
		t.Fatalf("ClearHistory: %v", err)
	}

	hist, _ = m.GetHistory(ctx)
	if len(hist) != 0 {
		t.Errorf("post-clear: expected 0 messages, got %d", len(hist))
	}
}

// ---------------------------------------------------------------------------
// HistoryProvider
// ---------------------------------------------------------------------------

func TestHistoryProvider(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()
	m.Start(ctx, AdapterConfig{Name: "test"})

	m.Send(ctx, Message{ID: "m1", Role: RoleUser, Content: TextContent("hello"), Timestamp: time.Now()})
	m.Send(ctx, Message{ID: "m2", Role: RoleUser, Content: TextContent("world"), Timestamp: time.Now()})

	var hp HistoryProvider = m
	hist, err := hp.GetHistory(ctx)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("expected 2, got %d", len(hist))
	}
	if hist[0].ID != "m1" || hist[1].ID != "m2" {
		t.Errorf("order: %q, %q", hist[0].ID, hist[1].ID)
	}
}

func TestHistoryProviderReturnsCopy(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()
	m.Start(ctx, AdapterConfig{Name: "test"})

	m.Send(ctx, Message{ID: "m1", Role: RoleUser, Content: TextContent("hello")})

	hist1, _ := m.GetHistory(ctx)
	hist1[0].ID = "mutated"

	hist2, _ := m.GetHistory(ctx)
	if hist2[0].ID == "mutated" {
		t.Error("GetHistory should return a copy, not a reference to internal state")
	}
}

func TestHistoryProviderEmpty(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()

	hist, err := m.GetHistory(ctx)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(hist) != 0 {
		t.Errorf("expected empty, got %d", len(hist))
	}
}

// ---------------------------------------------------------------------------
// ConversationManager
// ---------------------------------------------------------------------------

func TestConversationManagerListEmpty(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()

	var cm ConversationManager = m
	convos, err := cm.ListConversations(ctx)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(convos) != 0 {
		t.Errorf("expected 0, got %d", len(convos))
	}
}

func TestConversationManagerListAndResume(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()
	m.Start(ctx, AdapterConfig{Name: "test"})

	now := time.Now()
	m.conversations = []Conversation{
		{
			ID:      "conv-1",
			Adapter: "test",
			Title:   "First chat",
			Messages: []Message{
				{ID: "m1", Role: RoleUser, Content: TextContent("hello"), Timestamp: now},
				{ID: "m2", Role: RoleAssistant, Content: TextContent("hi"), Timestamp: now},
			},
			CreatedAt: now,
		},
		{
			ID:      "conv-2",
			Adapter: "test",
			Title:   "Second chat",
			Messages: []Message{
				{ID: "m3", Role: RoleUser, Content: TextContent("bye"), Timestamp: now},
			},
			CreatedAt: now,
		},
	}

	var cm ConversationManager = m
	convos, _ := cm.ListConversations(ctx)
	if len(convos) != 2 {
		t.Fatalf("expected 2 conversations, got %d", len(convos))
	}

	// Resume the first conversation
	if err := cm.ResumeConversation(ctx, "conv-1"); err != nil {
		t.Fatalf("ResumeConversation: %v", err)
	}

	hist, _ := m.GetHistory(ctx)
	if len(hist) != 2 {
		t.Fatalf("expected 2 messages after resume, got %d", len(hist))
	}
	if hist[0].ID != "m1" {
		t.Errorf("first message: got %q", hist[0].ID)
	}
}

func TestConversationManagerResumeNotFound(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()

	var cm ConversationManager = m
	err := cm.ResumeConversation(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing conversation")
	}
}

func TestConversationManagerResumeReplacesHistory(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()
	m.Start(ctx, AdapterConfig{Name: "test"})

	// Send some messages first
	m.Send(ctx, Message{ID: "x1", Role: RoleUser, Content: TextContent("original")})

	now := time.Now()
	m.conversations = []Conversation{
		{
			ID: "conv-1",
			Messages: []Message{
				{ID: "r1", Role: RoleUser, Content: TextContent("resumed"), Timestamp: now},
			},
		},
	}

	m.ResumeConversation(ctx, "conv-1")

	hist, _ := m.GetHistory(ctx)
	if len(hist) != 1 {
		t.Fatalf("expected 1 message, got %d", len(hist))
	}
	if hist[0].ID != "r1" {
		t.Errorf("expected resumed message, got %q", hist[0].ID)
	}
}

// ---------------------------------------------------------------------------
// PermissionResponder
// ---------------------------------------------------------------------------

func TestPermissionResponder(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()

	var pr PermissionResponder = m

	// Approve
	go func() {
		pr.RespondPermission(ctx, "tc-1", true)
	}()

	decision := <-m.permissionCh
	if decision.toolCallID != "tc-1" || !decision.approved {
		t.Errorf("expected approved tc-1, got %+v", decision)
	}

	// Deny
	go func() {
		pr.RespondPermission(ctx, "tc-2", false)
	}()

	decision = <-m.permissionCh
	if decision.toolCallID != "tc-2" || decision.approved {
		t.Errorf("expected denied tc-2, got %+v", decision)
	}
}

func TestPermissionResponderRespectsContext(t *testing.T) {
	m := newMockAdapter()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Fill the channel so it blocks
	for i := 0; i < cap(m.permissionCh); i++ {
		m.permissionCh <- permissionDecision{}
	}

	err := m.RespondPermission(ctx, "tc-1", true)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// ---------------------------------------------------------------------------
// StatusListener
// ---------------------------------------------------------------------------

func TestStatusListener(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()

	var transitions []AdapterStatus
	var sl StatusListener = m
	sl.OnStatusChange(func(s AdapterStatus) {
		transitions = append(transitions, s)
	})

	m.Start(ctx, AdapterConfig{Name: "test"})
	m.Stop()

	if len(transitions) != 2 {
		t.Fatalf("expected 2 transitions, got %d: %v", len(transitions), transitions)
	}
	if transitions[0] != StatusRunning {
		t.Errorf("first transition: got %d", transitions[0])
	}
	if transitions[1] != StatusStopped {
		t.Errorf("second transition: got %d", transitions[1])
	}
}

func TestStatusListenerMultipleCallbacks(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()

	count1 := 0
	count2 := 0

	m.OnStatusChange(func(s AdapterStatus) { count1++ })
	m.OnStatusChange(func(s AdapterStatus) { count2++ })

	m.Start(ctx, AdapterConfig{Name: "test"})

	if count1 != 1 || count2 != 1 {
		t.Errorf("expected both callbacks called once, got %d and %d", count1, count2)
	}
}

// ---------------------------------------------------------------------------
// Optional interface type assertions at runtime
// ---------------------------------------------------------------------------

func TestOptionalInterfaceTypeAssertions(t *testing.T) {
	var adapter Adapter = newMockAdapter()

	if _, ok := adapter.(SessionProvider); !ok {
		t.Error("should implement SessionProvider")
	}
	if _, ok := adapter.(HistoryClearer); !ok {
		t.Error("should implement HistoryClearer")
	}
	if _, ok := adapter.(HistoryProvider); !ok {
		t.Error("should implement HistoryProvider")
	}
	if _, ok := adapter.(ConversationManager); !ok {
		t.Error("should implement ConversationManager")
	}
	if _, ok := adapter.(PermissionResponder); !ok {
		t.Error("should implement PermissionResponder")
	}
	if _, ok := adapter.(StatusListener); !ok {
		t.Error("should implement StatusListener")
	}
}

// ---------------------------------------------------------------------------
// Integration: full send-receive-permission cycle
// ---------------------------------------------------------------------------

func TestFullSendReceivePermissionCycle(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()

	m.Start(ctx, AdapterConfig{
		Name:           "test",
		PermissionMode: PermissionDefault,
	})
	defer m.Stop()

	// Send a message
	msg := Message{
		ID:        "msg-1",
		Role:      RoleUser,
		Content:   TextContent("delete the temp files"),
		Timestamp: time.Now(),
	}
	m.Send(ctx, msg)

	// Simulate the adapter emitting events
	go func() {
		m.emit(StreamEvent{Type: EventThinking, Thinking: "I need to delete files"})
		m.emit(StreamEvent{
			Type: EventPermissionRequest,
			Permission: &PermissionRequest{
				ToolCallID:  "tc-1",
				ToolName:    "Bash",
				ToolInput:   map[string]any{"command": "rm /tmp/old*"},
				Description: "Delete temp files matching /tmp/old*",
			},
		})
		// Wait for permission response
		decision := <-m.permissionCh
		if decision.approved {
			m.emit(StreamEvent{Type: EventToolUse, ToolCallID: "tc-1", ToolName: "Bash", ToolStatus: ToolRunning})
			m.emit(StreamEvent{Type: EventToolResult, ToolCallID: "tc-1", ToolOutput: "deleted 3 files", ToolStatus: ToolComplete})
			m.emit(StreamEvent{Type: EventFileChange, FileChange: &FileChange{Op: FileDeleted, Path: "/tmp/old1.txt"}})
			m.emit(StreamEvent{Type: EventFileChange, FileChange: &FileChange{Op: FileDeleted, Path: "/tmp/old2.txt"}})
			m.emit(StreamEvent{Type: EventFileChange, FileChange: &FileChange{Op: FileDeleted, Path: "/tmp/old3.txt"}})
		}
		m.emit(StreamEvent{Type: EventToken, Token: "Done! Deleted 3 files."})
		m.emit(StreamEvent{Type: EventCostUpdate, Usage: &TokenUsage{InputTokens: 500, OutputTokens: 50, TotalCost: 0.002}})
		m.emit(StreamEvent{Type: EventDone})
	}()

	var eventTypes []StreamEventType
	var fileDeletes int

	for ev := range m.Receive() {
		eventTypes = append(eventTypes, ev.Type)

		// When we get a permission request, approve it
		if ev.Type == EventPermissionRequest {
			m.RespondPermission(ctx, ev.Permission.ToolCallID, true)
		}

		if ev.Type == EventFileChange && ev.FileChange.Op == FileDeleted {
			fileDeletes++
		}

		if ev.Type == EventDone {
			break
		}
	}

	// Verify we got the full sequence
	if len(eventTypes) < 8 {
		t.Fatalf("expected at least 8 events, got %d: %v", len(eventTypes), eventTypes)
	}
	if fileDeletes != 3 {
		t.Errorf("expected 3 file deletes, got %d", fileDeletes)
	}

	// Verify history
	hist, _ := m.GetHistory(ctx)
	if len(hist) != 1 {
		t.Errorf("expected 1 message in history, got %d", len(hist))
	}
}
