package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/psuijk/golem/internal/tool"
	"github.com/psuijk/golem/internal/tools/bash"
)

func main() {

	ctx := context.Background()

	r := tool.NewRegistry()

	if err := r.Register(bash.New(30 * time.Second)); err != nil {
		log.Fatalf("register bash tool: %v", err)
	}

	d := tool.NewDispatcher(r)

	result, err := d.Dispatch(ctx, "bash", json.RawMessage(`{"command": "echo hello world"}`))
	if err != nil {
		log.Fatalf("dispatch: %v", err)
	}

	fmt.Printf("text:    %s\n", result.Text)
	fmt.Printf("isError: %v\n", result.IsError)
}
