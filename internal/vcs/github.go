package vcs

import (
	"context"
	"fmt"
	"log"
	"strings"
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
	var query string
	if !until.IsZero() {
		// Use the inclusive range syntax; until is exclusive (midnight of next day), so subtract one day for the end bound.
		query = fmt.Sprintf("author:%s author-date:%s..%s", user, since.Format("2006-01-02"), until.AddDate(0, 0, -1).Format("2006-01-02"))
	} else {
		query = fmt.Sprintf("author:%s author-date:>=%s", user, since.Format("2006-01-02"))
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
				SHA:        item.GetSHA(),
				Message:    item.GetCommit().GetMessage(),
				Repository: item.GetRepository().GetFullName(),
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return commits, nil
}

// GetOpenPRCommits fetches all commits from currently open PRs (including drafts) authored by user.
func (g *GitHubProvider) GetOpenPRCommits(user string) ([]models.Commit, error) {
	ctx := context.Background()
	query := fmt.Sprintf("is:pr is:open author:%s", user)
	searchOpts := &gh.SearchOptions{
		ListOptions: gh.ListOptions{PerPage: 100},
	}

	var commits []models.Commit
	seen := make(map[string]bool)

	for {
		result, resp, err := g.client.Search.Issues(ctx, query, searchOpts)
		if err != nil {
			return nil, fmt.Errorf("searching open PRs: %w", err)
		}

		for _, pr := range result.Issues {
			owner, repo, err := parseRepoFromURL(pr.GetRepositoryURL())
			if err != nil {
				log.Printf("[WARN] could not parse repo URL %s: %v", pr.GetRepositoryURL(), err)
				continue
			}

			prOpts := &gh.ListOptions{PerPage: 100}
			for {
				prCommits, prResp, err := g.client.PullRequests.ListCommits(ctx, owner, repo, pr.GetNumber(), prOpts)
				if err != nil {
					log.Printf("[WARN] could not fetch commits for PR #%d in %s/%s: %v", pr.GetNumber(), owner, repo, err)
					break
				}

				for _, c := range prCommits {
					sha := c.GetSHA()
					if !seen[sha] {
						seen[sha] = true
						commits = append(commits, models.Commit{
							SHA:        sha,
							Message:    c.GetCommit().GetMessage(),
							Repository: owner + "/" + repo,
							PRNumber:   pr.GetNumber(),
						})
					}
				}

				if prResp.NextPage == 0 {
					break
				}
				prOpts.Page = prResp.NextPage
			}
		}

		if resp.NextPage == 0 {
			break
		}
		searchOpts.Page = resp.NextPage
	}

	return commits, nil
}

// parseRepoFromURL parses "https://api.github.com/repos/{owner}/{repo}" into owner and repo.
func parseRepoFromURL(rawURL string) (string, string, error) {
	parts := strings.SplitN(rawURL, "/repos/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected repository URL format: %s", rawURL)
	}
	ownerRepo := strings.SplitN(parts[1], "/", 2)
	if len(ownerRepo) < 2 {
		return "", "", fmt.Errorf("unexpected owner/repo in URL: %s", parts[1])
	}
	return ownerRepo[0], ownerRepo[1], nil
}
