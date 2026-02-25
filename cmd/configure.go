package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Interactively create or update the config file",
	Long: `Prompts for each required setting and saves them to the config file at:

  ` + configFilePath() + `

Environment variables always take precedence over the config file.
Run this command once to avoid setting env vars on every session.`,
	RunE: runConfigure,
}

func init() {
	rootCmd.AddCommand(configureCmd)
}

func runConfigure(cmd *cobra.Command, args []string) error {
	// Load existing config so we can show current values.
	existing, _ := loadConfigFile()
	if existing == nil {
		existing = &fileConfig{}
	}

	fmt.Println("Configure autobs")
	fmt.Printf("Settings will be saved to: %s\n", configFilePath())
	fmt.Println("Press Enter to keep the current value shown in [brackets].\n")

	provider := promptChoice("LLM_PROVIDER  (openai/gemini/bedrock)", existing.LLMProvider, []string{"openai", "gemini", "bedrock"})

	cfg := &fileConfig{
		GitHubToken: prompt("GITHUB_TOKEN  (personal access token)", existing.GitHubToken, true),
		GitHubUser:  prompt("GITHUB_USER   (your GitHub username)", existing.GitHubUser, false),
		JiraURL:     prompt("JIRA_URL      (e.g. https://yourorg.atlassian.net)", existing.JiraURL, false),
		JiraUser:    prompt("JIRA_USER     (your Jira account email)", existing.JiraUser, false),
		JiraToken:   prompt("JIRA_TOKEN    (Jira API token)", existing.JiraToken, true),
		LLMProvider: provider,
	}

	switch provider {
	case "bedrock":
		cfg.AWSRegion = prompt("AWS_REGION    (e.g. us-east-1)", existing.AWSRegion, false)
		cfg.LLMModel = prompt("LLM_MODEL     (e.g. anthropic.claude-3-5-sonnet-20241022-v2:0)", existing.LLMModel, false)
		fmt.Println("  Note: AWS credentials are read from the standard chain (env vars or ~/.aws/credentials).")
	default:
		cfg.LLMAPIKey = prompt("LLM_API_KEY   (API key for the chosen provider)", existing.LLMAPIKey, true)
		cfg.LLMModel = prompt("LLM_MODEL     (optional, leave blank for default)", existing.LLMModel, false)
	}

	if err := saveConfigFile(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("\nConfig saved to %s\n", configFilePath())
	return nil
}

func prompt(label, current string, secret bool) string {
	display := current
	if secret && current != "" {
		display = mask(current)
	}

	if display != "" {
		fmt.Printf("  %s [%s]: ", label, display)
	} else {
		fmt.Printf("  %s: ", label)
	}

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return current
	}
	return input
}

func promptChoice(label, current string, choices []string) string {
	for {
		val := prompt(fmt.Sprintf("%s (%s)", label, strings.Join(choices, "/")), current, false)
		if val == "" {
			return current
		}
		for _, c := range choices {
			if val == c {
				return val
			}
		}
		fmt.Printf("  Invalid choice. Must be one of: %s\n", strings.Join(choices, ", "))
	}
}

func mask(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return s[:4] + strings.Repeat("*", len(s)-4)
}
