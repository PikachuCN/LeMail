package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/PikachuCN/LeMail/internal/codeextract"
	"github.com/PikachuCN/LeMail/internal/config"
	"github.com/PikachuCN/LeMail/internal/mailstore"
)

type OpenAIExtractor struct {
	client *http.Client
}

type Extraction struct {
	Code       string  `json:"code"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

type extractionResponse struct {
	Codes []Extraction `json:"codes"`
}

type RegexSuggestion struct {
	Name       string  `json:"name"`
	Source     string  `json:"source"`
	Pattern    string  `json:"pattern"`
	SampleCode string  `json:"sampleCode"`
	Reason     string  `json:"reason"`
	Confidence float64 `json:"confidence"`
}

type regexSuggestionResponse struct {
	Suggestions []RegexSuggestion `json:"suggestions"`
}

type jsonTask struct {
	Instructions string
	Input        string
	SchemaName   string
	Schema       map[string]any
	ChatHint     string
}

type openAIHTTPError struct {
	API      string
	Status   int
	Endpoint string
	Body     string
}

func (e *openAIHTTPError) Error() string {
	body := strings.TrimSpace(e.Body)
	if body == "" {
		body = http.StatusText(e.Status)
	}
	return fmt.Sprintf("OpenAI %s API returned %d from %s: %s", e.API, e.Status, e.Endpoint, body)
}

func NewOpenAIExtractor() *OpenAIExtractor {
	return &OpenAIExtractor{client: &http.Client{}}
}

func (e *OpenAIExtractor) ExtractCodes(ctx context.Context, cfg config.Config, project config.CodeProject, msg mailstore.Message) ([]codeextract.Match, error) {
	if !cfg.OpenAI.Enabled {
		return nil, errors.New("OpenAI AI assisted extraction is disabled")
	}
	apiKey := strings.TrimSpace(cfg.OpenAI.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	if apiKey == "" {
		return nil, errors.New("OpenAI API key is not configured")
	}
	timeout, err := cfg.OpenAITimeout()
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	text, err := e.runJSONTask(ctx, cfg, apiKey, extractionTask(project, msg))
	if err != nil {
		return nil, err
	}
	parsed, err := parseExtractionResponse(text)
	if err != nil {
		return nil, fmt.Errorf("parse OpenAI extraction output: %w", err)
	}
	return matchesFromExtraction(parsed, project, msg), nil
}

func (e *OpenAIExtractor) SuggestRegex(ctx context.Context, cfg config.Config, project config.CodeProject, msg mailstore.Message) ([]RegexSuggestion, error) {
	if !cfg.OpenAI.Enabled {
		return nil, errors.New("OpenAI AI assisted regex generation is disabled")
	}
	apiKey := strings.TrimSpace(cfg.OpenAI.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	if apiKey == "" {
		return nil, errors.New("OpenAI API key is not configured")
	}
	timeout, err := cfg.OpenAITimeout()
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	text, err := e.runJSONTask(ctx, cfg, apiKey, regexSuggestionTask(project, msg))
	if err != nil {
		return nil, err
	}
	parsed, err := parseRegexSuggestionResponse(text)
	if err != nil {
		return nil, fmt.Errorf("parse OpenAI regex suggestion output: %w", err)
	}
	return normalizeRegexSuggestions(parsed.Suggestions), nil
}

func (e *OpenAIExtractor) runJSONTask(ctx context.Context, cfg config.Config, apiKey string, task jsonTask) (string, error) {
	switch strings.TrimSpace(cfg.OpenAI.APIMode) {
	case config.OpenAIAPIChatCompletions:
		return e.chatCompletionsOutput(ctx, cfg, apiKey, task)
	case config.OpenAIAPIResponses:
		return e.responsesOutput(ctx, cfg, apiKey, task)
	default:
		text, err := e.responsesOutput(ctx, cfg, apiKey, task)
		if err == nil || !shouldFallbackToChatCompletions(err) {
			return text, err
		}
		chatText, chatErr := e.chatCompletionsOutput(ctx, cfg, apiKey, task)
		if chatErr != nil {
			return "", fmt.Errorf("%w; Chat Completions fallback also failed: %v", err, chatErr)
		}
		return chatText, nil
	}
}

func (e *OpenAIExtractor) responsesOutput(ctx context.Context, cfg config.Config, apiKey string, task jsonTask) (string, error) {
	body, err := json.Marshal(responseRequest{
		Model:        strings.TrimSpace(cfg.OpenAI.Model),
		Instructions: task.Instructions,
		Input:        task.Input,
		Text: responseText{
			Format: responseFormat{
				Type:   "json_schema",
				Name:   task.SchemaName,
				Strict: true,
				Schema: task.Schema,
			},
		},
	})
	if err != nil {
		return "", err
	}

	endpoint := responsesURL(cfg.OpenAI.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &openAIHTTPError{API: "Responses", Status: resp.StatusCode, Endpoint: endpoint, Body: string(data)}
	}
	text := outputText(data)
	if strings.TrimSpace(text) == "" {
		return "", errors.New("OpenAI Responses API returned empty output")
	}
	return text, nil
}

func (e *OpenAIExtractor) chatCompletionsOutput(ctx context.Context, cfg config.Config, apiKey string, task jsonTask) (string, error) {
	body, err := json.Marshal(chatCompletionRequest{
		Model: strings.TrimSpace(cfg.OpenAI.Model),
		Messages: []chatMessage{
			{Role: "system", Content: task.Instructions + " Return valid JSON only. " + task.ChatHint},
			{Role: "user", Content: task.Input},
		},
		ResponseFormat: map[string]string{"type": "json_object"},
	})
	if err != nil {
		return "", err
	}
	endpoint := chatCompletionsURL(cfg.OpenAI.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &openAIHTTPError{API: "Chat Completions", Status: resp.StatusCode, Endpoint: endpoint, Body: string(data)}
	}
	text := chatCompletionText(data)
	if strings.TrimSpace(text) == "" {
		return "", errors.New("OpenAI Chat Completions API returned empty output")
	}
	return text, nil
}

type responseRequest struct {
	Model        string       `json:"model"`
	Instructions string       `json:"instructions"`
	Input        string       `json:"input"`
	Text         responseText `json:"text"`
}

type responseText struct {
	Format responseFormat `json:"format"`
}

type responseFormat struct {
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

type chatCompletionRequest struct {
	Model          string            `json:"model"`
	Messages       []chatMessage     `json:"messages"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func extractionTask(project config.CodeProject, msg mailstore.Message) jsonTask {
	return jsonTask{
		Instructions: extractionInstructions(),
		Input:        prompt(project, msg),
		SchemaName:   "verification_code_extraction",
		Schema:       extractionSchema(),
		ChatHint:     `The JSON shape must be {"codes":[{"code":"123456","confidence":0.95,"reason":"short reason"}]}.`,
	}
}

func regexSuggestionTask(project config.CodeProject, msg mailstore.Message) jsonTask {
	return jsonTask{
		Instructions: regexSuggestionInstructions(),
		Input:        prompt(project, msg),
		SchemaName:   "verification_regex_suggestions",
		Schema:       regexSuggestionSchema(),
		ChatHint:     `The JSON shape must be {"suggestions":[{"name":"HTML h1 code","source":"html","pattern":"<h1[^>]*>\\s*(\\d{6})\\s*</h1>","sampleCode":"123456","reason":"short reason","confidence":0.92}]}.`,
	}
}

func extractionInstructions() string {
	return strings.Join([]string{
		"You extract verification codes from emails for an administrator testing a temporary mailbox rule.",
		"Return only codes that are explicitly present in the provided email content.",
		"Do not invent codes. If no code exists, return an empty codes array.",
		"Prefer one-time passwords, login codes, registration codes, and numeric verification codes.",
	}, " ")
}

func regexSuggestionInstructions() string {
	return strings.Join([]string{
		"You generate reusable Go regular expressions for extracting verification codes from emails.",
		"Return multiple candidate regex patterns, not a one-time extracted code.",
		"Patterns must be compatible with Go regexp RE2: no lookbehind, no backreferences, no PCRE-only syntax.",
		"Prefer exactly one capturing group that captures only the verification code.",
		"Include source as text, html, raw, or all. Choose the source where the pattern should run.",
		"Use the provided email content and current project filters to create practical patterns for similar future emails.",
		"Include the sampleCode that the pattern is expected to capture from the current email.",
		"Return three to five distinct regex suggestions when possible.",
		"If possible, provide context-specific patterns first, then a safer fallback pattern.",
	}, " ")
}

func extractionSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"codes": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"code":       map[string]any{"type": "string"},
						"confidence": map[string]any{"type": "number"},
						"reason":     map[string]any{"type": "string"},
					},
					"required":             []string{"code", "confidence", "reason"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"codes"},
		"additionalProperties": false,
	}
}

func regexSuggestionSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"suggestions": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":       map[string]any{"type": "string"},
						"source":     map[string]any{"type": "string"},
						"pattern":    map[string]any{"type": "string"},
						"sampleCode": map[string]any{"type": "string"},
						"reason":     map[string]any{"type": "string"},
						"confidence": map[string]any{"type": "number"},
					},
					"required":             []string{"name", "source", "pattern", "sampleCode", "reason", "confidence"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"suggestions"},
		"additionalProperties": false,
	}
}

func prompt(project config.CodeProject, msg mailstore.Message) string {
	return fmt.Sprintf(`Project:
name: %s
description: %s
fromPattern: %s
subjectContains: %s
codePattern: %s
source: %s

Email:
from: %s
to: %s
subject: %s
text:
%s

html:
%s

raw:
%s`, project.Name, project.Description, project.FromPattern, project.Subject, project.CodePattern, project.Source, msg.From, strings.Join(msg.To, ", "), msg.Subject, limit(msg.Text), limit(msg.HTML), limit(msg.Raw))
}

func responsesURL(baseURL string) string {
	return openAIEndpointURL(baseURL, "responses")
}

func chatCompletionsURL(baseURL string) string {
	return openAIEndpointURL(baseURL, "chat/completions")
}

func openAIEndpointURL(baseURL string, endpoint string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	endpointPath := "/" + strings.Trim(strings.TrimSpace(endpoint), "/")
	parsed, err := url.Parse(baseURL)
	if err == nil {
		path := strings.TrimRight(parsed.Path, "/")
		if strings.EqualFold(parsed.Host, "api.openai.com") && path == "" {
			path = "/v1"
		}
		if strings.HasSuffix(path, endpointPath) {
			parsed.Path = path
			return parsed.String()
		}
		path = strings.TrimSuffix(path, "/responses")
		path = strings.TrimSuffix(path, "/chat/completions")
		parsed.Path = strings.TrimRight(path, "/") + endpointPath
		return parsed.String()
	}
	return baseURL + endpointPath
}

