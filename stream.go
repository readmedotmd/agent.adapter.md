package ai_adapters

import "time"

// ToolStatusValue represents the lifecycle state of a tool call.
type ToolStatusValue string

const (
	ToolRunning  ToolStatusValue = "running"
	ToolComplete ToolStatusValue = "complete"
	ToolFailed   ToolStatusValue = "failed"
)

// SubAgentStatus represents the lifecycle state of a sub-agent.
type SubAgentStatus string

const (
	SubAgentStarted   SubAgentStatus = "started"
	SubAgentCompleted SubAgentStatus = "completed"
	SubAgentFailed    SubAgentStatus = "failed"
)

// StreamEventType categorizes streaming events.
type StreamEventType int

const (
	EventToken             StreamEventType = iota
	EventDone
	EventError
	EventToolUse
	EventToolResult
	EventSystem
	EventThinking
	EventPermissionRequest // agent requests approval to run a tool
	EventPermissionResult  // result of a permission decision (for logging/replay)
	EventProgress          // progress update for a long-running tool call
	EventFileChange        // agent created, edited, or deleted a file
	EventSubAgent          // agent delegated to a sub-agent
	EventCostUpdate        // token usage / cost update
)

// TokenUsage reports token consumption and cost for a turn or session.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
	CacheRead    int
	CacheWrite   int
	TotalCost    float64 // estimated cost in USD
}

// FileChangeOp describes what happened to a file.
type FileChangeOp string

const (
	FileCreated  FileChangeOp = "created"
	FileEdited   FileChangeOp = "edited"
	FileDeleted  FileChangeOp = "deleted"
	FileRenamed  FileChangeOp = "renamed"
)

// FileChange describes a file operation performed by the agent.
//
// Security: UI consumers should validate that Path and OldPath are within
// expected bounds before using them for filesystem operations or display.
// Use filepath.Abs and prefix checks to prevent path traversal.
type FileChange struct {
	Op      FileChangeOp
	Path    string
	OldPath string // for renames
}

// PermissionRequest is sent when the agent needs user approval.
//
// Security: When rendering ToolName, Description, or ToolInput in a web UI,
// always HTML-escape these values to prevent XSS. These fields may contain
// arbitrary content from the agent.
type PermissionRequest struct {
	ToolCallID  string
	ToolName    string
	ToolInput   any
	Description string // human-readable summary of what the tool will do
}

// SubAgentEvent describes sub-agent lifecycle events.
type SubAgentEvent struct {
	AgentID   string
	AgentName string
	Status    SubAgentStatus
	Prompt    string
	Result    string
}

// StreamEvent represents a single event in the streaming response.
type StreamEvent struct {
	Type      StreamEventType
	Timestamp time.Time

	// Content
	Token    string
	Thinking string

	// Tool use — ToolCallID correlates request with result.
	ToolCallID string
	ToolName   string
	ToolInput  any
	ToolOutput any
	ToolStatus ToolStatusValue

	// Permission flow
	Permission *PermissionRequest

	// File operations
	FileChange *FileChange

	// Sub-agent delegation
	SubAgent *SubAgentEvent

	// Progress for long-running operations
	ProgressPct float64 // 0–1, -1 if indeterminate
	ProgressMsg string

	// Cost / usage
	Usage *TokenUsage

	// Control flow
	Error   error
	Message *Message
}
