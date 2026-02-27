package summarizer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

const systemPrompt = "You are a technical project manager writing a status update on behalf of a single individual contributor. Translate the following technical git commits into a single, professional status update. Use action-oriented sentences without a subject (e.g. 'Added support for...', 'Fixed the null pointer...', 'Refactored the auth module...'). Focus on business value and functional impact. Use plain text only — no markdown, no bullet symbols, no bold, no headers. Write in short, plain sentences separated by newlines for distinct updates. Do not mention file names or internal code structures. Never use 'I', 'we', 'the team', or any subject pronoun."

const standupSystemPrompt = "You are an individual developer giving a brief daily standup update. Summarize the following git commits using action-oriented sentences without a subject (e.g. 'Worked on...', 'Fixed...', 'Added...'). Keep it short, informal, and conversational — focus on what was done and any relevant context. Group related work together. Write in plain text only, no markdown, no bullet symbols. Use short paragraphs or line breaks to separate distinct topics. Never use 'I', 'we', 'the team', or any subject pronoun."

// LLMSummarizer implements Summarizer supporting OpenAI, Gemini, and AWS Bedrock.
type LLMSummarizer struct {
	Provider   string // "openai", "gemini", or "bedrock"
	APIKey     string
	Model      string
	AWSRegion  string
	httpClient *http.Client
}

// NewLLMSummarizer creates a new LLMSummarizer.
func NewLLMSummarizer(provider, apiKey, model, awsRegion string) *LLMSummarizer {
	return &LLMSummarizer{
		Provider:   provider,
		APIKey:     apiKey,
		Model:      model,
		AWSRegion:  awsRegion,
		httpClient: &http.Client{},
	}
}

// Summarize calls the configured LLM provider and returns a professional summary.
// ticketTitle and ticketDescription are optional Jira ticket context; pass empty strings if unavailable.
func (l *LLMSummarizer) Summarize(commits []string, ticketTitle, ticketDescription string) (string, error) {
	userContent := strings.Join(commits, "\n---\n")

	if ticketTitle != "" || ticketDescription != "" {
		var ctx strings.Builder
		ctx.WriteString("Ticket context:\n")
		if ticketTitle != "" {
			ctx.WriteString("Title: " + ticketTitle + "\n")
		}
		if ticketDescription != "" {
			ctx.WriteString("Description: " + strings.TrimSpace(ticketDescription) + "\n")
		}
		ctx.WriteString("\nCommits:\n")
		ctx.WriteString(userContent)
		userContent = ctx.String()
	}

	return l.summarize(systemPrompt, userContent)
}

// SummarizeStandup summarizes all commits in an informal standup style.
// All commits are included regardless of Jira-Ticket footers.
func (l *LLMSummarizer) SummarizeStandup(commits []string) (string, error) {
	return l.summarize(standupSystemPrompt, strings.Join(commits, "\n---\n"))
}

func (l *LLMSummarizer) summarize(sysPrompt, userContent string) (string, error) {
	switch l.Provider {
	case "openai":
		return l.summarizeOpenAI(sysPrompt, userContent)
	case "gemini":
		return l.summarizeGemini(sysPrompt, userContent)
	case "bedrock":
		return l.summarizeBedrock(sysPrompt, userContent)
	default:
		return "", fmt.Errorf("unsupported LLM_PROVIDER %q: must be \"openai\", \"gemini\", or \"bedrock\"", l.Provider)
	}
}

// --- OpenAI ---

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (l *LLMSummarizer) summarizeOpenAI(sysPrompt, userContent string) (string, error) {
	model := l.Model
	if model == "" {
		model = "gpt-4o-mini"
	}

	reqBody := openAIRequest{
		Model: model,
		Messages: []openAIMessage{
			{Role: "system", Content: sysPrompt},
			{Role: "user", Content: userContent},
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling OpenAI request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("creating OpenAI request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+l.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling OpenAI API: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("[WARN] closing response body: %v", err)
		}
	}()

	var result openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding OpenAI response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("OpenAI error: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("OpenAI returned no choices")
	}

	return result.Choices[0].Message.Content, nil
}

// --- Gemini ---

type geminiRequest struct {
	SystemInstruction geminiContent   `json:"system_instruction"`
	Contents          []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (l *LLMSummarizer) summarizeGemini(sysPrompt, userContent string) (string, error) {
	model := l.Model
	if model == "" {
		model = "gemini-1.5-flash"
	}

	reqBody := geminiRequest{
		SystemInstruction: geminiContent{Parts: []geminiPart{{Text: sysPrompt}}},
		Contents:          []geminiContent{{Parts: []geminiPart{{Text: userContent}}}},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling Gemini request: %w", err)
	}

	apiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, l.APIKey)
	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("creating Gemini request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling Gemini API: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("[WARN] closing response body: %v", err)
		}
	}()

	var result geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding Gemini response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("gemini error: %s", result.Error.Message)
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini returned no content")
	}

	return result.Candidates[0].Content.Parts[0].Text, nil
}

// --- AWS Bedrock (Converse API) ---

func (l *LLMSummarizer) summarizeBedrock(sysPrompt, userContent string) (string, error) {
	if l.Model == "" {
		return "", fmt.Errorf("LLM_MODEL is required for the bedrock provider (e.g. anthropic.claude-3-5-sonnet-20241022-v2:0)")
	}

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(l.AWSRegion),
	)
	if err != nil {
		return "", fmt.Errorf("loading AWS config: %w", err)
	}

	client := bedrockruntime.NewFromConfig(cfg)

	input := &bedrockruntime.ConverseInput{
		ModelId: aws.String(l.Model),
		System: []types.SystemContentBlock{
			&types.SystemContentBlockMemberText{Value: sysPrompt},
		},
		Messages: []types.Message{
			{
				Role: types.ConversationRoleUser,
				Content: []types.ContentBlock{
					&types.ContentBlockMemberText{Value: userContent},
				},
			},
		},
	}

	output, err := client.Converse(context.Background(), input)
	if err != nil {
		return "", fmt.Errorf("calling Bedrock Converse API: %w", err)
	}

	resp, ok := output.Output.(*types.ConverseOutputMemberMessage)
	if !ok || len(resp.Value.Content) == 0 {
		return "", fmt.Errorf("bedrock returned no content")
	}

	textBlock, ok := resp.Value.Content[0].(*types.ContentBlockMemberText)
	if !ok {
		return "", fmt.Errorf("bedrock response content is not text")
	}

	return textBlock.Value, nil
}
