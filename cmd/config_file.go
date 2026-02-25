package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type fileConfig struct {
	GitHubToken string `json:"github_token"`
	GitHubUser  string `json:"github_user"`
	JiraURL     string `json:"jira_url"`
	JiraUser    string `json:"jira_user"`
	JiraToken   string `json:"jira_token"`
	LLMProvider string `json:"llm_provider"`
	LLMAPIKey   string `json:"llm_api_key"`
	LLMModel    string `json:"llm_model"`
	AWSRegion   string `json:"aws_region"`
}

func configDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "autobs")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".config/autobs"
	}
	return filepath.Join(home, ".config", "autobs")
}

func configFilePath() string {
	return filepath.Join(configDir(), "config.json")
}

func loadConfigFile() (*fileConfig, error) {
	data, err := os.ReadFile(configFilePath())
	if err != nil {
		return nil, err
	}
	var cfg fileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveConfigFile(cfg *fileConfig) error {
	if err := os.MkdirAll(configDir(), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configFilePath(), data, 0600)
}
