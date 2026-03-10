# Building Adapters

This guide covers how to implement the `Adapter` interface for a new AI agent backend.

## Core Interface

Every adapter must implement these 8 methods:

```go
type Adapter interface {
    Start(ctx context.Context, cfg AdapterConfig) error
    Send(ctx context.Context, msg Message, opts ...SendOption) error
    Cancel() error
    Receive() <-chan StreamEvent
    Stop() error
    Status() AdapterStatus
    Capabilities() AdapterCapabilities
    Health(ctx context.Context) error
}
```

## Minimal Implementation

```go
type MyAdapter struct {
    status  AdapterStatus
    events  chan StreamEvent
    cancel  context.CancelFunc
    mu      sync.Mutex
}

func (a *MyAdapter) Start(ctx context.Context, cfg ai.AdapterConfig) error {
    a.mu.Lock()
    defer a.mu.Unlock()

    a.events = make(chan ai.StreamEvent, 256)
    a.status = ai.StatusRunning
    return nil
}

func (a *MyAdapter) Send(ctx context.Context, msg ai.Message, opts ...ai.SendOption) error {
    // Apply options
    var sendOpts ai.SendOptions
    for _, opt := range opts {
        opt(&sendOpts)
    }

    // Process the message and emit events to a.events
    go a.processMessage(ctx, msg, sendOpts)
    return nil
}

func (a *MyAdapter) Cancel() error {
    if a.cancel != nil {
        a.cancel()
    }
    return nil
}

func (a *MyAdapter) Receive() <-chan ai.StreamEvent {
    return a.events
}

func (a *MyAdapter) Stop() error {
    a.mu.Lock()
    defer a.mu.Unlock()

    a.status = ai.StatusStopped
    close(a.events)
    return nil
}

func (a *MyAdapter) Status() ai.AdapterStatus {
    a.mu.Lock()
    defer a.mu.Unlock()
    return a.status
}

func (a *MyAdapter) Capabilities() ai.AdapterCapabilities {
    return ai.AdapterCapabilities{
        SupportsStreaming: true,
        SupportsToolUse:  true,
    }
}

func (a *MyAdapter) Health(ctx context.Context) error {
    a.mu.Lock()
    defer a.mu.Unlock()

    if a.status == ai.StatusError {
        return &ai.AdapterError{Code: ai.ErrCrashed, Message: "adapter process died"}
    }
    return nil
}
```

## Emitting Events

Send events through the channel with timestamps:

```go
func (a *MyAdapter) emit(ev ai.StreamEvent) {
    ev.Timestamp = time.Now()
    a.events <- ev
}

// Token streaming
a.emit(ai.StreamEvent{Type: ai.EventToken, Token: "Hello"})

// Tool use
a.emit(ai.StreamEvent{
    Type:       ai.EventToolUse,
    ToolCallID: "tc-1",
    ToolName:   "Read",
    ToolInput:  map[string]any{"file_path": "/tmp/foo.txt"},
    ToolStatus: ai.ToolRunning,
})

// Tool result
a.emit(ai.StreamEvent{
    Type:       ai.EventToolResult,
    ToolCallID: "tc-1",
    ToolOutput: "file contents here",
    ToolStatus: ai.ToolComplete,
})

// File change
a.emit(ai.StreamEvent{
    Type:       ai.EventFileChange,
    FileChange: &ai.FileChange{Op: ai.FileEdited, Path: "/tmp/foo.txt"},
})

// Cost update
a.emit(ai.StreamEvent{
    Type: ai.EventCostUpdate,
    Usage: &ai.TokenUsage{
        InputTokens:  1500,
        OutputTokens: 300,
        TotalCost:    0.0087,
    },
})

// Done
a.emit(ai.StreamEvent{Type: ai.EventDone})
```

## Typed Errors

Return `*AdapterError` with an appropriate `ErrorCode` so the UI can react:

```go
a.emit(ai.StreamEvent{
    Type: ai.EventError,
    Error: &ai.AdapterError{
        Code:    ai.ErrRateLimited,
        Message: "rate limited by upstream API",
        Err:     originalErr,
    },
})
```

| Code | When to use |
|------|-------------|
| `ErrCrashed` | The underlying process exited unexpectedly |
| `ErrRateLimited` | API returned 429 or equivalent |
| `ErrContextLength` | Conversation exceeds the model's context window |
| `ErrAuth` | Invalid or expired API key |
| `ErrTimeout` | Operation exceeded the context deadline |
| `ErrCancelled` | User called `Cancel()` |
| `ErrPermission` | Tool execution was denied |

## Capabilities

Return accurate capabilities so the UI knows what to render:

```go
func (a *MyAdapter) Capabilities() ai.AdapterCapabilities {
    return ai.AdapterCapabilities{
        SupportsStreaming:    true,
        SupportsImages:       true,
        SupportsFiles:        false,
        SupportsToolUse:      true,
        SupportsMCP:          true,
        SupportsThinking:     true,
        SupportsCancellation: true,
        SupportsHistory:      true,
        SupportsSubAgents:    true,
        MaxContextWindow:     200000,
        SupportedModels:      []string{"claude-sonnet-4-6", "claude-opus-4-6"},
    }
}
```

The UI uses this to:
- Show/hide the thinking panel
- Enable/disable image upload
- Show/hide the cancel button
- Populate the model selector
- Warn when approaching context limits

