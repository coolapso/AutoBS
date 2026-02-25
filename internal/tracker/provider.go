package tracker

import "github.com/cfcolaco/autobs/pkg/models"

// TrackerProvider defines the interface for task tracker integrations.
type TrackerProvider interface {
	PostComment(ticketID string, body string) error
	GetTicket(ticketID string) (*models.TicketInfo, error)
}
