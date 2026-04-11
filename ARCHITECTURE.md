# Agentic System Architecture

## 1. Core Execution: The Agentic Loop

The system operates as a **deterministic state machine** driven by an LLM-in-the-loop. Unlike traditional chatbots (1:1 request-response), this is an **O(N) loop** where N is the number of tool-calls required to reach a terminal state.

### The While Loop Logic

```rust
while (!agent_terminated) {
    let context = assemble_context(); // Stage 5 & 8
    let response = llm.generate(context);

    if (response.tool_calls.is_empty()) {
        agent_terminated = true;
    } else {
        for call in response.tool_calls {
            let result = execute_with_safety_gate(call); // Stage 6
            record_interaction(call, result);
        }
    }
}
```

---

## 2. The 10-Step Pipeline (Per Turn)

Every iteration of the loop follows a rigid pipeline to ensure safety and context integrity.

1. **Context Assembly** - Aggregates history, system prompt layers, and tool definitions.
2. **API Payload Packing** - Serialization of the conversation state.
3. **Inference** - LLM processing (Extended Thinking / Reasoning blocks).
4. **Streaming & Extraction** - Real-time parsing of tool use blocks.
5. **Permission Check** - The 6-stage safety gate.
6. **Pre-tool Hooks** - Deterministic execution of external logic.
7. **Tool Execution** - The runtime performs the IO (file system, shell).
8. **Result Capture** - Captures stdout, stderr, or file diffs.
9. **Post-tool Hooks** - Cleanup or verification logic.
10. **Loop Decision** - Evaluate if the termination signal (zero tool calls) is met.

---

## 3. Context Engineering & Prompt Layering

The system prompt is not static; it is an **8-layer "DNA" stack** (up to 10k tokens) rebuilt every turn to maintain high-precision behavior.

| Layer | Component | Function |
|-------|-----------|----------|
| L1 | Core Identity | ~270 tokens defining the agent persona. |
| L2 | Behavioral Rules | Hard constraints (e.g., "Use read not cat"). |
| L3 | Tool Schemas | JSON definitions for parameters and error recovery. |
| L4 | golem.md | Injected team/project context. |
| L5-8 | Dynamic Context | Auto-memory, skill descriptions, and MCP tool definitions. |

---

## 4. Safety & Sandboxing Architecture

Safety is implemented via a **"Deny-First" priority architecture**.

### The 6-Stage Permission Gate

1. **Pre-tool Use Hooks** - Custom user scripts (exit 0 to proceed, exit 2 to block).
2. **Deny Rules** - Explicit blocklists (e.g., `rm -rf /`).
3. **Allow Rules** - Pre-approved patterns.
4. **Ask Rules** - Patterns requiring manual Y/N.
5. **Permission Mode** - Global state (auto-approve, plan-only, etc.).
6. **canUseTool Callback** - Programmatic runtime check.

### OS-Level Isolation

- **macOS**: Seatbelt sandboxing for filesystem/network restriction.
- **Linux**: Bubblewrap (namespaces + seccomp filters) to block dangerous syscalls.

---

## 5. Memory & Context Management

Managing the token window is treated as a **garbage collection problem**.

- **Quadratic Cost O(N^2)**: Since the API is stateless, each turn `i` resends all previous `i-1` turns.
- **Auto-Compaction**: Triggered at ~65% utilization. Summarizes older conversation history and clears verbose tool outputs to recover space.
- **Prompt Caching**: Reduces costs by ~90% by caching the L1-L8 system prompt layers.

---

## 6. Multi-Agent Delegation

To prevent "context bloating" and maintain precision, the main agent delegates verbose or parallel tasks to **sub-agents**.

- **Architecture**: Recursive. Sub-agents run their own 10-step pipeline.
- **Isolation**: Git worktree isolation allows sub-agents to operate in separate directories without filesystem collisions.
- **Specialization**: Built-in types (fast explorer, planner, generalist, etc.).

---

## 7. Model Context Protocol (MCP)

MCP acts as the **"USB-C" for the agent**, decoupling tool servers from the agentic core.

- **Host**: The agent application (manages permissions/lifecycle).
- **Client**: Maintains JSON-RPC 2.0 sessions over stdio or HTTP.
- **Server**: Exposes three primitives:
  - **Tools** - Executable actions.
  - **Resources** - Data access.
  - **Prompts** - Reusable templates.


## 0. golem-Specific Commitments

The sections below describe the architecture golem is being built toward.
The following commitments are more specific to golem's design and take
precedence when they conflict with the general descriptions:

- **Provider-agnostic LLM layer**: The LLM is the most replaceable component
  in the system, not the most central. Canonical message, content, and tool
  types live in golem's own code (`internal/llm/`). Provider adapters
  (`internal/llm/anthropic/`, `internal/llm/ollama/`, etc.) translate between
  the canonical types and each provider's wire format. The agent loop never
  imports a provider package directly.

- **Decoupled agent loop and UI**: The agent loop runs as a goroutine and
  communicates with consumers (UI, logger, tests) through a typed event
  channel. The loop has no UI awareness; the UI has no loop awareness. Each
  is independently testable.

- **LLM-removable architecture**: The tool dispatcher, registry, conversation
  store, and approval gate are all designed to function and be tested with
  the LLM removed entirely. The LLM is one possible source of tool calls,
  alongside test fixtures and a REPL. This is the inversion of the
  "LLM-in-the-loop" framing in §1: golem's loop coordinates *whatever
  produces tool calls*, and the LLM is one such producer.

- **Concrete-first abstraction**: Default to writing concrete code without
  interfaces, and extract abstractions when you have enough information to
  design them well. The usual signals to extract are: (a) two concrete
  implementations exist and you can compare them, (b) testing requires a
  seam to mock an external dependency, or (c) an external framework requires
  you to satisfy a specific interface. Don't extract because you anticipate
  needing flexibility someday — that path produces interfaces shaped by
  imagination instead of requirements.