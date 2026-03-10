package ai_adapters

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Role constants
// ---------------------------------------------------------------------------

func TestRoleValues(t *testing.T) {
	tests := []struct {
		role Role
		want string
	}{
		{RoleUser, "user"},
		{RoleAssistant, "assistant"},
		{RoleSystem, "system"},
		{RoleTool, "tool"},
	}
	for _, tc := range tests {
		if string(tc.role) != tc.want {
			t.Errorf("Role %q: expected %q", tc.role, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// ContentType constants
// ---------------------------------------------------------------------------

func TestContentTypeValues(t *testing.T) {
	tests := []struct {
		ct   ContentType
		want string
	}{
		{ContentText, "text"},
		{ContentCode, "code"},
		{ContentImage, "image"},
		{ContentFile, "file"},
		{ContentToolUse, "tool_use"},
		{ContentToolResult, "tool_result"},
	}
	for _, tc := range tests {
		if string(tc.ct) != tc.want {
			t.Errorf("ContentType %q: expected %q", tc.ct, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// TextContent helper
// ---------------------------------------------------------------------------

func TestTextContent(t *testing.T) {
	blocks := TextContent("hello world")

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Type != ContentText {
		t.Errorf("Type: got %q", blocks[0].Type)
	}
	if blocks[0].Text != "hello world" {
		t.Errorf("Text: got %q", blocks[0].Text)
	}
}

func TestTextContentEmpty(t *testing.T) {
	blocks := TextContent("")
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Text != "" {
		t.Errorf("Text: got %q", blocks[0].Text)
	}
}

// ---------------------------------------------------------------------------
// ContentBlock variants
// ---------------------------------------------------------------------------

func TestContentBlockText(t *testing.T) {
	b := ContentBlock{Type: ContentText, Text: "hello"}
	if b.Type != ContentText || b.Text != "hello" {
		t.Errorf("unexpected: %+v", b)
	}
}

func TestContentBlockCode(t *testing.T) {
	b := ContentBlock{Type: ContentCode, Text: "fmt.Println()", Language: "go"}
	if b.Language != "go" {
		t.Errorf("Language: got %q", b.Language)
	}
}

func TestContentBlockImage(t *testing.T) {
	data := []byte{0x89, 0x50, 0x4E, 0x47} // PNG magic bytes
	b := ContentBlock{Type: ContentImage, Data: data, MimeType: "image/png"}

	if b.MimeType != "image/png" {
		t.Errorf("MimeType: got %q", b.MimeType)
	}
	if len(b.Data) != 4 {
		t.Errorf("Data length: got %d", len(b.Data))
	}
}

func TestContentBlockFile(t *testing.T) {
	data := []byte("key: value\n")
	b := ContentBlock{Type: ContentFile, Data: data, MimeType: "application/yaml"}

	if b.Type != ContentFile {
		t.Errorf("Type: got %q", b.Type)
	}
	if string(b.Data) != "key: value\n" {
		t.Errorf("Data: got %q", string(b.Data))
	}
}

func TestContentBlockToolUse(t *testing.T) {
	tc := &ToolCall{
		ID:     "tc-1",
		Name:   "Read",
		Input:  map[string]any{"file_path": "/tmp/foo"},
		Status: ToolRunning,
	}
	b := ContentBlock{Type: ContentToolUse, ToolCall: tc}

	if b.ToolCall.Name != "Read" {
		t.Errorf("ToolCall.Name: got %q", b.ToolCall.Name)
	}
	if b.ToolCall.ID != "tc-1" {
		t.Errorf("ToolCall.ID: got %q", b.ToolCall.ID)
	}
}

func TestContentBlockToolResult(t *testing.T) {
	tc := &ToolCall{
		ID:     "tc-1",
		Name:   "Read",
		Output: "file contents",
		Status: ToolComplete,
	}
	b := ContentBlock{Type: ContentToolResult, ToolCall: tc}

	if b.ToolCall.Status != ToolComplete {
		t.Errorf("ToolCall.Status: got %q", b.ToolCall.Status)
	}
	if b.ToolCall.Output != "file contents" {
		t.Errorf("ToolCall.Output: got %v", b.ToolCall.Output)
	}
}

// ---------------------------------------------------------------------------
// Message construction
// ---------------------------------------------------------------------------

func TestMessageSimple(t *testing.T) {
	now := time.Now()
	msg := Message{
		ID:        "msg-1",
		Role:      RoleUser,
		Content:   TextContent("hello"),
		Timestamp: now,
		Metadata:  map[string]string{"source": "web"},
	}

	if msg.ID != "msg-1" {
		t.Errorf("ID: got %q", msg.ID)
	}
	if msg.Role != RoleUser {
		t.Errorf("Role: got %q", msg.Role)
	}
	if len(msg.Content) != 1 {
		t.Fatalf("Content: expected 1 block, got %d", len(msg.Content))
	}
	if msg.Metadata["source"] != "web" {
		t.Errorf("Metadata: got %v", msg.Metadata)
	}
	if msg.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestMessageMultiModal(t *testing.T) {
	msg := Message{
		ID:   "msg-2",
		Role: RoleUser,
		Content: []ContentBlock{
			{Type: ContentText, Text: "What is this?"},
			{Type: ContentImage, Data: []byte{0xFF}, MimeType: "image/jpeg"},
			{Type: ContentFile, Data: []byte("data"), MimeType: "text/csv"},
		},
		Timestamp: time.Now(),
	}

	if len(msg.Content) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(msg.Content))
	}
	if msg.Content[0].Type != ContentText {
		t.Errorf("block 0 type: got %q", msg.Content[0].Type)
	}
	if msg.Content[1].Type != ContentImage {
		t.Errorf("block 1 type: got %q", msg.Content[1].Type)
	}
	if msg.Content[2].Type != ContentFile {
		t.Errorf("block 2 type: got %q", msg.Content[2].Type)
	}
}

func TestMessageWithToolCalls(t *testing.T) {
	msg := Message{
		ID:   "msg-3",
		Role: RoleAssistant,
		Content: []ContentBlock{
			{Type: ContentText, Text: "Let me read that file."},
			{
				Type: ContentToolUse,
				ToolCall: &ToolCall{
					ID:     "tc-1",
					Name:   "Read",
					Input:  map[string]any{"file_path": "/tmp/x"},
					Status: ToolRunning,
				},
			},
			{
				Type: ContentToolUse,
				ToolCall: &ToolCall{
					ID:     "tc-2",
					Name:   "Grep",
					Input:  map[string]any{"pattern": "TODO"},
					Status: ToolRunning,
				},
			},
		},
		Timestamp: time.Now(),
	}

	toolCalls := 0
	for _, b := range msg.Content {
		if b.Type == ContentToolUse {
			toolCalls++
		}
	}
	if toolCalls != 2 {
		t.Errorf("expected 2 tool calls, got %d", toolCalls)
	}
}

func TestMessageEmptyContent(t *testing.T) {
	msg := Message{ID: "empty", Role: RoleSystem}
	if msg.Content != nil {
		t.Errorf("expected nil Content, got %v", msg.Content)
	}
}

// ---------------------------------------------------------------------------
// ToolCall
// ---------------------------------------------------------------------------

func TestToolCallFields(t *testing.T) {
	tc := ToolCall{
		ID:     "tc-99",
		Name:   "Bash",
		Input:  map[string]any{"command": "ls"},
		Output: "file1\nfile2\n",
		Status: ToolComplete,
	}

	if tc.ID != "tc-99" {
		t.Errorf("ID: got %q", tc.ID)
	}
	if tc.Name != "Bash" {
		t.Errorf("Name: got %q", tc.Name)
	}
	if tc.Status != ToolComplete {
		t.Errorf("Status: got %q", tc.Status)
	}

	input, ok := tc.Input.(map[string]any)
	if !ok {
		t.Fatalf("Input type: got %T", tc.Input)
	}
	if input["command"] != "ls" {
		t.Errorf("Input[command]: got %v", input["command"])
	}
}

// ---------------------------------------------------------------------------
// Conversation
// ---------------------------------------------------------------------------

func TestConversation(t *testing.T) {
	now := time.Now()
	conv := Conversation{
		ID:      "conv-1",
		Adapter: "claude-code",
		Title:   "Fix login bug",
		Messages: []Message{
			{ID: "m1", Role: RoleUser, Content: TextContent("fix it"), Timestamp: now},
			{ID: "m2", Role: RoleAssistant, Content: TextContent("done"), Timestamp: now.Add(time.Second)},
		},
		CreatedAt: now,
		UpdatedAt: now.Add(time.Second),
		Metadata:  map[string]string{"branch": "fix/login"},
	}

	if conv.ID != "conv-1" {
		t.Errorf("ID: got %q", conv.ID)
	}
	if conv.Title != "Fix login bug" {
		t.Errorf("Title: got %q", conv.Title)
	}
	if len(conv.Messages) != 2 {
		t.Errorf("Messages: got %d", len(conv.Messages))
	}
	if conv.Metadata["branch"] != "fix/login" {
		t.Errorf("Metadata: got %v", conv.Metadata)
	}
	if conv.UpdatedAt.Before(conv.CreatedAt) {
		t.Error("UpdatedAt should be after CreatedAt")
	}
}
