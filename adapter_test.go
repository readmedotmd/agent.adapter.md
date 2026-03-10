package ai_adapters

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

func TestMockImplementsAdapter(t *testing.T) {
	var _ Adapter = (*mockAdapter)(nil)
}

func TestMockImplementsSessionProvider(t *testing.T) {
	var _ SessionProvider = (*mockAdapter)(nil)
}

func TestMockImplementsHistoryClearer(t *testing.T) {
	var _ HistoryClearer = (*mockAdapter)(nil)
}

func TestMockImplementsHistoryProvider(t *testing.T) {
	var _ HistoryProvider = (*mockAdapter)(nil)
}

func TestMockImplementsConversationManager(t *testing.T) {
	var _ ConversationManager = (*mockAdapter)(nil)
}

func TestMockImplementsPermissionResponder(t *testing.T) {
	var _ PermissionResponder = (*mockAdapter)(nil)
}

func TestMockImplementsStatusListener(t *testing.T) {
	var _ StatusListener = (*mockAdapter)(nil)
}

// ---------------------------------------------------------------------------
// Lifecycle: Start / Status / Stop
// ---------------------------------------------------------------------------

func TestStartSetsRunning(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()

	if m.Status() != StatusIdle {
		t.Fatalf("expected StatusIdle, got %d", m.Status())
	}

	if err := m.Start(ctx, AdapterConfig{Name: "test"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if m.Status() != StatusRunning {
		t.Fatalf("expected StatusRunning, got %d", m.Status())
	}
}

func TestStopSetsStoppedAndClosesChannel(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()
	m.Start(ctx, AdapterConfig{Name: "test"})

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if m.Status() != StatusStopped {
		t.Fatalf("expected StatusStopped, got %d", m.Status())
	}

	// Channel should be closed — reading should return zero value immediately.
	select {
	case _, ok := <-m.Receive():
		if ok {
			t.Fatal("expected channel closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("channel not closed after Stop")
	}
}

func TestDoubleStopIsIdempotent(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()
	m.Start(ctx, AdapterConfig{Name: "test"})

	if err := m.Stop(); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := m.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestStartRespectsContextCancellation(t *testing.T) {
	m := newMockAdapter()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before start

	err := m.Start(ctx, AdapterConfig{Name: "test"})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	var ae *AdapterError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *AdapterError, got %T", err)
	}
	if ae.Code != ErrTimeout {
		t.Fatalf("expected ErrTimeout, got %d", ae.Code)
	}
}

func TestStartReturnsConfiguredError(t *testing.T) {
	m := newMockAdapter()
	m.startErr = &AdapterError{Code: ErrAuth, Message: "bad key"}

	err := m.Start(context.Background(), AdapterConfig{Name: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	var ae *AdapterError
	if !errors.As(err, &ae) || ae.Code != ErrAuth {
		t.Fatalf("expected ErrAuth, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

func TestStartStoresConfig(t *testing.T) {
	m := newMockAdapter()
	cfg := AdapterConfig{
		Name:           "claude-code",
		Command:        "claude",
		WorkDir:        "/tmp",
		Model:          "claude-sonnet-4-6",
		PermissionMode: PermissionDefault,
		Env:            map[string]string{"KEY": "VAL"},
		MCPServers: map[string]MCPServerConfig{
			"fs": {Command: "npx", Args: []string{"-y", "server-fs"}},
		},
		AllowedTools:    []string{"Read", "Write"},
		DisallowedTools: []string{"Bash"},
		Agents: map[string]AgentDef{
			"researcher": {Description: "search", Prompt: "find stuff", Tools: []string{"Grep"}, Model: "fast"},
		},
		ContextWindow:     200000,
		MaxThinkingTokens: 10000,
	}

	m.Start(context.Background(), cfg)

	if m.config.Name != "claude-code" {
		t.Errorf("Name: got %q", m.config.Name)
	}
	if m.config.Model != "claude-sonnet-4-6" {
		t.Errorf("Model: got %q", m.config.Model)
	}
	if m.config.Env["KEY"] != "VAL" {
		t.Errorf("Env: got %v", m.config.Env)
	}
	if m.config.MCPServers["fs"].Command != "npx" {
		t.Errorf("MCPServers: got %v", m.config.MCPServers)
	}
	if len(m.config.AllowedTools) != 2 {
		t.Errorf("AllowedTools: got %v", m.config.AllowedTools)
	}
	if len(m.config.DisallowedTools) != 1 {
		t.Errorf("DisallowedTools: got %v", m.config.DisallowedTools)
	}
	if m.config.Agents["researcher"].Description != "search" {
		t.Errorf("Agents: got %v", m.config.Agents)
	}
	if m.config.ContextWindow != 200000 {
		t.Errorf("ContextWindow: got %d", m.config.ContextWindow)
	}
	if m.config.MaxThinkingTokens != 10000 {
		t.Errorf("MaxThinkingTokens: got %d", m.config.MaxThinkingTokens)
	}
	if m.config.PermissionMode != PermissionDefault {
		t.Errorf("PermissionMode: got %q", m.config.PermissionMode)
	}
}

// ---------------------------------------------------------------------------
// Send
// ---------------------------------------------------------------------------

func TestSendAppendsMessage(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()
	m.Start(ctx, AdapterConfig{Name: "test"})

	msg := Message{
		ID:        "msg-1",
		Role:      RoleUser,
		Content:   TextContent("hello"),
		Timestamp: time.Now(),
	}
	if err := m.Send(ctx, msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	hist, _ := m.GetHistory(ctx)
	if len(hist) != 1 {
		t.Fatalf("expected 1 message, got %d", len(hist))
	}
	if hist[0].ID != "msg-1" {
		t.Errorf("ID: got %q", hist[0].ID)
	}
}

func TestSendFailsWhenNotRunning(t *testing.T) {
	m := newMockAdapter()
	msg := Message{ID: "msg-1", Role: RoleUser, Content: TextContent("hello")}

	err := m.Send(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error when adapter not running")
	}
}

func TestSendRespectsContextCancellation(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()
	m.Start(ctx, AdapterConfig{Name: "test"})

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	err := m.Send(cancelCtx, Message{ID: "1", Role: RoleUser, Content: TextContent("hi")})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	var ae *AdapterError
	if !errors.As(err, &ae) || ae.Code != ErrTimeout {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
}

func TestSendReturnsConfiguredError(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()
	m.Start(ctx, AdapterConfig{Name: "test"})
	m.sendErr = &AdapterError{Code: ErrRateLimited, Message: "429"}

	err := m.Send(ctx, Message{ID: "1", Role: RoleUser, Content: TextContent("hi")})
	var ae *AdapterError
	if !errors.As(err, &ae) || ae.Code != ErrRateLimited {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// SendOptions (functional options)
// ---------------------------------------------------------------------------

func TestSendOptions(t *testing.T) {
	var opts SendOptions

	WithMaxTokens(4096)(&opts)
	WithTemperature(0.7)(&opts)
	WithStopSequences([]string{"STOP", "END"})(&opts)
	WithTools([]string{"Read", "Grep"})(&opts)

	if opts.MaxTokens != 4096 {
		t.Errorf("MaxTokens: got %d", opts.MaxTokens)
	}
	if opts.Temperature != 0.7 {
		t.Errorf("Temperature: got %f", opts.Temperature)
	}
	if len(opts.StopSequences) != 2 || opts.StopSequences[0] != "STOP" {
		t.Errorf("StopSequences: got %v", opts.StopSequences)
	}
	if len(opts.Tools) != 2 || opts.Tools[0] != "Read" {
		t.Errorf("Tools: got %v", opts.Tools)
	}
}

func TestSendOptionsDefaultsAreZero(t *testing.T) {
	var opts SendOptions
	if opts.MaxTokens != 0 {
		t.Errorf("MaxTokens default: got %d", opts.MaxTokens)
	}
	if opts.Temperature != 0 {
		t.Errorf("Temperature default: got %f", opts.Temperature)
	}
	if opts.StopSequences != nil {
		t.Errorf("StopSequences default: got %v", opts.StopSequences)
	}
	if opts.Tools != nil {
		t.Errorf("Tools default: got %v", opts.Tools)
	}
}

// ---------------------------------------------------------------------------
// Cancel
// ---------------------------------------------------------------------------

func TestCancel(t *testing.T) {
	m := newMockAdapter()
	ctx := context.Background()
	m.Start(ctx, AdapterConfig{Name: "test"})

	if err := m.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	m.mu.Lock()
	cancelled := m.cancelled
	m.mu.Unlock()
	if !cancelled {
		t.Fatal("expected cancelled to be true")
	}
}

// ---------------------------------------------------------------------------
// Capabilities
// ---------------------------------------------------------------------------

func TestCapabilities(t *testing.T) {
	m := newMockAdapter()
	caps := m.Capabilities()

	if !caps.SupportsStreaming {
		t.Error("SupportsStreaming should be true")
	}
	if !caps.SupportsImages {
		t.Error("SupportsImages should be true")
	}
	if !caps.SupportsFiles {
		t.Error("SupportsFiles should be true")
	}
	if !caps.SupportsToolUse {
		t.Error("SupportsToolUse should be true")
	}
	if !caps.SupportsMCP {
		t.Error("SupportsMCP should be true")
	}
	if !caps.SupportsThinking {
		t.Error("SupportsThinking should be true")
	}
	if !caps.SupportsCancellation {
		t.Error("SupportsCancellation should be true")
	}
	if !caps.SupportsHistory {
		t.Error("SupportsHistory should be true")
	}
	if !caps.SupportsSubAgents {
		t.Error("SupportsSubAgents should be true")
	}
	if caps.MaxContextWindow != 200000 {
		t.Errorf("MaxContextWindow: got %d", caps.MaxContextWindow)
	}
	if len(caps.SupportedModels) != 2 {
		t.Errorf("SupportedModels: got %v", caps.SupportedModels)
	}
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

func TestHealthOK(t *testing.T) {
	m := newMockAdapter()
	if err := m.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
}

func TestHealthCrashed(t *testing.T) {
	m := newMockAdapter()
	m.healthy = false

	err := m.Health(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var ae *AdapterError
	if !errors.As(err, &ae) || ae.Code != ErrCrashed {
		t.Fatalf("expected ErrCrashed, got %v", err)
	}
}

func TestHealthCustomError(t *testing.T) {
	m := newMockAdapter()
	m.healthErr = &AdapterError{Code: ErrAuth, Message: "expired"}

	err := m.Health(context.Background())
	var ae *AdapterError
	if !errors.As(err, &ae) || ae.Code != ErrAuth {
		t.Fatalf("expected ErrAuth, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// AdapterError
// ---------------------------------------------------------------------------

func TestAdapterErrorMessage(t *testing.T) {
	err := &AdapterError{Code: ErrRateLimited, Message: "rate limited"}
	if err.Error() != "rate limited" {
		t.Errorf("got %q", err.Error())
	}
}

func TestAdapterErrorMessageWithWrapped(t *testing.T) {
	inner := errors.New("connection refused")
	err := &AdapterError{Code: ErrCrashed, Message: "adapter died", Err: inner}

	if err.Error() != "adapter died: connection refused" {
		t.Errorf("got %q", err.Error())
	}
}

func TestAdapterErrorUnwrap(t *testing.T) {
	inner := errors.New("inner")
	err := &AdapterError{Code: ErrCrashed, Message: "outer", Err: inner}

	if !errors.Is(err, inner) {
		t.Fatal("Unwrap should expose inner error")
	}
}

func TestAdapterErrorUnwrapNil(t *testing.T) {
	err := &AdapterError{Code: ErrUnknown, Message: "oops"}
	if err.Unwrap() != nil {
		t.Fatal("Unwrap should return nil when Err is nil")
	}
}

func TestAdapterErrorAs(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", &AdapterError{Code: ErrContextLength, Message: "too long"})

	var ae *AdapterError
	if !errors.As(err, &ae) {
		t.Fatal("errors.As should find *AdapterError")
	}
	if ae.Code != ErrContextLength {
		t.Errorf("Code: got %d", ae.Code)
	}
}

func TestErrorCodeValues(t *testing.T) {
	// Verify iota ordering hasn't shifted
	codes := []struct {
		code ErrorCode
		want int
	}{
		{ErrUnknown, 0},
		{ErrCrashed, 1},
		{ErrRateLimited, 2},
		{ErrContextLength, 3},
		{ErrAuth, 4},
		{ErrTimeout, 5},
		{ErrCancelled, 6},
		{ErrPermission, 7},
	}
	for _, tc := range codes {
		if int(tc.code) != tc.want {
			t.Errorf("ErrorCode %d: expected %d", tc.code, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// PermissionMode constants
// ---------------------------------------------------------------------------

func TestPermissionModeValues(t *testing.T) {
	if PermissionDefault != "default" {
		t.Errorf("got %q", PermissionDefault)
	}
	if PermissionAcceptAll != "accept_all" {
		t.Errorf("got %q", PermissionAcceptAll)
	}
	if PermissionPlan != "plan" {
		t.Errorf("got %q", PermissionPlan)
	}
}

// ---------------------------------------------------------------------------
// AdapterStatus constants
// ---------------------------------------------------------------------------

func TestAdapterStatusValues(t *testing.T) {
	statuses := []struct {
		status AdapterStatus
		want   int
	}{
		{StatusIdle, 0},
		{StatusRunning, 1},
		{StatusStopped, 2},
		{StatusError, 3},
	}
	for _, tc := range statuses {
		if int(tc.status) != tc.want {
			t.Errorf("AdapterStatus %d: expected %d", tc.status, tc.want)
		}
	}
}
