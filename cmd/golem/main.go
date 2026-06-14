package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/psuijk/golem/internal/agent"
	"github.com/psuijk/golem/internal/conversation"
	"github.com/psuijk/golem/internal/event"
	"github.com/psuijk/golem/internal/tool"
	"github.com/psuijk/golem/internal/tools/bash"
	"github.com/psuijk/golem/internal/tools/readfile"
)

func main() {
	ctx := context.Background()

	tools := []tool.Interface{
		bash.New(30 * time.Second),
		readfile.New(1 << 20),
	}

	d, err := tool.NewDispatcher(tools, nil)
	if err != nil {
		log.Fatalf("create dispatcher: %v", err)
	}

	r := agent.NewResolver()

	a, err := agent.New(agent.Config{Resolver: r, Dispatcher: d, Store: conversation.New()})
	if err != nil {
		log.Fatalf("create agent: %v", err)
	}

	var userInput string

	fmt.Print("How can I help you today?: ")

	scanner := bufio.NewScanner(os.Stdin)

	for userInput != "/exit" {
		for scanner.Scan() {
			userInput = scanner.Text()
		}
		ch := a.Run(ctx, "llama3.2:latest", userInput)

		for ev := range ch {
			switch e := ev.(type) {
			case event.TextDeltaEvent:
				fmt.Printf("%s", e.Text)
			case event.ToolCallStartedEvent:
				fmt.Printf("calling %s with arguments %s", e.Name, string(e.Input))
			case event.ToolCallCompletedEvent:
				if e.IsError {
					fmt.Printf("tool %s failed", e.Name)
				} else {
					fmt.Printf("tool %s results: %s", e.Name, e.Text)
				}
			case event.UsageEvent:
				fmt.Printf("\n Usage: Input Tokens: %d, Output Tokens: %d", e.InputTokens, e.OutputTokens)
			case event.ErrorEvent:
				fmt.Printf("An error occured: %s", e.Err.Error())
			}
		}
		println()
	}
}