func outputText(data []byte) string {
	var payload struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return ""
	}
	if strings.TrimSpace(payload.OutputText) != "" {
		return payload.OutputText
	}
	var builder strings.Builder
	for _, output := range payload.Output {
		for _, content := range output.Content {
			if content.Text != "" {
				builder.WriteString(content.Text)
			}
		}
	}
	return builder.String()
}

func chatCompletionText(data []byte) string {
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return ""
	}
	var builder strings.Builder
	for _, choice := range payload.Choices {
		if choice.Message.Content != "" {
			builder.WriteString(choice.Message.Content)
		}
	}
	return builder.String()
}

func parseExtractionResponse(text string) (extractionResponse, error) {
	var parsed extractionResponse
	candidate := strings.TrimSpace(text)
	if err := json.Unmarshal([]byte(candidate), &parsed); err == nil {
		return parsed, nil
	}
	start := strings.Index(candidate, "{")
	end := strings.LastIndex(candidate, "}")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(candidate[start:end+1]), &parsed); err == nil {
			return parsed, nil
		}
	}
	return parsed, fmt.Errorf("invalid extraction JSON: %s", candidate)
}

func parseRegexSuggestionResponse(text string) (regexSuggestionResponse, error) {
	var parsed regexSuggestionResponse
	candidate := strings.TrimSpace(text)
	if err := json.Unmarshal([]byte(candidate), &parsed); err == nil {
		return parsed, nil
	}
	start := strings.Index(candidate, "{")
	end := strings.LastIndex(candidate, "}")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(candidate[start:end+1]), &parsed); err == nil {
			return parsed, nil
		}
	}
	return parsed, fmt.Errorf("invalid regex suggestion JSON: %s", candidate)
}

func normalizeRegexSuggestions(items []RegexSuggestion) []RegexSuggestion {
	out := make([]RegexSuggestion, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		item.Pattern = strings.TrimSpace(item.Pattern)
		if item.Pattern == "" {
			continue
		}
		if _, err := regexp.Compile(item.Pattern); err != nil {
			continue
		}
		if _, ok := seen[item.Pattern]; ok {
			continue
		}
		seen[item.Pattern] = struct{}{}
		item.Name = strings.TrimSpace(item.Name)
		if item.Name == "" {
			item.Name = "验证码正则"
		}
		item.Source = normalizeSuggestionSource(item.Source)
		item.SampleCode = strings.TrimSpace(item.SampleCode)
		item.Reason = strings.TrimSpace(item.Reason)
		if item.Confidence < 0 {
			item.Confidence = 0
		}
		if item.Confidence > 1 {
			item.Confidence = 1
		}
		out = append(out, item)
	}
	return out
}

func normalizeSuggestionSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "text", "html", "raw", "all":
		return strings.ToLower(strings.TrimSpace(source))
	default:
		return "all"
	}
}

func matchesFromExtraction(parsed extractionResponse, project config.CodeProject, msg mailstore.Message) []codeextract.Match {
	matches := make([]codeextract.Match, 0, len(parsed.Codes)*len(msg.To))
	for _, item := range parsed.Codes {
		code := strings.TrimSpace(item.Code)
		if code == "" {
			continue
		}
		for _, mailbox := range msg.To {
			matches = append(matches, codeextract.Match{
				ProjectID:   project.ID,
				ProjectName: project.Name,
				Mailbox:     mailbox,
				Code:        code,
				Subject:     msg.Subject,
				From:        msg.From,
				ReceivedAt:  msg.ReceivedAt,
				MessageID:   msg.ID,
			})
		}
	}
	return matches
}

func shouldFallbackToChatCompletions(err error) bool {
	var apiErr *openAIHTTPError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.API != "Responses" {
		return false
	}
	body := strings.ToLower(apiErr.Body)
	return apiErr.Status == http.StatusNotFound ||
		apiErr.Status == http.StatusMethodNotAllowed ||
		apiErr.Status == http.StatusNotImplemented ||
		strings.Contains(body, "not found") ||
		strings.Contains(body, "unsupported")
}

func limit(value string) string {
	const max = 30000
	if len(value) <= max {
		return value
	}
	return value[:max] + "\n...[truncated]"
}
