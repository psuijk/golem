# Agentic System Architecture

## 0. golem-Specific Commitments

These commitments are the source of truth for golem's design. They take
precedence when they conflict with the general descriptions in later sections.

- **Provider-agnostic LLM layer**: The LLM is the most replaceable component
  in the system, not the most central. Canonical message, content, and tool
  types live in golem's own code (`internal/llm/`). Provider adapters
  (`internal/llm/anthropic/`, `internal/llm/ollama/`, etc.) translate between
  the canonical types and each provider's wire format. The agent loop never
  imports a provider package directly.

- **Decoupled agent loop and UI**: The agent loop runs as a goroutine and
  communicates with consumers (UI, logger, tests) through a typed event
  channel (`<-chan event.Event`). The loop has no UI awareness; the UI has
  no loop awareness. Each is independently testable. The channel is created
  and closed by the producer (the loop); consumers receive from it via
  `for range` and stop when it closes.

- **LLM-removable architecture**: The tool dispatcher, registry, conversation
  store, and approval gate are all designed to function and be tested with
  the LLM removed entirely. The LLM is one possible source of tool calls,
  alongside test fixtures and a REPL. This is the inversion of the
  "LLM-in-the-loop" framing in §1: golem's loop coordinates *whatever
  produces tool calls*, and the LLM is one such producer. For pre-LLM
  development, the loop accepts a slice of ToolCall structs as its input
  source.

- **Concrete-first abstraction**: Default to writing concrete code without
  interfaces, and extract abstractions when you have enough information to
  design them well. The usual signals to extract are: (a) two concrete
  implementations exist and you can compare them, (b) testing requires a
  seam to mock an external dependency, or (c) an external framework requires
  you to satisfy a specific interface. Don't extract because you anticipate
  needing flexibility someday — that path produces interfaces shaped by
  imagination instead of requirements.

- **Agent and sub-agent are the same type**: Both are instances of the agent
  loop with different configuration (step limits, available tools, eventually
  model choice and token budget). There is no separate SubAgent type. The
  loop's Config struct controls the differences.

---

## 1. Core Execution: The Agent Loop

The system operates as a bounded loop driven by a source of tool calls
(eventually an LLM, currently test fixtures or a REPL).

### Loop Structure

The loop lives in `internal/agent/`. It is configured via a `Config` struct:

- `MaxSteps int` — hard ceiling on iterations. Prevents runaway loops from
  confused models or infinite tool-call cycles. Main agents and sub-agents
  set this differently.
- `Dispatcher *tool.Dispatcher` — the tool dispatcher to use. Sub-agents
  may receive a restricted dispatcher with fewer tools.

`Run(ctx context.Context, calls []ToolCall) <-chan event.Event` launches
a goroutine, iterates over the input, dispatches tools, emits events, and
closes the channel when done. The `calls []ToolCall` parameter is a
temporary stand-in; it will be replaced by an LLM adapter when that layer
is built.

### Three Escape Hatches

Every iteration checks all three:

1. **Max steps**: Counter hits `Config.MaxSteps` — loop stops, emits
   `TurnCompletedEvent`, closes channel.
2. **Context cancellation**: `ctx.Err() != nil` — user hit Ctrl-C, or a
   parent agent timed out a sub-agent. Loop stops immediately.
3. **Natural termination**: No more tool calls from the source (empty slice
   now, zero tool calls from LLM later). Normal exit.

---

## 2. Event System

Events are typed messages emitted by the loop into a channel for consumers
to process. Defined in `internal/event/`.

### Event Interface

