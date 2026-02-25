package tracker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/cfcolaco/autobs/pkg/models"
)

// JiraTracker implements TrackerProvider for Jira.
type JiraTracker struct {
	BaseURL    string
	Email      string
	Token      string
	httpClient *http.Client
}

// NewJiraTracker creates a new JiraTracker.
func NewJiraTracker(baseURL, email, token string) *JiraTracker {
	return &JiraTracker{
		BaseURL:    baseURL,
		Email:      email,
		Token:      token,
		httpClient: &http.Client{},
	}
}

// GetTicket fetches the title and description of a Jira ticket.
func (j *JiraTracker) GetTicket(ticketID string) (*models.TicketInfo, error) {
	endpoint := fmt.Sprintf("%s/rest/api/3/issue/%s?fields=summary,description", j.BaseURL, ticketID)

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.SetBasicAuth(j.Email, j.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Jira API returned status %d for ticket %s", resp.StatusCode, ticketID)
	}

	var result struct {
		Fields struct {
			Summary     string                 `json:"summary"`
			Description map[string]interface{} `json:"description"`
		} `json:"fields"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &models.TicketInfo{
		Title:       result.Fields.Summary,
		Description: extractADFText(result.Fields.Description),
	}, nil
}

// extractADFText recursively extracts plain text from an Atlassian Document Format node.
func extractADFText(node map[string]interface{}) string {
	if node == nil {
		return ""
	}
	var sb strings.Builder
	if text, ok := node["text"].(string); ok {
		sb.WriteString(text)
	}
	if content, ok := node["content"].([]interface{}); ok {
		for _, child := range content {
			if childMap, ok := child.(map[string]interface{}); ok {
				sb.WriteString(extractADFText(childMap))
			}
		}
		// Add a newline after block-level nodes for readability.
		if t, ok := node["type"].(string); ok {
			switch t {
			case "paragraph", "heading", "bulletList", "orderedList", "listItem", "blockquote":
				sb.WriteString("\n")
			}
		}
	}
	return sb.String()
}

// PostComment posts a comment to the given Jira ticket using Atlassian Document Format (ADF).
func (j *JiraTracker) PostComment(ticketID string, body string) error {
	endpoint := fmt.Sprintf("%s/rest/api/3/issue/%s/comment", j.BaseURL, ticketID)

	payload := map[string]interface{}{
		"body": map[string]interface{}{
			"type":    "doc",
			"version": 1,
			"content": []map[string]interface{}{
				{
					"type": "paragraph",
					"content": []map[string]interface{}{
						{
							"type": "text",
							"text": body,
						},
					},
				},
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.SetBasicAuth(j.Email, j.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Jira API returned status %d for ticket %s", resp.StatusCode, ticketID)
	}

	return nil
}
