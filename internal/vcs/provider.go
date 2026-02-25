package vcs

import (
	"time"

	"github.com/cfcolaco/autobs/pkg/models"
)

// VCSProvider defines the interface for version control system integrations.
type VCSProvider interface {
	GetCommits(since time.Time, user string) ([]models.Commit, error)
}
