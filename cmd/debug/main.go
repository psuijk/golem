package main

import (
	"context"
	"fmt"
	"time"

	"github.com/psuijk/golem/internal/agent"
	"github.com/psuijk/golem/internal/conversation"
	"github.com/psuijk/golem/internal/llm"
	"github.com/psuijk/golem/internal/tool"
	"github.com/psuijk/golem/internal/tools/bash"
	"github.com/psuijk/golem/internal/tools/readfile"
)

func main() {
	// First test: call ollama provider directly
	fmt.Println("=== Direct provider test ===")
	r := agent.NewResolver()
	p, err := r.Resolve("llama3.2:latest")
	if err != nil {
		fmt.Printf("resolve error: %v\n", err)
		return
	}

	ch, err := p.Stream(context.Background(), llm.RequestParams{
		Model: "llama3.2:latest",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: []llm.Content{llm.TextContent{Text: "say hello"}}},
		},
	})
	if err != nil {
		fmt.Printf("stream error: %v\n", err)
		return
	}
	for ev := range ch {
		fmt.Printf("[%T] %+v\n", ev, ev)
	}

	// Second test: through agent
	fmt.Println("\n=== Agent test ===")
	tools := []tool.Interface{
		bash.New(30 * time.Second),
		readfile.New(1 << 20),
	}
	d, _ := tool.NewDispatcher(tools, nil)
	r2 := agent.NewResolver()
	a, _ := agent.New(agent.Config{
		Resolver:   r2,
		Dispatcher: d,
		Store:      conversation.New(),
	})

	ch2 := a.Run(context.Background(), "llama3.2:latest", "say hello")
	for ev := range ch2 {
		fmt.Printf("[%T] %+v\n", ev, ev)
	}
}
