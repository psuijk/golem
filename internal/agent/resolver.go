package agent

import (
	"fmt"
	"os"

	"github.com/psuijk/golem/internal/llm"
	"github.com/psuijk/golem/internal/llm/anthropic"
	"github.com/psuijk/golem/internal/llm/ollama"
)

// Resolver is the default ModelResolver implementation. It checks which
// providers are available (via API keys or reachable servers) and maps
// model IDs to the appropriate provider. Providers are lazily created
// and cached for reuse across calls.
type Resolver struct {
	providers map[string]llm.Provider
}

// NewResolver returns a Resolver ready to discover available providers.
func NewResolver() *Resolver {
	return &Resolver{providers: make(map[string]llm.Provider)}
}

// Resolve returns the provider for the given model ID. It checks cached
// providers first, then probes available backends (Anthropic, Ollama)
// to find one that serves the requested model.
func (r *Resolver) Resolve(modelID string) (llm.Provider, error) {
	if p, ok := r.providers[modelID]; ok {
		return p, nil
	}

	if anthropic.Available() {
		for _, m := range anthropic.Models {
			if modelID == m.ID {
				apiKey := os.Getenv("ANTHROPIC_API_KEY")
				p, err := anthropic.New(nil, apiKey, "")
				if err != nil {
					return nil, fmt.Errorf("resolver: getting anthropic provider: %w", err)
				}
				r.providers[modelID] = p
				return p, nil
			}
		}
	}

	if ollama.Available("") {
		for _, m := range ollama.Models {
			if modelID == m.ID {
				p := ollama.New(nil, "")

				return p, nil
			}
		}
	}

	return nil, fmt.Errorf("resolver: model %q not found", modelID)
}
