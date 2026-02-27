package cmd

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cfcolaco/autobs/internal/summarizer"
	"github.com/cfcolaco/autobs/internal/tracker"
	"github.com/cfcolaco/autobs/internal/vcs"
	"github.com/cfcolaco/autobs/pkg/models"
	"github.com/spf13/cobra"
)

var dryRun bool
var yesterday bool
var standup bool
var includePRs bool

var rootCmd = &cobra.Command{
	Use:   "autobs",
	Short: "Aggregates daily commits and posts summaries to Jira tickets",
	Long: `autobs is a CLI tool that:

  1. Fetches all your GitHub commits from today
  2. Groups them by Jira ticket (reads "Jira-Ticket: PROJ-123" from commit footers)
  3. Summarizes each ticket's commits using an LLM (OpenAI or Gemini)
  4. Posts the professional summary as a comment on each Jira ticket

Required environment variables:

  GITHUB_TOKEN    GitHub personal access token (needs repo + read:user scopes)
  GITHUB_USER     Your GitHub username
  JIRA_URL        Base URL of your Jira instance (e.g. https://yourorg.atlassian.net)
  JIRA_USER       Your Jira account email
  JIRA_TOKEN      Jira API token (https://id.atlassian.com/manage-profile/security/api-tokens)
  LLM_PROVIDER    LLM provider to use: "openai" or "gemini"
  LLM_API_KEY     API key for the chosen LLM provider

Commit footer format (required for ticket linking):

  Jira-Ticket: PROJ-123

Example usage:

  export GITHUB_TOKEN=ghp_...
  export GITHUB_USER=johndoe
  export JIRA_URL=https://myorg.atlassian.net
  export JIRA_USER=john@myorg.com
  export JIRA_TOKEN=ATATT3x...
  export LLM_PROVIDER=openai
  export LLM_API_KEY=sk-...

  autobs`,
	RunE: runE,
}

func init() {
	rootCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Summarize commits but print output instead of posting to Jira")
	rootCmd.Flags().BoolVar(&yesterday, "yesterday", false, "Fetch commits from yesterday instead of today")
	rootCmd.Flags().BoolVar(&standup, "standup", false, "Print a standup-style summary of all commits (skips Jira posting)")
	rootCmd.Flags().BoolVar(&includePRs, "include-prs", false, "Include commits from currently open PRs (drafts included)")
}

// Execute is the entry point called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runE(cmd *cobra.Command, args []string) error {
	// Load and validate required environment variables.
	env, err := loadEnv(standup)
	if err != nil {
		return err
	}

	// Instantiate providers.
	githubProvider := vcs.NewGitHubProvider(env.githubToken)
	llmSummarizer := summarizer.NewLLMSummarizer(env.llmProvider, env.llmAPIKey, env.llmModel, env.awsRegion)
	jiraTracker := tracker.NewJiraTracker(env.jiraURL, env.jiraUser, env.jiraToken)

	if dryRun {
		fmt.Println("--- DRY RUN — nothing will be posted to Jira ---")
	}

	// Collect commits for the target day.
	now := time.Now()
	year, month, day := now.Date()
	today := time.Date(year, month, day, 0, 0, 0, 0, now.Location())
	var since, until time.Time
	if yesterday {
		since = today.AddDate(0, 0, -1)
		until = today
	} else {
		since = today
	}
	commits, err := githubProvider.GetCommits(since, until, env.githubUser)
	if err != nil {
		return fmt.Errorf("fetching commits: %w", err)
	}

	if includePRs {
		prCommits, err := githubProvider.GetOpenPRCommits(env.githubUser)
		if err != nil {
			log.Printf("[WARN] could not fetch open PR commits: %v", err)
		} else {
			// Deduplicate by SHA before merging.
			seen := make(map[string]bool, len(commits))
			for _, c := range commits {
				seen[c.SHA] = true
			}
			for _, c := range prCommits {
				if !seen[c.SHA] {
					commits = append(commits, c)
				}
			}
		}
	}

	fmt.Printf("Found %d commit(s) from GitHub for user %q on %s.\n", len(commits), env.githubUser, since.Format("2006-01-02"))

	// Standup mode: summarize all commits in an informal style and print; never post to Jira.
	if standup {
		if len(commits) == 0 {
			fmt.Printf("No commits found for %s.\n", since.Format("2006-01-02"))
			return nil
		}
		messages := make([]string, 0, len(commits))
		for _, c := range commits {
			messages = append(messages, c.Message)
		}
		summary, err := llmSummarizer.SummarizeStandup(messages)
		if err != nil {
			return fmt.Errorf("summarizing standup: %w", err)
		}
		fmt.Println("\n=== Standup Summary ===")
		fmt.Println()
		for _, line := range strings.Split(strings.TrimSpace(summary), "\n") {
			fmt.Println(line)
		}
		fmt.Println()
		return nil
	}

	// Extract Jira ticket IDs from commit message footers and group by ticket.
	jiraTicketRe := regexp.MustCompile(`Jira-Ticket:\s*([A-Z]+-\d+)`)
	ticketCommits := make(map[string][]models.Commit)
	for _, c := range commits {
		m := jiraTicketRe.FindStringSubmatch(c.Message)
		if len(m) == 2 {
			ticketCommits[m[1]] = append(ticketCommits[m[1]], c)
		}
	}

	if len(commits) > 0 && len(ticketCommits) == 0 {
		fmt.Println("No commits had a 'Jira-Ticket: PROJ-123' footer — nothing to post.")
		fmt.Println("Tip: add a footer to your commits like:\n\n  Jira-Ticket: PROJ-123")
		return nil
	}

	if len(ticketCommits) == 0 {
		fmt.Printf("No commits found for %s.\n", since.Format("2006-01-02"))
		fmt.Println("Tips:")
		fmt.Println("  • Check GITHUB_USER matches your GitHub login exactly")
		fmt.Println("  • If your commits are in private org repos, your token needs the 'repo' scope")
		fmt.Println("  • Try using the gh CLI token which has the right scopes:")
		fmt.Println("      export GITHUB_TOKEN=$(gh auth token)")
		return nil
	}

	fmt.Printf("%d unique ticket(s) found: ", len(ticketCommits))
	for tid := range ticketCommits {
		fmt.Printf("%s ", tid)
	}
	fmt.Println()

	// Concurrently summarize and (optionally) post comments for each ticket.
	type result struct {
		ticketID string
		commits  []models.Commit
		summary  string
		err      error
	}

	results := make(chan result, len(ticketCommits))
	var wg sync.WaitGroup

	for ticketID, ticketCmts := range ticketCommits {
		wg.Add(1)
		go func(tid string, cmts []models.Commit) {
			defer wg.Done()

			messages := make([]string, len(cmts))
			for i, c := range cmts {
				messages[i] = c.Message
			}

			// Fetch ticket context for better summaries; proceed even if it fails.
			var ticketTitle, ticketDescription string
			if info, err := jiraTracker.GetTicket(tid); err != nil {
				log.Printf("[WARN] could not fetch ticket info for %s: %v", tid, err)
			} else {
				ticketTitle = info.Title
				ticketDescription = info.Description
			}

			summary, err := llmSummarizer.Summarize(messages, ticketTitle, ticketDescription)
			if err != nil {
				log.Printf("[ERROR] summarizing %s: %v", tid, err)
				results <- result{ticketID: tid, commits: cmts, err: err}
				return
			}

			if !dryRun {
				if err := jiraTracker.PostComment(tid, summary); err != nil {
					log.Printf("[ERROR] posting comment to %s: %v", tid, err)
					results <- result{ticketID: tid, commits: cmts, err: err}
					return
				}
			}

			results <- result{ticketID: tid, commits: cmts, summary: summary}
		}(ticketID, ticketCmts)
	}

	wg.Wait()
	close(results)

	// Print final report.
	if dryRun {
		fmt.Println("\n=== autobs Dry Run Preview ===")
		for r := range results {
			if r.err != nil {
				fmt.Printf("\n[ERROR] %s — %v\n", r.ticketID, r.err)
			} else {
				fmt.Printf("\n┌─ %s\n", r.ticketID)
				for _, line := range strings.Split(strings.TrimSpace(r.summary), "\n") {
					fmt.Printf("│  %s\n", line)
				}
				fmt.Println("│")
				fmt.Println("│  Commits:")
				for _, c := range r.commits {
					sha := c.SHA
					if len(sha) > 7 {
						sha = sha[:7]
					}
					fmt.Printf("│    %s  %s\n", sha, c.Repository)
				}
				fmt.Println("└─ (not posted)")
			}
		}
	} else {
		fmt.Println("\n=== autobs Report ===")
		for r := range results {
			if r.err != nil {
				fmt.Printf("  [FAILED]  %s — %v\n", r.ticketID, r.err)
			} else {
				fmt.Printf("  [UPDATED] %s\n", r.ticketID)
			}
		}
	}

	return nil
}

