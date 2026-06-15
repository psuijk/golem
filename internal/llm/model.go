package llm

// Model describes an LLM model's identity and capabilities. Provider
// packages export concrete instances (e.g. anthropic.Sonnet46) and the
// resolver uses them to map model IDs to providers.
type Model struct {
	ID              string
	Name            string
	MaxInputTokens  int
	MaxOutputTokens int
	Thinking        bool
}
