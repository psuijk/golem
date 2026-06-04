# Golem

A provider-agnostic agentic framework, handwritten in Go from scratch. Built as a learning project to get hands-on with Go and to understand how agentic harnesses work at a deeper level.

Golem implements a bounded agent loop for tool-use coordination with LLMs, where the LLM is a replaceable component — not the center of the system.

## Architecture

```
cmd/golem/          CLI entry point
internal/
  agent/            Core agent loop (goroutine + event channel)
  conversation/     Append-only multi-turn message store
  event/            Typed event system (started, completed, turn)
  fsops/            Filesystem sandboxing (path policies, symlink-safe)
  llm/              Provider interface + canonical message types
    anthropic/      Anthropic adapter (SSE streaming)
    ollama/         Ollama adapter (local models)
  tool/             Tool interface, registry, and dispatcher
  tools/
    bash/           Shell command execution
    readfile/       Size-capped file reading
    writefile/      File writing (no implicit mkdir)
    editfile/       Targeted string replacement (unique-match constraint)
```

## Key ideas

- **Provider-agnostic** — Canonical types in `internal/llm/`; adapters translate. Swap providers without touching the loop.
- **LLM-removable** — The loop runs on `[]ToolCall` input. Drive it with test fixtures, a REPL, or an LLM.
- **Event-driven** — The loop emits typed events to a `<-chan event.Event`. UI is fully decoupled.
- **Filesystem sandboxing** — Ordered path rules (read-only / read-write) enforced at the dispatcher level, with symlink resolution.
- **Zero external dependencies** — Stdlib only.

## Build & run

```bash
go build ./cmd/golem
go test ./...
```

## Status

Early-stage. Core loop, tool system, conversation store, filesystem policies, and two LLM providers are built and tested. Approval gates, OS-level sandboxing, context management, and MCP support are designed but not yet implemented.
