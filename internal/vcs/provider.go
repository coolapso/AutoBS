package vcs

import (
	"time"

	"github.com/cfcolaco/autobs/pkg/models"
)

// VCSProvider defines the interface for version control system integrations.
type VCSProvider interface {
	// GetCommits returns commits authored by user on or after since.
	// If until is non-zero, only commits before that time are returned.
	GetCommits(since, until time.Time, user string) ([]models.Commit, error)
}
