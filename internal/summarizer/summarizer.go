package summarizer

// Summarizer defines the interface for LLM summarization integrations.
type Summarizer interface {
	Summarize(commits []string, ticketTitle, ticketDescription string) (string, error)
}
