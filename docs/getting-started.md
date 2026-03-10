# Getting Started

## Install

```bash
go get github.com/readmedotmd/agent.adapter.md
```

Requires **Go 1.23.6** or later.

## The Interface

Every adapter implements this:

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

All methods that do I/O take a `context.Context` for cancellation and timeouts.

## Configuration

```go
cfg := ai.AdapterConfig{
    Name:    "claude-code",
    Command: "claude",
    WorkDir: "/path/to/project",
    Args:    []string{"--print"},
    Env:     map[string]string{"ANTHROPIC_API_KEY": "sk-..."},

    Model:             "claude-sonnet-4-6",
    SystemPrompt:      "You are a helpful coding assistant.",
    MaxThinkingTokens: 10000,
    PermissionMode:    ai.PermissionDefault,
    ContextWindow:     200000,

    MCPServers: map[string]ai.MCPServerConfig{
        "filesystem": {
            Command: "npx",
            Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
        },
    },

    AllowedTools:    []string{"Read", "Write", "Bash"},
    DisallowedTools: []string{"mcp__dangerous__delete_all"},

    Agents: map[string]ai.AgentDef{
        "researcher": {
            Description: "Searches the codebase for relevant context",
            Prompt:      "Find all files related to the user's question",
            Tools:       []string{"Glob", "Grep", "Read"},
            Model:       "claude-haiku-4-5-20251001",
        },
    },
}
```

## Messages

Messages use **content blocks** — a list of typed parts that can mix text, code, images, and tool calls.

```go
// Simple text message
msg := ai.Message{
    ID:        "msg-1",
    Role:      ai.RoleUser,
    Content:   ai.TextContent("Fix the login bug"),
    Timestamp: time.Now(),
}

// Multi-modal message with text and image
msg := ai.Message{
    ID:   "msg-2",
    Role: ai.RoleUser,
    Content: []ai.ContentBlock{
        {Type: ai.ContentText, Text: "What's wrong with this screenshot?"},
        {Type: ai.ContentImage, Data: pngBytes, MimeType: "image/png"},
    },
    Timestamp: time.Now(),
}

// Message with file attachment
msg := ai.Message{
    ID:   "msg-3",
    Role: ai.RoleUser,
    Content: []ai.ContentBlock{
        {Type: ai.ContentText, Text: "Review this config"},
        {Type: ai.ContentFile, Data: yamlBytes, MimeType: "application/yaml"},
    },
    Timestamp: time.Now(),
}
```

### Content Block Types

| Type | Fields | Use |
|------|--------|-----|
| `ContentText` | `.Text` | Plain text |
| `ContentCode` | `.Text`, `.Language` | Code with syntax info |
| `ContentImage` | `.Data`, `.MimeType` | Binary image data |
| `ContentFile` | `.Data`, `.MimeType` | File attachment |
| `ContentToolUse` | `.ToolCall` | Tool invocation (assistant messages) |
| `ContentToolResult` | `.ToolCall` | Tool output (tool messages) |

## Per-Turn Options

Override model parameters on any individual send using functional options:

```go
agent.Send(ctx, msg,
    ai.WithMaxTokens(4096),
    ai.WithTemperature(0.0),
    ai.WithTools([]string{"Read", "Grep"}),
)
```

## Streaming Events

All responses come through the event channel:

```go
for ev := range agent.Receive() {
    switch ev.Type {
    case ai.EventToken:
        fmt.Print(ev.Token)

    case ai.EventThinking:
        showThinking(ev.Thinking)

    case ai.EventToolUse:
        showToolCall(ev.ToolCallID, ev.ToolName, ev.ToolInput)

    case ai.EventToolResult:
        showToolResult(ev.ToolCallID, ev.ToolOutput, ev.ToolStatus)

    case ai.EventPermissionRequest:
        handlePermission(ev.Permission)

    case ai.EventFileChange:
        showFileChange(ev.FileChange.Op, ev.FileChange.Path)

    case ai.EventCostUpdate:
        updateCost(ev.Usage)

    case ai.EventProgress:
        updateProgress(ev.ProgressPct, ev.ProgressMsg)

    case ai.EventSubAgent:
        showSubAgent(ev.SubAgent)

    case ai.EventDone:
        break

    case ai.EventError:
        handleError(ev.Error)
    }
}
```

## Error Handling

Errors are typed so your UI can react appropriately:

```go
case ai.EventError:
    var adapterErr *ai.AdapterError
    if errors.As(ev.Error, &adapterErr) {
        switch adapterErr.Code {
        case ai.ErrRateLimited:
            showRetryMessage()
        case ai.ErrContextLength:
            suggestNewConversation()
        case ai.ErrCrashed:
            attemptReconnect()
        case ai.ErrAuth:
            showAuthDialog()
        case ai.ErrTimeout:
            showTimeoutMessage()
        }
    }
```

## Health Checks

Use `Health()` to verify the adapter process is still alive:

```go
if err := agent.Health(ctx); err != nil {
    var adapterErr *ai.AdapterError
    if errors.As(err, &adapterErr) && adapterErr.Code == ai.ErrCrashed {
        // restart the adapter
    }
}
```

## Cancellation

Cancel an in-progress generation without stopping the adapter:

```go
// User clicks "Stop generating"
agent.Cancel()
```

The adapter stays running and ready for the next `Send()`.

## Lifecycle

```
New() → Start() → Send()/Receive() loop → Stop()
                ↕
            Cancel() (interrupts current turn)
```

1. **Start** — launches the underlying agent process
2. **Send/Receive** — exchange messages and stream events
3. **Cancel** — interrupt the current turn (adapter stays alive)
4. **Stop** — shut down the agent process
5. **Health** — check if the process is still alive (call any time after Start)

---

[Streaming events &rarr;](./streaming.md) · [Permission flow &rarr;](./permissions.md) · [Building adapters &rarr;](./adapters.md)
