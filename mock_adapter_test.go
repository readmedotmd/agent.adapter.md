package ai_adapters

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// mockAdapter is a full-featured mock that implements Adapter and all optional interfaces.
// It is used across all test files to validate the interface contracts.
type mockAdapter struct {
	mu              sync.Mutex
	status          AdapterStatus
	config          AdapterConfig
	events          chan StreamEvent
	messages        []Message
	conversations   []Conversation
	sessionID       string
	statusCallbacks []func(AdapterStatus)
	permissionCh    chan permissionDecision
	started         bool
	cancelled       bool
	healthy         bool
	startErr        error
	sendErr         error
	healthErr       error
}

type permissionDecision struct {
	toolCallID string
	response   ApprovalResponse
}

func newMockAdapter() *mockAdapter {
	return &mockAdapter{
		status:       StatusIdle,
		events:       make(chan StreamEvent, 256),
		permissionCh: make(chan permissionDecision, 16),
		healthy:      true,
		sessionID:    "session-001",
	}
}

// --- Core Adapter interface ---

func (m *mockAdapter) Start(ctx context.Context, cfg AdapterConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.startErr != nil {
		return m.startErr
	}

	select {
	case <-ctx.Done():
		return &AdapterError{Code: ErrTimeout, Message: "start cancelled", Err: ctx.Err()}
	default:
	}

	m.config = cfg
	m.started = true
	m.setStatusLocked(StatusRunning)
	return nil
}

func (m *mockAdapter) Send(ctx context.Context, msg Message, opts ...SendOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sendErr != nil {
		return m.sendErr
	}
	if m.status != StatusRunning {
		return &AdapterError{Code: ErrUnknown, Message: "adapter not running"}
	}

	select {
	case <-ctx.Done():
		return &AdapterError{Code: ErrTimeout, Message: "send cancelled", Err: ctx.Err()}
	default:
	}

	var sendOpts SendOptions
	for _, opt := range opts {
		opt(&sendOpts)
	}

	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockAdapter) Cancel() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelled = true
	return nil
}

func (m *mockAdapter) Receive() <-chan StreamEvent {
	return m.events
}

func (m *mockAdapter) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.status == StatusStopped {
		return nil
	}

	m.setStatusLocked(StatusStopped)
	close(m.events)
	return nil
}

func (m *mockAdapter) Status() AdapterStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *mockAdapter) Capabilities() AdapterCapabilities {
	return AdapterCapabilities{
		SupportsStreaming:    true,
		SupportsImages:       true,
		SupportsFiles:        true,
		SupportsToolUse:      true,
		SupportsMCP:          true,
		SupportsThinking:     true,
		SupportsCancellation: true,
		SupportsHistory:      true,
		SupportsSubAgents:    true,
		MaxContextWindow:     200000,
		SupportedModels:      []string{"model-a", "model-b"},
	}
}

func (m *mockAdapter) Health(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.healthErr != nil {
		return m.healthErr
	}
	if !m.healthy {
		return &AdapterError{Code: ErrCrashed, Message: "adapter process died"}
	}
	return nil
}

// --- SessionProvider ---

func (m *mockAdapter) SessionID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessionID
}

// --- HistoryClearer ---

func (m *mockAdapter) ClearHistory(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = nil
	return nil
}

// --- HistoryProvider ---

func (m *mockAdapter) GetHistory(ctx context.Context) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]Message, len(m.messages))
	copy(cp, m.messages)
	return cp, nil
}

// --- ConversationManager ---

func (m *mockAdapter) ListConversations(ctx context.Context) ([]Conversation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]Conversation, len(m.conversations))
	copy(cp, m.conversations)
	return cp, nil
}

func (m *mockAdapter) ResumeConversation(ctx context.Context, conversationID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.conversations {
		if c.ID == conversationID {
			m.messages = make([]Message, len(c.Messages))
			copy(m.messages, c.Messages)
			return nil
		}
	}
	return fmt.Errorf("conversation %q not found", conversationID)
}

// --- PermissionResponder ---

func (m *mockAdapter) RespondPermission(ctx context.Context, toolCallID string, response ApprovalResponse) error {
	select {
	case m.permissionCh <- permissionDecision{toolCallID: toolCallID, response: response}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// --- StatusListener ---

func (m *mockAdapter) OnStatusChange(fn func(AdapterStatus)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusCallbacks = append(m.statusCallbacks, fn)
}

// --- helpers ---

func (m *mockAdapter) setStatusLocked(s AdapterStatus) {
	m.status = s
	for _, fn := range m.statusCallbacks {
		fn(s)
	}
}

func (m *mockAdapter) emit(ev StreamEvent) {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	m.events <- ev
}
