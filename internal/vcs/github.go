package vcs

import (
	"context"
	"fmt"
	"time"

	"github.com/cfcolaco/autobs/pkg/models"
	gh "github.com/google/go-github/v65/github"
	"golang.org/x/oauth2"
)

// GitHubProvider implements VCSProvider using the go-github SDK.
type GitHubProvider struct {
	client *gh.Client
}

// NewGitHubProvider creates a new GitHubProvider authenticated with the given token.
func NewGitHubProvider(token string) *GitHubProvider {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	return &GitHubProvider{client: gh.NewClient(tc)}
}

// GetCommits fetches all commits authored by user on or after since, using the GitHub Search API.
// If until is non-zero, an upper bound is added to the query.
// Returned commits contain raw SHA and message only — ticket parsing is left to the caller.
func (g *GitHubProvider) GetCommits(since, until time.Time, user string) ([]models.Commit, error) {
	ctx := context.Background()
	query := fmt.Sprintf("author:%s author-date:>=%s", user, since.Format("2006-01-02"))
	if !until.IsZero() {
		query += fmt.Sprintf(" author-date:<%s", until.Format("2006-01-02"))
	}

	opts := &gh.SearchOptions{
		ListOptions: gh.ListOptions{PerPage: 100},
	}

	var commits []models.Commit
	for {
		result, resp, err := g.client.Search.Commits(ctx, query, opts)
		if err != nil {
			return nil, fmt.Errorf("searching GitHub commits: %w", err)
		}

		for _, item := range result.Commits {
			commits = append(commits, models.Commit{
				SHA:     item.GetSHA(),
				Message: item.GetCommit().GetMessage(),
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return commits, nil
}
