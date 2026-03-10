package ai_adapters

import (
	"errors"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// StreamEventType constants
// ---------------------------------------------------------------------------

func TestStreamEventTypeValues(t *testing.T) {
	types := []struct {
		typ  StreamEventType
		want int
	}{
		{EventToken, 0},
		{EventDone, 1},
		{EventError, 2},
		{EventToolUse, 3},
		{EventToolResult, 4},
		{EventSystem, 5},
		{EventThinking, 6},
		{EventPermissionRequest, 7},
		{EventPermissionResult, 8},
		{EventProgress, 9},
		{EventFileChange, 10},
		{EventSubAgent, 11},
		{EventCostUpdate, 12},
	}
	for _, tc := range types {
		if int(tc.typ) != tc.want {
			t.Errorf("StreamEventType %d: expected %d", tc.typ, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// FileChangeOp constants
// ---------------------------------------------------------------------------

func TestFileChangeOpValues(t *testing.T) {
	tests := []struct {
		op   FileChangeOp
		want string
	}{
		{FileCreated, "created"},
		{FileEdited, "edited"},
		{FileDeleted, "deleted"},
		{FileRenamed, "renamed"},
	}
	for _, tc := range tests {
		if string(tc.op) != tc.want {
			t.Errorf("FileChangeOp %q: expected %q", tc.op, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Streaming: token events
// ---------------------------------------------------------------------------

func TestStreamTokenEvents(t *testing.T) {
	m := newMockAdapter()

	tokens := []string{"Hello", " ", "world", "!"}
	go func() {
		for _, tok := range tokens {
			m.emit(StreamEvent{Type: EventToken, Token: tok})
		}
		m.emit(StreamEvent{Type: EventDone})
	}()

	var collected []string
	for ev := range m.Receive() {
		if ev.Type == EventDone {
			break
		}
		if ev.Type == EventToken {
			collected = append(collected, ev.Token)
		}
	}

	if len(collected) != len(tokens) {
		t.Fatalf("expected %d tokens, got %d", len(tokens), len(collected))
	}
	for i, tok := range tokens {
		if collected[i] != tok {
			t.Errorf("token %d: got %q, want %q", i, collected[i], tok)
		}
	}
}

// ---------------------------------------------------------------------------
// Streaming: thinking events
// ---------------------------------------------------------------------------

func TestStreamThinkingEvents(t *testing.T) {
	m := newMockAdapter()

	go func() {
		m.emit(StreamEvent{Type: EventThinking, Thinking: "Let me consider..."})
		m.emit(StreamEvent{Type: EventThinking, Thinking: "I should check the file."})
		m.emit(StreamEvent{Type: EventToken, Token: "Here's what I found."})
		m.emit(StreamEvent{Type: EventDone})
	}()

	var thinkingParts []string
	var tokenParts []string
loop1:
	for ev := range m.Receive() {
		switch ev.Type {
		case EventThinking:
			thinkingParts = append(thinkingParts, ev.Thinking)
		case EventToken:
			tokenParts = append(tokenParts, ev.Token)
		case EventDone:
			break loop1
		}
	}
	if len(thinkingParts) != 2 {
		t.Errorf("expected 2 thinking events, got %d", len(thinkingParts))
	}
	if len(tokenParts) != 1 {
		t.Errorf("expected 1 token event, got %d", len(tokenParts))
	}
}

// ---------------------------------------------------------------------------
// Streaming: tool use / result correlation
// ---------------------------------------------------------------------------

func TestStreamToolUseAndResult(t *testing.T) {
	m := newMockAdapter()

	go func() {
		m.emit(StreamEvent{
			Type:       EventToolUse,
			ToolCallID: "tc-1",
			ToolName:   "Read",
			ToolInput:  map[string]any{"file_path": "/tmp/foo"},
			ToolStatus: ToolRunning,
		})
		m.emit(StreamEvent{
			Type:       EventToolResult,
			ToolCallID: "tc-1",
			ToolOutput: "file contents",
			ToolStatus: ToolComplete,
		})
		m.emit(StreamEvent{Type: EventDone})
	}()

	var toolUse, toolResult *StreamEvent
loop2:
	for ev := range m.Receive() {
		switch ev.Type {
		case EventToolUse:
			cp := ev
			toolUse = &cp
		case EventToolResult:
			cp := ev
			toolResult = &cp
		case EventDone:
			break loop2
		}
	}
	if toolUse == nil {
		t.Fatal("missing EventToolUse")
	}
	if toolResult == nil {
		t.Fatal("missing EventToolResult")
	}
	if toolUse.ToolCallID != toolResult.ToolCallID {
		t.Errorf("ToolCallID mismatch: %q vs %q", toolUse.ToolCallID, toolResult.ToolCallID)
	}
	if toolUse.ToolName != "Read" {
		t.Errorf("ToolName: got %q", toolUse.ToolName)
	}
	if toolUse.ToolStatus != ToolRunning {
		t.Errorf("ToolUse status: got %q", toolUse.ToolStatus)
	}
	if toolResult.ToolStatus != ToolComplete {
		t.Errorf("ToolResult status: got %q", toolResult.ToolStatus)
	}
	if toolResult.ToolOutput != "file contents" {
		t.Errorf("ToolOutput: got %v", toolResult.ToolOutput)
	}
}

func TestStreamParallelToolCalls(t *testing.T) {
	m := newMockAdapter()

	go func() {
		// Two tool calls in parallel
		m.emit(StreamEvent{Type: EventToolUse, ToolCallID: "tc-1", ToolName: "Grep", ToolStatus: ToolRunning})
		m.emit(StreamEvent{Type: EventToolUse, ToolCallID: "tc-2", ToolName: "Read", ToolStatus: ToolRunning})
		// Results come back out of order
		m.emit(StreamEvent{Type: EventToolResult, ToolCallID: "tc-2", ToolStatus: ToolComplete})
		m.emit(StreamEvent{Type: EventToolResult, ToolCallID: "tc-1", ToolStatus: ToolComplete})
		m.emit(StreamEvent{Type: EventDone})
	}()

	inflight := map[string]bool{}
loop3:
	for ev := range m.Receive() {
		switch ev.Type {
		case EventToolUse:
			inflight[ev.ToolCallID] = true
		case EventToolResult:
			if !inflight[ev.ToolCallID] {
				t.Errorf("result for unknown tool call %q", ev.ToolCallID)
			}
			delete(inflight, ev.ToolCallID)
		case EventDone:
			break loop3
		}
	}
	if len(inflight) != 0 {
		t.Errorf("unclosed tool calls: %v", inflight)
	}
}

func TestStreamToolUseFailed(t *testing.T) {
	m := newMockAdapter()

	go func() {
		m.emit(StreamEvent{Type: EventToolUse, ToolCallID: "tc-1", ToolName: "Bash", ToolStatus: ToolRunning})
		m.emit(StreamEvent{Type: EventToolResult, ToolCallID: "tc-1", ToolOutput: "exit code 1", ToolStatus: ToolFailed})
		m.emit(StreamEvent{Type: EventDone})
	}()

	var result *StreamEvent
	for ev := range m.Receive() {
		if ev.Type == EventToolResult {
			cp := ev
			result = &cp
		}
		if ev.Type == EventDone {
			break
		}
	}

	if result == nil {
		t.Fatal("missing EventToolResult")
	}
	if result.ToolStatus != ToolFailed {
		t.Errorf("ToolStatus: got %q", result.ToolStatus)
	}
}

// ---------------------------------------------------------------------------
// Streaming: permission request
// ---------------------------------------------------------------------------

func TestStreamPermissionRequest(t *testing.T) {
	m := newMockAdapter()

	go func() {
		m.emit(StreamEvent{
			Type: EventPermissionRequest,
			Permission: &PermissionRequest{
				ToolCallID:  "tc-5",
				ToolName:    "Bash",
				ToolInput:   map[string]any{"command": "rm -rf /tmp/stuff"},
				Description: "Delete /tmp/stuff recursively",
			},
		})
		m.emit(StreamEvent{Type: EventDone})
	}()

	var perm *PermissionRequest
	for ev := range m.Receive() {
		if ev.Type == EventPermissionRequest {
			perm = ev.Permission
		}
		if ev.Type == EventDone {
			break
		}
	}

	if perm == nil {
		t.Fatal("missing permission request")
	}
	if perm.ToolCallID != "tc-5" {
		t.Errorf("ToolCallID: got %q", perm.ToolCallID)
	}
	if perm.ToolName != "Bash" {
		t.Errorf("ToolName: got %q", perm.ToolName)
	}
	if perm.Description != "Delete /tmp/stuff recursively" {
		t.Errorf("Description: got %q", perm.Description)
	}
	input, ok := perm.ToolInput.(map[string]any)
	if !ok {
		t.Fatalf("ToolInput type: got %T", perm.ToolInput)
	}
	if input["command"] != "rm -rf /tmp/stuff" {
		t.Errorf("ToolInput[command]: got %v", input["command"])
	}
}

// ---------------------------------------------------------------------------
// Streaming: file change events
// ---------------------------------------------------------------------------

func TestStreamFileChangeCreated(t *testing.T) {
	m := newMockAdapter()

	go func() {
		m.emit(StreamEvent{
			Type:       EventFileChange,
			FileChange: &FileChange{Op: FileCreated, Path: "/tmp/new.go"},
		})
		m.emit(StreamEvent{Type: EventDone})
	}()

	var fc *FileChange
	for ev := range m.Receive() {
		if ev.Type == EventFileChange {
			fc = ev.FileChange
		}
		if ev.Type == EventDone {
			break
		}
	}

	if fc == nil {
		t.Fatal("missing file change")
	}
	if fc.Op != FileCreated {
		t.Errorf("Op: got %q", fc.Op)
	}
	if fc.Path != "/tmp/new.go" {
		t.Errorf("Path: got %q", fc.Path)
	}
}

func TestStreamFileChangeEdited(t *testing.T) {
	m := newMockAdapter()

	go func() {
		m.emit(StreamEvent{
			Type:       EventFileChange,
			FileChange: &FileChange{Op: FileEdited, Path: "/tmp/main.go"},
		})
		m.emit(StreamEvent{Type: EventDone})
	}()

	var fc *FileChange
	for ev := range m.Receive() {
		if ev.Type == EventFileChange {
			fc = ev.FileChange
		}
		if ev.Type == EventDone {
			break
		}
	}

	if fc.Op != FileEdited {
		t.Errorf("Op: got %q", fc.Op)
	}
}

func TestStreamFileChangeDeleted(t *testing.T) {
	m := newMockAdapter()

	go func() {
		m.emit(StreamEvent{
			Type:       EventFileChange,
			FileChange: &FileChange{Op: FileDeleted, Path: "/tmp/old.go"},
		})
		m.emit(StreamEvent{Type: EventDone})
	}()

	var fc *FileChange
	for ev := range m.Receive() {
		if ev.Type == EventFileChange {
			fc = ev.FileChange
		}
		if ev.Type == EventDone {
			break
		}
	}

	if fc.Op != FileDeleted {
		t.Errorf("Op: got %q", fc.Op)
	}
}

func TestStreamFileChangeRenamed(t *testing.T) {
	m := newMockAdapter()

	go func() {
		m.emit(StreamEvent{
			Type:       EventFileChange,
			FileChange: &FileChange{Op: FileRenamed, Path: "/tmp/new_name.go", OldPath: "/tmp/old_name.go"},
		})
		m.emit(StreamEvent{Type: EventDone})
	}()

	var fc *FileChange
	for ev := range m.Receive() {
		if ev.Type == EventFileChange {
			fc = ev.FileChange
		}
		if ev.Type == EventDone {
			break
		}
	}

	if fc.Op != FileRenamed {
		t.Errorf("Op: got %q", fc.Op)
	}
	if fc.OldPath != "/tmp/old_name.go" {
		t.Errorf("OldPath: got %q", fc.OldPath)
	}
	if fc.Path != "/tmp/new_name.go" {
		t.Errorf("Path: got %q", fc.Path)
	}
}

// ---------------------------------------------------------------------------
// Streaming: sub-agent events
// ---------------------------------------------------------------------------

func TestStreamSubAgentLifecycle(t *testing.T) {
	m := newMockAdapter()

	go func() {
		m.emit(StreamEvent{
			Type: EventSubAgent,
			SubAgent: &SubAgentEvent{
				AgentID:   "sa-1",
				AgentName: "researcher",
				Status:    SubAgentStarted,
				Prompt:    "Find all TODO comments",
			},
		})
		m.emit(StreamEvent{
			Type: EventSubAgent,
			SubAgent: &SubAgentEvent{
				AgentID:   "sa-1",
				AgentName: "researcher",
				Status:    SubAgentCompleted,
				Result:    "Found 5 TODOs",
			},
		})
		m.emit(StreamEvent{Type: EventDone})
	}()

	var events []*SubAgentEvent
	for ev := range m.Receive() {
		if ev.Type == EventSubAgent {
			cp := *ev.SubAgent
			events = append(events, &cp)
		}
		if ev.Type == EventDone {
			break
		}
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 sub-agent events, got %d", len(events))
	}
	if events[0].Status != SubAgentStarted {
		t.Errorf("first event status: got %q", events[0].Status)
	}
	if events[0].Prompt != "Find all TODO comments" {
		t.Errorf("prompt: got %q", events[0].Prompt)
	}
	if events[1].Status != SubAgentCompleted {
		t.Errorf("second event status: got %q", events[1].Status)
	}
	if events[1].Result != "Found 5 TODOs" {
		t.Errorf("result: got %q", events[1].Result)
	}
	if events[0].AgentID != events[1].AgentID {
		t.Error("AgentID should match across lifecycle")
	}
}

func TestStreamSubAgentFailed(t *testing.T) {
	m := newMockAdapter()

	go func() {
		m.emit(StreamEvent{
			Type: EventSubAgent,
			SubAgent: &SubAgentEvent{
				AgentID:   "sa-2",
				AgentName: "test-runner",
				Status:    SubAgentFailed,
				Result:    "exit code 1",
			},
		})
		m.emit(StreamEvent{Type: EventDone})
	}()

	var sa *SubAgentEvent
	for ev := range m.Receive() {
		if ev.Type == EventSubAgent {
			cp := *ev.SubAgent
			sa = &cp
		}
		if ev.Type == EventDone {
			break
		}
	}

	if sa.Status != SubAgentFailed {
		t.Errorf("Status: got %q", sa.Status)
	}
}

// ---------------------------------------------------------------------------
// Streaming: cost update
// ---------------------------------------------------------------------------

func TestStreamCostUpdate(t *testing.T) {
	m := newMockAdapter()

	go func() {
		m.emit(StreamEvent{
			Type: EventCostUpdate,
			Usage: &TokenUsage{
				InputTokens:  1500,
				OutputTokens: 300,
				CacheRead:    200,
				CacheWrite:   50,
				TotalCost:    0.0087,
			},
		})
		m.emit(StreamEvent{Type: EventDone})
	}()

	var usage *TokenUsage
	for ev := range m.Receive() {
		if ev.Type == EventCostUpdate {
			usage = ev.Usage
		}
		if ev.Type == EventDone {
			break
		}
	}

	if usage == nil {
		t.Fatal("missing cost update")
	}
	if usage.InputTokens != 1500 {
		t.Errorf("InputTokens: got %d", usage.InputTokens)
	}
	if usage.OutputTokens != 300 {
		t.Errorf("OutputTokens: got %d", usage.OutputTokens)
	}
	if usage.CacheRead != 200 {
		t.Errorf("CacheRead: got %d", usage.CacheRead)
	}
	if usage.CacheWrite != 50 {
		t.Errorf("CacheWrite: got %d", usage.CacheWrite)
	}
	if usage.TotalCost != 0.0087 {
		t.Errorf("TotalCost: got %f", usage.TotalCost)
	}
}

// ---------------------------------------------------------------------------
// Streaming: progress events
// ---------------------------------------------------------------------------

func TestStreamProgressDeterminate(t *testing.T) {
	m := newMockAdapter()

	go func() {
		m.emit(StreamEvent{Type: EventProgress, ProgressPct: 0.25, ProgressMsg: "Scanning files..."})
		m.emit(StreamEvent{Type: EventProgress, ProgressPct: 0.75, ProgressMsg: "Analyzing..."})
		m.emit(StreamEvent{Type: EventProgress, ProgressPct: 1.0, ProgressMsg: "Done"})
		m.emit(StreamEvent{Type: EventDone})
	}()

	var pcts []float64
	for ev := range m.Receive() {
		if ev.Type == EventProgress {
			pcts = append(pcts, ev.ProgressPct)
		}
		if ev.Type == EventDone {
			break
		}
	}

	if len(pcts) != 3 {
		t.Fatalf("expected 3 progress events, got %d", len(pcts))
	}
	if pcts[0] != 0.25 || pcts[1] != 0.75 || pcts[2] != 1.0 {
		t.Errorf("unexpected pcts: %v", pcts)
	}
}

func TestStreamProgressIndeterminate(t *testing.T) {
	m := newMockAdapter()

	go func() {
		m.emit(StreamEvent{Type: EventProgress, ProgressPct: -1, ProgressMsg: "Working..."})
		m.emit(StreamEvent{Type: EventDone})
	}()

	var prog *StreamEvent
	for ev := range m.Receive() {
		if ev.Type == EventProgress {
			cp := ev
			prog = &cp
		}
		if ev.Type == EventDone {
			break
		}
	}

	if prog.ProgressPct != -1 {
		t.Errorf("expected -1 for indeterminate, got %f", prog.ProgressPct)
	}
}

// ---------------------------------------------------------------------------
// Streaming: error events
// ---------------------------------------------------------------------------

func TestStreamErrorEvent(t *testing.T) {
	m := newMockAdapter()

	go func() {
		m.emit(StreamEvent{
			Type:  EventError,
			Error: &AdapterError{Code: ErrRateLimited, Message: "429 Too Many Requests"},
		})
	}()

	ev := <-m.Receive()
	if ev.Type != EventError {
		t.Fatalf("expected EventError, got %d", ev.Type)
	}

	var ae *AdapterError
	if !errors.As(ev.Error, &ae) {
		t.Fatalf("expected *AdapterError, got %T", ev.Error)
	}
	if ae.Code != ErrRateLimited {
		t.Errorf("Code: got %d", ae.Code)
	}
}

func TestStreamContextLengthError(t *testing.T) {
	m := newMockAdapter()

	go func() {
		m.emit(StreamEvent{
			Type:  EventError,
			Error: &AdapterError{Code: ErrContextLength, Message: "context window exceeded"},
		})
	}()

	ev := <-m.Receive()
	var ae *AdapterError
	errors.As(ev.Error, &ae)
	if ae.Code != ErrContextLength {
		t.Errorf("expected ErrContextLength, got %d", ae.Code)
	}
}

// ---------------------------------------------------------------------------
// Streaming: system events
// ---------------------------------------------------------------------------

func TestStreamSystemEvent(t *testing.T) {
	m := newMockAdapter()

	sysMsg := &Message{
		ID:      "sys-1",
		Role:    RoleSystem,
		Content: TextContent("Session resumed"),
	}

	go func() {
		m.emit(StreamEvent{Type: EventSystem, Message: sysMsg})
		m.emit(StreamEvent{Type: EventDone})
	}()

	var msg *Message
	for ev := range m.Receive() {
		if ev.Type == EventSystem {
			msg = ev.Message
		}
		if ev.Type == EventDone {
			break
		}
	}

	if msg == nil {
		t.Fatal("missing system message")
	}
	if msg.ID != "sys-1" {
		t.Errorf("ID: got %q", msg.ID)
	}
}

// ---------------------------------------------------------------------------
// Streaming: timestamps
// ---------------------------------------------------------------------------

func TestStreamEventsHaveTimestamps(t *testing.T) {
	m := newMockAdapter()

	go func() {
		m.emit(StreamEvent{Type: EventToken, Token: "hi"})
		m.emit(StreamEvent{Type: EventDone})
	}()

	ev := <-m.Receive()
	if ev.Timestamp.IsZero() {
		t.Error("emit should set a timestamp")
	}
	if time.Since(ev.Timestamp) > time.Second {
		t.Errorf("timestamp too old: %v", ev.Timestamp)
	}
}

func TestStreamEventPreservesExplicitTimestamp(t *testing.T) {
	m := newMockAdapter()

	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	go func() {
		m.emit(StreamEvent{Type: EventToken, Token: "hi", Timestamp: ts})
	}()

	ev := <-m.Receive()
	if !ev.Timestamp.Equal(ts) {
		t.Errorf("expected preserved timestamp %v, got %v", ts, ev.Timestamp)
	}
}

// ---------------------------------------------------------------------------
// Streaming: complex multi-event sequence
// ---------------------------------------------------------------------------

func TestStreamFullTurnSequence(t *testing.T) {
	m := newMockAdapter()

	go func() {
		// Thinking
		m.emit(StreamEvent{Type: EventThinking, Thinking: "I need to read the file"})
		// Permission
		m.emit(StreamEvent{
			Type: EventPermissionRequest,
			Permission: &PermissionRequest{
				ToolCallID: "tc-1", ToolName: "Read", Description: "Read /tmp/main.go",
			},
		})
		// Tool use
		m.emit(StreamEvent{Type: EventToolUse, ToolCallID: "tc-1", ToolName: "Read", ToolStatus: ToolRunning})
		// Progress
		m.emit(StreamEvent{Type: EventProgress, ProgressPct: 0.5, ProgressMsg: "Reading..."})
		// Tool result
		m.emit(StreamEvent{Type: EventToolResult, ToolCallID: "tc-1", ToolOutput: "package main", ToolStatus: ToolComplete})
		// File change
		m.emit(StreamEvent{Type: EventFileChange, FileChange: &FileChange{Op: FileEdited, Path: "/tmp/main.go"}})
		// Tokens
		m.emit(StreamEvent{Type: EventToken, Token: "I've updated "})
		m.emit(StreamEvent{Type: EventToken, Token: "the file."})
		// Cost
		m.emit(StreamEvent{Type: EventCostUpdate, Usage: &TokenUsage{InputTokens: 100, OutputTokens: 20, TotalCost: 0.001}})
		// Done
		m.emit(StreamEvent{Type: EventDone})
	}()

	var eventTypes []StreamEventType
	for ev := range m.Receive() {
		eventTypes = append(eventTypes, ev.Type)
		if ev.Type == EventDone {
			break
		}
	}

	expected := []StreamEventType{
		EventThinking,
		EventPermissionRequest,
		EventToolUse,
		EventProgress,
		EventToolResult,
		EventFileChange,
		EventToken,
		EventToken,
		EventCostUpdate,
		EventDone,
	}

	if len(eventTypes) != len(expected) {
		t.Fatalf("expected %d events, got %d: %v", len(expected), len(eventTypes), eventTypes)
	}
	for i, et := range expected {
		if eventTypes[i] != et {
			t.Errorf("event %d: expected %d, got %d", i, et, eventTypes[i])
		}
	}
}

// ---------------------------------------------------------------------------
// TokenUsage
// ---------------------------------------------------------------------------

func TestTokenUsageZeroValues(t *testing.T) {
	u := TokenUsage{}
	if u.InputTokens != 0 || u.OutputTokens != 0 || u.CacheRead != 0 || u.CacheWrite != 0 || u.TotalCost != 0 {
		t.Errorf("zero value should be all zeros: %+v", u)
	}
}

// ---------------------------------------------------------------------------
// FileChange
// ---------------------------------------------------------------------------

func TestFileChangeFields(t *testing.T) {
	fc := FileChange{Op: FileRenamed, Path: "/new", OldPath: "/old"}
	if fc.Op != FileRenamed {
		t.Errorf("Op: got %q", fc.Op)
	}
	if fc.Path != "/new" || fc.OldPath != "/old" {
		t.Errorf("paths: %q -> %q", fc.OldPath, fc.Path)
	}
}

// ---------------------------------------------------------------------------
// PermissionRequest
// ---------------------------------------------------------------------------

func TestPermissionRequestFields(t *testing.T) {
	pr := PermissionRequest{
		ToolCallID:  "tc-1",
		ToolName:    "Write",
		ToolInput:   map[string]any{"file_path": "/x", "content": "y"},
		Description: "Write to /x",
	}
	if pr.ToolCallID != "tc-1" {
		t.Errorf("ToolCallID: got %q", pr.ToolCallID)
	}
	if pr.ToolName != "Write" {
		t.Errorf("ToolName: got %q", pr.ToolName)
	}
	if pr.Description != "Write to /x" {
		t.Errorf("Description: got %q", pr.Description)
	}
}

// ---------------------------------------------------------------------------
// SubAgentEvent
// ---------------------------------------------------------------------------

func TestSubAgentEventFields(t *testing.T) {
	sa := SubAgentEvent{
		AgentID:   "sa-1",
		AgentName: "researcher",
		Status:    SubAgentStarted,
		Prompt:    "find bugs",
		Result:    "",
	}
	if sa.AgentID != "sa-1" || sa.AgentName != "researcher" {
		t.Errorf("unexpected: %+v", sa)
	}
	if sa.Status != SubAgentStarted {
		t.Errorf("Status: got %q", sa.Status)
	}
	if sa.Prompt != "find bugs" {
		t.Errorf("Prompt: got %q", sa.Prompt)
	}
}
