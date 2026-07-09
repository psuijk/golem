package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/psuijk/golem/internal/agent"
	"github.com/psuijk/golem/internal/config"
	"github.com/psuijk/golem/internal/conversation"
	"github.com/psuijk/golem/internal/sandbox"
	"github.com/psuijk/golem/internal/terminal"
	"github.com/psuijk/golem/internal/tool"
	"github.com/psuijk/golem/internal/tools/bash"
	"github.com/psuijk/golem/internal/tools/editfile"
	"github.com/psuijk/golem/internal/tools/readfile"
	"github.com/psuijk/golem/internal/tools/writefile"
)

func main() {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("get working directory: %v", err)
	}
	playgrounDir := filepath.Join(wd, "playground")

	r := agent.NewResolver()

	s, err := config.Load(playgrounDir)
	if err != nil {
		log.Fatalf("load golem settings: %v", err)
	}

	a, err := agent.New(agent.Config{
		Resolver: r,
		Tools: []tool.Interface{
			bash.New(30 * time.Second),
			readfile.New(1 << 20),
			writefile.New(),
			editfile.New(1 << 20),
		},
		Store: conversation.New(),
		Boundaries: sandbox.NewBoundaries(
			append(s.Boundaries, sandbox.PathRule{Path: playgrounDir, Access: sandbox.ReadWrite}),
		),
		Permissions:       s.Permissions,
		OnPermissionGrant: func(permKey string) error { return config.AddPermission(playgrounDir, permKey) },
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
