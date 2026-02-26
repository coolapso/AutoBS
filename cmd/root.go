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
	"github.com/spf13/cobra"
)

var dryRun bool
var yesterday bool

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
}

// Execute is the entry point called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runE(cmd *cobra.Command, args []string) error {
	// Load and validate required environment variables.
	env, err := loadEnv()
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
	today := time.Now().Truncate(24 * time.Hour)
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

	fmt.Printf("Found %d commit(s) from GitHub for user %q on %s.\n", len(commits), env.githubUser, since.Format("2006-01-02"))

	// Extract Jira ticket IDs from commit message footers and group by ticket.
	jiraTicketRe := regexp.MustCompile(`Jira-Ticket:\s*([A-Z]+-\d+)`)
	ticketCommits := make(map[string][]string)
	for _, c := range commits {
		m := jiraTicketRe.FindStringSubmatch(c.Message)
		if len(m) == 2 {
			ticketCommits[m[1]] = append(ticketCommits[m[1]], c.Message)
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
		summary  string
		err      error
	}

	results := make(chan result, len(ticketCommits))
	var wg sync.WaitGroup

	for ticketID, messages := range ticketCommits {
		wg.Add(1)
		go func(tid string, msgs []string) {
			defer wg.Done()

			// Fetch ticket context for better summaries; proceed even if it fails.
			var ticketTitle, ticketDescription string
			if info, err := jiraTracker.GetTicket(tid); err != nil {
				log.Printf("[WARN] could not fetch ticket info for %s: %v", tid, err)
			} else {
				ticketTitle = info.Title
				ticketDescription = info.Description
			}

			summary, err := llmSummarizer.Summarize(msgs, ticketTitle, ticketDescription)
			if err != nil {
				log.Printf("[ERROR] summarizing %s: %v", tid, err)
				results <- result{ticketID: tid, err: err}
				return
			}

			if !dryRun {
				if err := jiraTracker.PostComment(tid, summary); err != nil {
					log.Printf("[ERROR] posting comment to %s: %v", tid, err)
					results <- result{ticketID: tid, err: err}
					return
				}
			}

			results <- result{ticketID: tid, summary: summary}
		}(ticketID, messages)
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

func loadEnv() (*envConfig, error) {
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
		{"JIRA_URL", cfg.jiraURL},
		{"JIRA_USER", cfg.jiraUser},
		{"JIRA_TOKEN", cfg.jiraToken},
		{"LLM_PROVIDER", cfg.llmProvider},
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
