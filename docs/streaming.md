# Streaming Events

All adapter output flows through a single channel:

```go
events := agent.Receive() // <-chan StreamEvent
```

The channel closes when the adapter stops (via `Stop()` or a crash).

## Event Types

### EventToken

Incremental text output from the model.

```go
case ai.EventToken:
    buffer.WriteString(ev.Token)
    renderMarkdown(buffer.String())
```

### EventThinking

Extended thinking / chain-of-thought content. Only emitted when the model supports it and `MaxThinkingTokens > 0`.

```go
case ai.EventThinking:
    thinkingPanel.Append(ev.Thinking)
```

### EventToolUse

The agent is invoking a tool. Emitted when the tool call starts.

```go
case ai.EventToolUse:
    // ev.ToolCallID  — unique ID, correlates with EventToolResult
    // ev.ToolName    — "Read", "Bash", "Grep", etc.
    // ev.ToolInput   — structured input (any)
    // ev.ToolStatus  — ai.ToolRunning
    showSpinner(ev.ToolCallID, ev.ToolName)
```

### EventToolResult

The tool finished. Correlates with a prior `EventToolUse` by `ToolCallID`.

```go
case ai.EventToolResult:
    // ev.ToolCallID  — matches the EventToolUse
    // ev.ToolOutput  — structured output (any)
    // ev.ToolStatus  — ai.ToolComplete or ai.ToolFailed
    hideSpinner(ev.ToolCallID)
    showResult(ev.ToolOutput)
```

### EventPermissionRequest

The agent wants to run a tool but needs user approval first. Only emitted when `PermissionMode` is `PermissionDefault`.

```go
case ai.EventPermissionRequest:
    // ev.Permission.ToolCallID   — ID to respond with
    // ev.Permission.ToolName     — what tool
    // ev.Permission.ToolInput    — what arguments
    // ev.Permission.Description  — human-readable summary
    showApprovalDialog(ev.Permission)
```

See [permissions.md](./permissions.md) for the full flow.

### EventPermissionResult

Logged after a permission decision is made. Useful for conversation replay and audit trails.

### EventFileChange

The agent created, edited, deleted, or renamed a file.

```go
case ai.EventFileChange:
    // ev.FileChange.Op      — FileCreated, FileEdited, FileDeleted, FileRenamed
    // ev.FileChange.Path    — affected path
    // ev.FileChange.OldPath — previous path (renames only)
    updateFileTree(ev.FileChange)
```

### EventSubAgent

A sub-agent was spawned, completed, or failed.

```go
case ai.EventSubAgent:
    // ev.SubAgent.AgentID   — unique ID
    // ev.SubAgent.AgentName — "researcher", "test-runner", etc.
    // ev.SubAgent.Status    — ai.SubAgentStarted, ai.SubAgentCompleted, ai.SubAgentFailed
    // ev.SubAgent.Prompt    — what the sub-agent was asked
    // ev.SubAgent.Result    — what it returned (on completion)
    updateSubAgentPanel(ev.SubAgent)
```

### EventCostUpdate

Token usage and cost information. May be emitted per-turn or periodically.

```go
case ai.EventCostUpdate:
    // ev.Usage.InputTokens
    // ev.Usage.OutputTokens
    // ev.Usage.CacheRead
    // ev.Usage.CacheWrite
    // ev.Usage.TotalCost    — estimated USD
    updateCostDisplay(ev.Usage)
```

### EventProgress

Progress update for long-running operations. `ProgressPct` is 0–1, or -1 for indeterminate.

```go
case ai.EventProgress:
    if ev.ProgressPct < 0 {
        showIndeterminateSpinner(ev.ProgressMsg)
    } else {
        showProgressBar(ev.ProgressPct, ev.ProgressMsg)
    }
```

### EventDone

The current turn is complete. No more events for this send.

### EventError

Something went wrong. The `.Error` field may be a typed `*AdapterError`.

```go
case ai.EventError:
    var adapterErr *ai.AdapterError
    if errors.As(ev.Error, &adapterErr) {
        handleTypedError(adapterErr.Code)
    } else {
        showGenericError(ev.Error)
    }
```

### EventSystem

System-level notifications from the adapter (not from the model). Carries a `*Message`.

## Timestamps

Every event has a `Timestamp` field (`time.Time`). Use it for:

- Ordering events in the UI
- Calculating latency (time-to-first-token, total generation time)
- Audit logging

## Tool Call Correlation

`EventToolUse` and `EventToolResult` share a `ToolCallID`. A single turn can have multiple tool calls, and they may overlap if the agent runs tools in parallel.

```
EventToolUse    {ToolCallID: "tc-1", ToolName: "Grep",  ToolStatus: ToolRunning}
EventToolUse    {ToolCallID: "tc-2", ToolName: "Read",  ToolStatus: ToolRunning}
EventToolResult {ToolCallID: "tc-2", ToolStatus: ToolComplete}
EventToolResult {ToolCallID: "tc-1", ToolStatus: ToolComplete}
EventToken      {Token: "Based on the search results..."}
```

Track in-flight tool calls by ID so your UI can show parallel operations.

---

[Getting started &rarr;](./getting-started.md) · [Permission flow &rarr;](./permissions.md) · [Building adapters &rarr;](./adapters.md)