## Optional Interfaces

Implement any of these to unlock additional features:

### SessionProvider

```go
func (a *MyAdapter) SessionID() string {
    return a.sessionID
}
```

### HistoryClearer

```go
func (a *MyAdapter) ClearHistory(ctx context.Context) error {
    a.messages = nil
    return nil
}
```

### HistoryProvider

```go
func (a *MyAdapter) GetHistory(ctx context.Context) ([]ai.Message, error) {
    return a.messages, nil
}
```

### ConversationManager

```go
func (a *MyAdapter) ListConversations(ctx context.Context) ([]ai.Conversation, error) {
    return a.store.ListAll()
}

func (a *MyAdapter) ResumeConversation(ctx context.Context, id string) error {
    conv, err := a.store.Load(id)
    if err != nil {
        return err
    }
    a.messages = conv.Messages
    return nil
}
```

### PermissionResponder

```go
func (a *MyAdapter) RespondPermission(ctx context.Context, toolCallID string, approved bool) error {
    a.permissionCh <- permissionDecision{id: toolCallID, approved: approved}
    return nil
}
```

### StatusListener

```go
func (a *MyAdapter) OnStatusChange(fn func(ai.AdapterStatus)) {
    a.statusCallbacks = append(a.statusCallbacks, fn)
}
```

## CLI-Based Adapters

For adapters that wrap CLI tools (Claude Code, Codex, Aider), the typical approach is:

1. **Start** — spawn the process with `exec.CommandContext`
2. **Send** — write to stdin (JSON-RPC, line protocol, or raw text)
3. **Receive** — parse stdout/stderr into `StreamEvent`s in a goroutine
4. **Cancel** — send SIGINT or a cancel message via stdin
5. **Stop** — send SIGTERM, wait, then SIGKILL if needed
6. **Health** — check if the process is still running (`cmd.Process.Signal(syscall.Signal(0))`)

```go
func (a *CLIAdapter) Start(ctx context.Context, cfg ai.AdapterConfig) error {
    // Validate WorkDir to prevent path traversal (see Security section below).
    absDir, err := filepath.Abs(cfg.WorkDir)
    if err != nil {
        return fmt.Errorf("invalid WorkDir: %w", err)
    }

    a.cmd = exec.CommandContext(ctx, cfg.Command, cfg.Args...)
    a.cmd.Dir = absDir
    a.cmd.Env = buildEnv(cfg.Env)

    stdin, _ := a.cmd.StdinPipe()
    stdout, _ := a.cmd.StdoutPipe()
    stderr, _ := a.cmd.StderrPipe()

    if err := a.cmd.Start(); err != nil {
        return &ai.AdapterError{Code: ai.ErrCrashed, Message: "failed to start", Err: err}
    }

    a.stdin = stdin
    go a.streamOutput(stdout)
    go a.streamErrors(stderr)
    go a.watchProcess()

    return nil
}
```

## Thread Safety

Adapters must be safe for concurrent use:

- `Send()` and `Cancel()` may be called from different goroutines
- `Receive()` returns a channel that one goroutine reads
- `Status()` and `Health()` can be called at any time
- Use `sync.Mutex` or channels internally to protect shared state

## Security

Adapter implementations are responsible for input validation and output escaping. The interface library deliberately does not enforce these — it defines contracts, not policy — but implementers should follow these guidelines.

### Path Validation

Always validate `WorkDir` and file paths to prevent directory traversal:

```go
func validatePath(path, allowedBase string) (string, error) {
    absPath, err := filepath.Abs(path)
    if err != nil {
        return "", fmt.Errorf("invalid path: %w", err)
    }
    if !strings.HasPrefix(absPath, allowedBase) {
        return "", fmt.Errorf("path %q is outside allowed base %q", absPath, allowedBase)
    }
    return absPath, nil
}
```

Apply this to `AdapterConfig.WorkDir` during `Start()` and to `FileChange.Path` when processing events.

### UI Output Escaping

When rendering event fields in a web UI, always HTML-escape user-facing strings:

```go
case ai.EventPermissionRequest:
    // Escape before rendering in HTML to prevent XSS.
    name := html.EscapeString(ev.Permission.ToolName)
    desc := html.EscapeString(ev.Permission.Description)
    showDialog(name, desc)

case ai.EventToolUse:
    name := html.EscapeString(ev.ToolName)
    showSpinner(name)
```

Fields that should be escaped: `Token`, `Thinking`, `ToolName`, `ProgressMsg`, `PermissionRequest.Description`, `SubAgentEvent.AgentName`, and any other string rendered in markup.

### Sensitive Data

- Do not log or serialize the `Env` map from `AdapterConfig` — it typically contains API keys.
- Do not store secrets in `Message.Metadata` or `Conversation.Metadata` — these may be persisted or transmitted.
- Default to `PermissionDefault` mode in production. Only use `PermissionAcceptAll` in trusted automation environments (CI/CD).

### Command Execution

When spawning processes from `AdapterConfig.Command`:

- Always use `exec.CommandContext` with separate arguments (never shell expansion).
- Never invoke commands via a shell (`sh -c`) with user-provided input.
- Validate `Command` against an allowlist when accepting untrusted configuration.

---

[Getting started &rarr;](./getting-started.md) · [Streaming events &rarr;](./streaming.md) · [Permission flow &rarr;](./permissions.md)