Uses the marker-interface pattern (Go's closest approximation of sum types):

```go
type Event interface {
    isEvent()  // unexported marker — only event package types can implement
}
```

Concrete event types use value receivers on the marker method, so both
values and pointers satisfy the interface.

### Event Types (v1)

- `ToolCallStartedEvent{Name string, Input json.RawMessage}` — loop is
  about to dispatch a tool.
- `ToolCallCompletedEvent{Name string, Result *tool.Result, Err error}` —
  tool returned.
- `UserMessageEvent{Text string}` — user message entered the conversation.
- `TurnCompletedEvent{}` — loop finished a turn. Empty struct used as a
  pure signal.

### Channel Ownership

The loop (producer) creates the channel, sends into it, and closes it.
`Run` returns `<-chan event.Event` (receive-only) so consumers cannot
accidentally send or close. Consumers use `for range` to read until close.

---

## 3. Tool System

### Interface and Registry

Defined in `internal/tool/`:

- `Tool` interface: `Name()`, `Description()`, `Schema()`, `Execute(ctx, input)`.
- `Registry`: `map[string]Tool` with `Register` (exported) and `get`
  (unexported). Sentinel `ErrToolNotFound` used with `errors.Is`, wrapped
  via `fmt.Errorf` with `%w`.
- `Dispatcher`: wraps a `*Registry`, exposes `Dispatch(ctx, name, input)`.

Every concrete tool has `var _ tool.Tool = (*Tool)(nil)` as a compile-time
interface assertion.

### Error Convention

Two return paths from `Execute(ctx, input) (*tool.Result, error)`:

- **Go error (non-nil error, nil result)**: Caller bug. Malformed JSON
  input, programming error. The tool itself broke.
- **IsError result (non-nil result with IsError: true, nil error)**: Expected
  operational failure. File not found, permission denied, timeout, oversized
  file. The tool worked correctly but the operation failed. The LLM receives
  the error text and can react (retry, try a different path, etc.).

This distinction is load-bearing. Tests assert on which path fired, not
just whether something failed.

### Concrete Tools

All tools live under `internal/tools/<name>/`, one package per tool:

**bash** (`internal/tools/bash/`):
- `New(defaultTimeout time.Duration) *Tool`
- Input: `{command: string, timeout_seconds?: int}` (pointer for optional semantics)
- Validates negative/zero timeout as IsError result
- Derives child context with timeout, calls `shellexec.Run`
- Translates `shellexec.Result` into `tool.Result`

**readfile** (`internal/tools/readfile/`):
- `New(maxSize int64) *Tool`
- Input: `{path: string}`
- `os.Stat` before read to reject oversized files without loading into memory
- Size check: `info.Size() > t.maxSize` → IsError with actual size and cap
- `os.ReadFile` → `string(data)` conversion for Result.Text
- Missing file, permission denied, oversized → IsError results (not Go errors)

**writefile** (`internal/tools/writefile/`):
- `New() *Tool` (empty struct, no config)
- Input: `{path: string, content: string}`
- `os.WriteFile` with `0644` (mode only applies on create; existing perms preserved)
- Does not create missing parent directories (intentional — forces LLM to be explicit)
- Overwrites unconditionally via `O_TRUNC`

**editfile** (`internal/tools/editfile/`):
- `New(maxSize int64) *Tool`
- Input: `{path: string, old_string: string, new_string: string}`
- Reads entire file, counts occurrences of `old_string`
- Zero matches → IsError "not found"
- Multiple matches → IsError "must be unique" with count
- Empty `old_string` → IsError (pre-check before file read)
- Exactly one match → `strings.Replace` with count 1, write back
- `new_string` may be empty (deletion is legitimate)
- Uniqueness constraint is a safety property: forces the LLM to supply
  enough context to unambiguously identify the edit location

### JSON Schema and Wire Format

Tool schemas use JSON Schema with snake_case keys on the wire (`old_string`,
`new_string`). Go struct fields are PascalCase with `json:"snake_case"` tags
bridging the two conventions. This matches LLM tool-use API conventions
(Anthropic, OpenAI, MCP all use snake_case for parameter names).

### Shell Execution

`internal/shellexec/` wraps `os/exec`:
- `Run(ctx, name, args...)` returns `*Result` with Stdout/Stderr/ExitCode
- Distinguishes start failures (returns error) from non-zero exits (returns
  Result with ExitCode) from context cancellation (returns `ctx.Err()`)

### fsops — Deferred

No `internal/fsops/` package exists yet. Three filesystem tools (readfile,
writefile, editfile) all call `os.*` functions directly. The decision to
extract a shared filesystem layer is reviewed each time a new filesystem
tool is added.

Extraction signals that haven't fired yet:
- No meaningful shared logic between tools (each uses different `os.*` calls
  with different failure semantics)
- `t.TempDir()` handles test isolation without mocks
- No sandbox enforcement exists yet to centralize

Extraction signals that *would* fire:
- A sandbox-root check that all filesystem tools must enforce (path validation,
  allowed/excluded paths). This is the most likely trigger.
- A common path-canonicalization or symlink-resolution step.
- When enforcement exists, `fsops` becomes the single chokepoint to audit,
  and the value is policy enforcement, not code reuse.

---

## 4. Filesystem Sandboxing — Design Direction (Not Yet Built)

Filesystem restrictions (allowed paths, exclusions, parallel paths) are a
tool-layer concern, not an agent-loop concern. The loop dispatches tool calls
without inspecting their contents; it doesn't see paths.

Enforcement will live in one of:
- A middleware layer in the dispatcher that intercepts filesystem tool calls
  and validates paths before forwarding.
- An `fsops` package that filesystem tools call, which checks paths against
  a policy before performing OS operations.
- Per-tool constructor config (each tool receives its own path restrictions).

The design will be grounded when sandbox enforcement is actually built.
Placeholder comments in Config or tool structs are explicitly avoided — if
the code doesn't enforce it, it shouldn't claim to.

### OS-Level Isolation (Aspirational)

Beyond path-level restrictions, the agent process itself can be sandboxed
at the OS level to limit what syscalls it can make:

- **macOS**: Seatbelt sandboxing for filesystem and network restriction.
- **Linux**: Bubblewrap (namespaces + seccomp filters) to block dangerous
  syscalls.

This is a separate concern from path validation — OS isolation restricts
the process globally, while path validation restricts individual tool
operations. Both are desirable; neither is built. OS-level isolation will
be designed when golem is closer to running untrusted or semi-trusted
LLM-generated commands in production.

---

## 5. Safety & Approval Gates — Design Direction (Not Yet Built)

Destructive tools (writefile, editfile, bash) currently execute without
approval. An approval gate is needed but deferred until the loop is
functional and we can design it as a cross-cutting concern rather than
per-tool logic.

The approval gate is a dispatcher-layer or loop-layer concern:
- The loop emits a `ToolCallStartedEvent` before dispatch.
- A future approval mechanism could intercept between "started" and
  "dispatched" to request user confirmation.
- The tool itself should not contain approval logic.

### Target: 6-Stage Permission Gate

The full permission model golem is building toward, evaluated in order
(deny-first architecture). golem will implement a subset appropriate to
its scope:

1. **Pre-tool Use Hooks** — Custom user scripts (exit 0 to proceed,
   exit 2 to block). Allows project-specific policy without modifying
   golem's code.
2. **Deny Rules** — Explicit blocklists (e.g., `rm -rf /`). Checked
   before any allow rules. Hard stop, no override.
3. **Allow Rules** — Pre-approved patterns that skip the ask step.
   Common operations the user has pre-authorized.
4. **Ask Rules** — Patterns requiring manual Y/N confirmation from the
   user before execution. The default for destructive operations.
5. **Permission Mode** — Global state controlling the overall posture:
   auto-approve (trust the model), plan-only (describe but don't execute),
   ask-every-time, etc.
6. **canUseTool Callback** — Programmatic runtime check. Allows the
   embedding application to enforce arbitrary policy at the last gate
   before execution.

---

## 6. Multi-Agent Delegation

Agent and sub-agent are the same `Loop` type with different `Config`:

- Main agent: higher `MaxSteps`, full `Dispatcher`, broad filesystem access.
- Sub-agent: lower `MaxSteps`, possibly restricted `Dispatcher`, scoped
  filesystem access (once sandboxing is built).

Sub-agents are recursive — they run their own loop, emit their own events
into their own channel. The parent agent (or a coordinator) consumes those
events. Each sub-agent is an independent goroutine.

Future considerations (not yet designed):
- Git worktree isolation for parallel sub-agents editing files.
- Token budget limits per sub-agent.
- Model selection per sub-agent (fast/cheap model for exploration,
  capable model for complex edits).

---

## 7. Context Engineering & Prompt Layering (Not Yet Built)

The system prompt will be a multi-layer stack rebuilt every turn:

| Layer | Component | Function |
|-------|-----------|----------|
| L1 | Core Identity | Agent persona definition |
| L2 | Behavioral Rules | Hard constraints |
| L3 | Tool Schemas | JSON definitions for parameters and error recovery |
| L4 | golem.md | Injected team/project context |
| L5+ | Dynamic Context | Auto-memory, skill descriptions, MCP tool definitions |

---

## 8. Provider-Agnostic LLM Layer (Not Yet Built)

Canonical message, content, and tool types will live in `internal/llm/`.
Provider adapters (`internal/llm/anthropic/`, etc.) translate between
canonical types and wire format. The agent loop imports `internal/llm/`,
never a provider package directly.

Design will be grounded when the first adapter is built. Key decisions
deferred until then:
- Streaming vs batch response handling.
- How tool-use blocks are extracted from provider-specific response formats.
- How to handle provider-specific features (extended thinking, caching)
  without leaking them into canonical types.

---

## 9. Memory & Context Management (Not Yet Built)

Managing the token window is a garbage-collection problem:

- Each turn resends all previous turns (quadratic cost).
- Auto-compaction will summarize older history at a utilization threshold.
- Prompt caching will reduce costs by caching stable system prompt layers.

---

## 10. Model Context Protocol (Not Yet Built)

MCP decouples tool servers from the agentic core:

- **Host**: The agent application (manages permissions/lifecycle).
- **Client**: JSON-RPC 2.0 sessions over stdio or HTTP.
- **Server**: Exposes tools, resources, and prompt templates.

---

## 11. Project Layout

```
github.com/psuijk/golem/
  cmd/golem/main.go          — binary entrypoint, wires registry + dispatcher
  internal/
    agent/                   — agent loop (in progress)
    event/                   — event types (Event interface + concrete structs)
    llm/                     — canonical LLM types (future)
      anthropic/             — Anthropic adapter (future)
    shellexec/               — os/exec wrapper
    tool/                    — Tool interface, Registry, Dispatcher
    tools/
      bash/                  — bash tool
      readfile/              — readfile tool
      writefile/             — writefile tool
      editfile/              — editfile tool
```

Conventions:
- One package per directory, tests live next to code as `*_test.go`.
- `gofmt` + `goimports` on save, `golangci-lint` eventually.
- Cost-conscious: default to Claude Haiku for iteration, small `max_tokens`
  while debugging, hard turn limits during testing.

---

## 12. Testing Conventions

- Tests use `t.TempDir()` for filesystem isolation — real OS operations
  in auto-cleaned scratch directories, no mocks.
- Test helpers use `t.Helper()` so failures report the caller's line number.
- `t.Fatalf` for setup failures and nil-guard checks (continuing would crash).
- `t.Errorf` for assertion failures (want to see all failures in one run).
- Malformed-JSON tests assert Go error return (caller bug).
- Expected-failure tests (missing file, oversized, etc.) assert IsError
  result with nil Go error.
- `strings.Contains` for OS-dependent error messages, exact equality for
  content the test controls.
