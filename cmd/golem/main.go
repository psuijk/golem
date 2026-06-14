package main

import (
	"log"

	"github.com/psuijk/golem/internal/agent"
	"github.com/psuijk/golem/internal/conversation"
	"github.com/psuijk/golem/internal/terminal"
	"github.com/psuijk/golem/internal/tool"
)

func main() {
	d, err := tool.NewDispatcher([]tool.Interface{}, nil)
	if err != nil {
		log.Fatalf("create dispatcher: %v", err)
	}

	r := agent.NewResolver()

	a, err := agent.New(agent.Config{
		Resolver:   r,
		Dispatcher: d,
		Store:      conversation.New(),
	})
	if err != nil {
		log.Fatalf("create agent: %v", err)
	}

	if err := terminal.Run(terminal.Config{
		Agent:   a,
		ModelID: "qwen3:30b",
	}); err != nil {
		log.Fatalf("terminal: %v", err)
	}
}
