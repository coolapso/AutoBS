package models

// Commit represents a single VCS commit.
type Commit struct {
	SHA        string
	Message    string
	Repository string // owner/repo, e.g. "acme/infra"
}

// Summary holds the LLM-generated summary for a ticket.
type Summary struct {
	TicketID string
	Body     string
}

// TicketInfo holds metadata fetched from the tracker for context.
type TicketInfo struct {
	Title       string
	Description string
}