type envConfig struct {
	githubToken string
	githubUser  string
	jiraURL     string
	jiraUser    string
	jiraToken   string
	llmAPIKey   string
	llmProvider string
	llmModel    string
	awsRegion   string
}

// resolve returns the env var value if set, otherwise falls back to the config file value.
func resolve(envKey, fileVal string) string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return fileVal
}

func loadEnv(skipJira bool) (*envConfig, error) {
	// Load config file as fallback (errors silently ignored if file doesn't exist).
	fileCfg, _ := loadConfigFile()
	if fileCfg == nil {
		fileCfg = &fileConfig{}
	}

	cfg := &envConfig{
		githubToken: resolve("GITHUB_TOKEN", fileCfg.GitHubToken),
		githubUser:  resolve("GITHUB_USER", fileCfg.GitHubUser),
		jiraURL:     resolve("JIRA_URL", fileCfg.JiraURL),
		jiraUser:    resolve("JIRA_USER", fileCfg.JiraUser),
		jiraToken:   resolve("JIRA_TOKEN", fileCfg.JiraToken),
		llmAPIKey:   resolve("LLM_API_KEY", fileCfg.LLMAPIKey),
		llmProvider: resolve("LLM_PROVIDER", fileCfg.LLMProvider),
		llmModel:    resolve("LLM_MODEL", fileCfg.LLMModel),
		awsRegion:   resolve("AWS_REGION", fileCfg.AWSRegion),
	}

	// Always-required fields.
	type field struct{ name, val string }
	always := []field{
		{"GITHUB_TOKEN", cfg.githubToken},
		{"GITHUB_USER", cfg.githubUser},
		{"LLM_PROVIDER", cfg.llmProvider},
	}
	if !skipJira {
		always = append(always,
			field{"JIRA_URL", cfg.jiraURL},
			field{"JIRA_USER", cfg.jiraUser},
			field{"JIRA_TOKEN", cfg.jiraToken},
		)
	}
	missing := []string{}
	for _, f := range always {
		if f.val == "" {
			missing = append(missing, f.name)
		}
	}

	// Provider-specific required fields.
	switch cfg.llmProvider {
	case "bedrock":
		if cfg.awsRegion == "" {
			missing = append(missing, "AWS_REGION")
		}
		if cfg.llmModel == "" {
			missing = append(missing, "LLM_MODEL")
		}
	default:
		if cfg.llmAPIKey == "" {
			missing = append(missing, "LLM_API_KEY")
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required settings (set via env or run 'autobs configure'): %v", missing)
	}

	return cfg, nil
}
