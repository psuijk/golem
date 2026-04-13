package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/psuijk/golem/internal/tool"
	"github.com/psuijk/golem/internal/tools/bash"
	"github.com/psuijk/golem/internal/tools/readfile"
)

func main() {

	ctx := context.Background()

	r := tool.NewRegistry()

	if err := r.Register(bash.New(30 * time.Second)); err != nil {
		log.Fatalf("register bash tool: %v", err)
	}

	if err := r.Register(readfile.New(1 << 20)); err != nil {
		log.Fatalf("register readfile tool: %v", err)
	}

	d := tool.NewDispatcher(r)

	result, err := d.Dispatch(ctx, "bash", json.RawMessage(`{"command": "echo hello world"}`))
	if err != nil {
		log.Fatalf("dispatch: %v", err)
	}

	fmt.Printf("text:    %s\n", result.Text)
	fmt.Printf("isError: %v\n", result.IsError)

	result2, err := d.Dispatch(ctx, "readfile", json.RawMessage(`{"path": "go.mod"}`))
	if err != nil {
		log.Fatalf("dispatch: %v", err)
	}

	fmt.Printf("text:    %s\n", result2.Text)
	fmt.Printf("isError: %v\n", result2.IsError)
}
