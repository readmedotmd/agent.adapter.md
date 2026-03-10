# Permission Flow

AI coding agents execute real tools — file writes, shell commands, git operations. The permission system lets your UI mediate what gets approved.

## Permission Modes

Set via `AdapterConfig.PermissionMode`:

| Mode | Behaviour | Use case |
|------|-----------|----------|
| `PermissionDefault` | Agent emits `EventPermissionRequest`, waits for response | Interactive UI |
| `PermissionAcceptAll` | All tool calls auto-approved | CI/CD, trusted automation |
| `PermissionPlan` | Agent can read and plan but not write or execute | Preview mode, code review |

## The Flow

```
Agent wants to run a tool
        │
        ▼
PermissionMode == AcceptAll?  ──yes──▶  Tool runs immediately
        │ no
        ▼
Adapter emits EventPermissionRequest
        │
        ▼
UI shows approval dialog to user
        │
        ▼
User approves or denies
        │
        ▼
UI calls RespondPermission(ctx, toolCallID, approved)
        │
        ▼
Agent proceeds or skips the tool
```

## Receiving Permission Requests

```go
case ai.EventPermissionRequest:
    req := ev.Permission

    fmt.Printf("Agent wants to run: %s\n", req.ToolName)
    fmt.Printf("With input: %v\n", req.ToolInput)
    fmt.Printf("Description: %s\n", req.Description)

    // req.ToolCallID is the key — you need it to respond
```

The `PermissionRequest` struct:

```go
type PermissionRequest struct {
    ToolCallID  string  // unique ID for this request
    ToolName    string  // "Bash", "Write", "Edit", etc.
    ToolInput   any     // structured arguments
    Description string  // human-readable: "Delete file /tmp/data.json"
}
```

## Responding

Use the `PermissionResponder` optional interface:

```go
if pr, ok := agent.(ai.PermissionResponder); ok {
    err := pr.RespondPermission(ctx, req.ToolCallID, true)  // approved
    if err != nil {
        // handle error
    }
}
```

Pass `false` to deny:

```go
pr.RespondPermission(ctx, req.ToolCallID, false)
```

If the adapter doesn't implement `PermissionResponder`, it handles permissions internally (e.g., stdin-based CLI adapters).

## UI Patterns

### Simple approve/deny

```go
func handlePermission(agent ai.Adapter, ctx context.Context, req *ai.PermissionRequest) {
    msg := fmt.Sprintf("Allow %s?\n%s", req.ToolName, req.Description)
    approved := confirm(msg)

    if pr, ok := agent.(ai.PermissionResponder); ok {
        pr.RespondPermission(ctx, req.ToolCallID, approved)
    }
}
```

### Always-allow for a tool

Track user preferences per tool name and auto-respond for tools the user has previously marked as "always allow":

```go
var alwaysAllow = map[string]bool{}

func handlePermission(agent ai.Adapter, ctx context.Context, req *ai.PermissionRequest) {
    approved := alwaysAllow[req.ToolName]
    if !approved {
        approved, remember := confirmWithRemember(req)
        if remember {
            alwaysAllow[req.ToolName] = approved
        }
    }

    if pr, ok := agent.(ai.PermissionResponder); ok {
        pr.RespondPermission(ctx, req.ToolCallID, approved)
    }
}
```

### Timeout with auto-deny

```go
func handlePermission(agent ai.Adapter, ctx context.Context, req *ai.PermissionRequest) {
    ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    approved := confirmWithContext(ctx, req) // returns false on timeout

    if pr, ok := agent.(ai.PermissionResponder); ok {
        pr.RespondPermission(ctx, req.ToolCallID, approved)
    }
}
```

## Audit Trail

`EventPermissionResult` is emitted after each decision. Use it to build an audit log:

```go
case ai.EventPermissionResult:
    log.Printf("permission: tool=%s approved=%v at=%v",
        ev.ToolName, ev.ToolStatus == ai.ToolComplete, ev.Timestamp)
```

---

[Getting started &rarr;](./getting-started.md) · [Streaming events &rarr;](./streaming.md) · [Building adapters &rarr;](./adapters.md)
