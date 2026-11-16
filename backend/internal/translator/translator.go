package translator

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultAPIURL     = "https://api.deepseek.com"
	DefaultModel      = "deepseek-flash"
	DefaultTargetLang = "zh-CN"

	defaultMaxBatchRunes = 2400
	defaultMaxBatchItems = 20
	perBatchTimeout      = 120 * time.Second
	probeTimeout         = 20 * time.Second
)

var (
	ErrMissingAPIKey = errors.New("translation api key is not configured")
	ErrInvalidAPIURL = errors.New("invalid translation api url")
	versionPathRe    = regexp.MustCompile(`(?i)/v\d+[a-z0-9_-]*$`)
	segmentMarkerRe  = regexp.MustCompile(`^\[\[(title|\d+)\]\]$`)
)

// Segment represents one translatable unit identified by a numeric block ID.
type Segment struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

// Config identifies the upstream LLM endpoint and credentials.
type Config struct {
	APIURL string
	APIKey string
	Model  string
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Error *upstreamErrorDetail `json:"error"`
}

type upstreamErrorEnvelope struct {
	Error *upstreamErrorDetail `json:"error"`
}

type upstreamErrorDetail struct {
	Message string `json:"message"`
}

type taggedItem struct {
	key  string
	text string
}

// ValidateAPIURL verifies that a non-empty URL uses http/https with a host.
// An empty string is allowed and means "use default DeepSeek endpoint".
func ValidateAPIURL(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	parsed, err := url.ParseRequestURI(trimmed)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidAPIURL, err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%w: unsupported scheme", ErrInvalidAPIURL)
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return fmt.Errorf("%w: host is required", ErrInvalidAPIURL)
	}
	return nil
}

// ResolveCompletionsURL normalizes a user-configured base URL or full endpoint
// into an OpenAI-compatible chat completions URL.
func ResolveCompletionsURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		trimmed = DefaultAPIURL
	}
	if err := ValidateAPIURL(trimmed); err != nil {
		return "", err
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidAPIURL, err)
	}

	cleanPath := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(cleanPath, "/chat/completions"):
		parsed.Path = cleanPath
	case versionPathRe.MatchString(cleanPath):
		parsed.Path = cleanPath + "/chat/completions"
	default:
		parsed.Path = cleanPath + "/v1/chat/completions"
	}
	return parsed.String(), nil
}

// EffectiveModel returns the configured model name or DefaultModel when empty.
func EffectiveModel(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return DefaultModel
	}
	return trimmed
}

// ComputeContentHash returns a deterministic SHA-256 hex digest covering the
// article title and every extracted translatable block in order.
func ComputeContentHash(title string, blocks []Segment) string {
	h := sha256.New()
	writeLenPrefixed(h, "title:"+strings.TrimSpace(title))
	for _, b := range blocks {
		writeLenPrefixed(h, strconv.Itoa(b.ID)+":"+b.Text)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ComputeListContentHash returns a deterministic SHA-256 hex digest for one
// article-list item's title and excerpt. The namespace prevents these cache
// entries from colliding with article-body translation hashes.
func ComputeListContentHash(title, summary string) string {
	h := sha256.New()
	writeLenPrefixed(h, "list-title:"+strings.TrimSpace(title))
	writeLenPrefixed(h, "list-summary:"+strings.TrimSpace(summary))
	return hex.EncodeToString(h.Sum(nil))
}

// ComputeSummaryHash returns a deterministic SHA-256 hex digest covering the
// article title and normalized plain-text content for AI summary caching.
func ComputeSummaryHash(title, content string) string {
	h := sha256.New()
	writeLenPrefixed(h, "summary-title:"+strings.TrimSpace(title))
	writeLenPrefixed(h, "summary-body:"+strings.TrimSpace(content))
	return hex.EncodeToString(h.Sum(nil))
}

func writeLenPrefixed(w io.Writer, s string) {
	_, _ = fmt.Fprintf(w, "%d:%s\n", len(s), s)
}

// BuildBatches splits title + body segments into size-bounded translation batches.
func BuildBatches(title string, blocks []Segment, maxRunes, maxItems int) [][]taggedItem {
	if maxRunes <= 0 {
		maxRunes = defaultMaxBatchRunes
	}
	if maxItems <= 0 {
		maxItems = defaultMaxBatchItems
	}

	items := make([]taggedItem, 0, len(blocks)+1)
	if trimmedTitle := strings.TrimSpace(title); trimmedTitle != "" {
		items = append(items, taggedItem{key: "title", text: trimmedTitle})
	}
	for _, b := range blocks {
		if strings.TrimSpace(b.Text) == "" {
			continue
		}
		items = append(items, taggedItem{key: strconv.Itoa(b.ID), text: b.Text})
	}
	if len(items) == 0 {
		return nil
	}

	var batches [][]taggedItem
	var current []taggedItem
	currentRunes := 0

	for _, item := range items {
		itemRunes := utf8.RuneCountInString(item.text) + 16
		if len(current) > 0 && (len(current) >= maxItems || currentRunes+itemRunes > maxRunes) {
			batches = append(batches, current)
			current = nil
			currentRunes = 0
		}
		current = append(current, item)
		currentRunes += itemRunes
	}
	if len(current) > 0 {
		batches = append(batches, current)
	}
	return batches
}

// Client calls an OpenAI-compatible DeepSeek endpoint.
type Client struct {
	httpClient *http.Client
}

// NewClient constructs a translator HTTP client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				TLSHandshakeTimeout:   15 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
			},
		},
	}
}

// TestConnection verifies the API endpoint, credentials, and model with a
// minimal non-streaming completion request.
func (c *Client) TestConnection(ctx context.Context, cfg Config) error {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return ErrMissingAPIKey
	}
	endpoint, err := ResolveCompletionsURL(cfg.APIURL)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	reqBody, err := json.Marshal(chatCompletionRequest{
		Model: EffectiveModel(cfg.Model),
		Messages: []chatMessage{
			{Role: "user", Content: "Reply with OK."},
		},
		Stream:    false,
		MaxTokens: 8,
	})
	if err != nil {
		return fmt.Errorf("marshal test request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("build test request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.APIKey))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseUpstreamHTTPError(resp.StatusCode, resp.Body)
	}
	return nil
}

// Result holds the complete translated output returned after streaming finishes.
type Result struct {
	TranslatedTitle string
	Blocks          []Segment
	Model           string
}

// TranslateArticle translates the title and blocks progressively using streaming
// responses, invoking onSegment as soon as each segment completes.
func (c *Client) TranslateArticle(
	ctx context.Context,
	cfg Config,
	title string,
	blocks []Segment,
	onSegment func(key string, text string) error,
) (*Result, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, ErrMissingAPIKey
	}
	endpoint, err := ResolveCompletionsURL(cfg.APIURL)
	if err != nil {
		return nil, err
	}
	modelName := EffectiveModel(cfg.Model)

	batches := BuildBatches(title, blocks, defaultMaxBatchRunes, defaultMaxBatchItems)
	if len(batches) == 0 {
		return &Result{Model: modelName}, nil
	}

	translatedMap := make(map[string]string, len(blocks)+1)

	for batchIdx, batch := range batches {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		expected := make(map[string]struct{}, len(batch))
		for _, item := range batch {
			expected[item.key] = struct{}{}
		}

		err := c.translateBatch(ctx, endpoint, cfg.APIKey, modelName, title, batchIdx > 0, batch, expected, func(key, text string) error {
			translatedMap[key] = text
			if onSegment != nil {
				return onSegment(key, text)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	outBlocks := make([]Segment, 0, len(blocks))
	for _, b := range blocks {
		k := strconv.Itoa(b.ID)
		if text, ok := translatedMap[k]; ok {
			outBlocks = append(outBlocks, Segment{ID: b.ID, Text: text})
		}
	}

	return &Result{
		TranslatedTitle: translatedMap["title"],
		Blocks:          outBlocks,
		Model:           modelName,
	}, nil
}

func (c *Client) translateBatch(
	ctx context.Context,
	endpoint string,
	apiKey string,
	modelName string,
	articleTitle string,
	isContinuation bool,
	batch []taggedItem,
	expected map[string]struct{},
	onSegment func(key, text string) error,
) error {
	batchCtx, cancel := context.WithTimeout(ctx, perBatchTimeout)
	defer cancel()

	var userPrompt strings.Builder
	if isContinuation && strings.TrimSpace(articleTitle) != "" {
		userPrompt.WriteString("Article title for context only (already translated, do not output again unless listed below): ")
		userPrompt.WriteString(strings.TrimSpace(articleTitle))
		userPrompt.WriteString("\n\n")
	}
	for i, item := range batch {
		if i > 0 {
			userPrompt.WriteString("\n")
		}
		userPrompt.WriteString("[[")
		userPrompt.WriteString(item.key)
		userPrompt.WriteString("]]\n")
		userPrompt.WriteString(item.text)
		userPrompt.WriteString("\n")
	}

	const systemPrompt = `You are a professional technical translator. Translate each segment into natural, fluent Simplified Chinese (简体中文).
Rules:
1. Every input segment begins with a marker line like [[title]] or [[0]]. Keep the exact same marker on its own line before its translation.
2. Never skip, merge, split, or reorder segments, and do not add commentary or markdown code fences around your output.
3. Preserve inline markup such as <a0>...</a0>, <a1>...</a1>, <code>...</code>, <strong>...</strong>, and <em>...</em> accurately (translate visible prose inside them while keeping identifiers/code inside <code>...</code> verbatim).
4. If a segment is already Simplified Chinese or consists purely of a code/product/version identifier, keep it concise and accurate.`

	reqBody := chatCompletionRequest{
		Model: modelName,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt.String()},
		},
		Stream:      true,
		Temperature: 0.2,
	}

	parser := &segmentStreamParser{
		expected:  expected,
		onSegment: onSegment,
	}

	if err := c.streamCompletion(batchCtx, endpoint, apiKey, reqBody, parser.Feed); err != nil {
		return err
	}
	if err := parser.Finish(); err != nil {
		return err
	}
	if len(parser.received) == 0 {
		return errors.New("upstream returned no translated segments")
	}
	var missing []string
	for _, item := range batch {
		if _, ok := parser.received[item.key]; !ok {
			missing = append(missing, item.key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("upstream response incomplete: missing translated segments (%s)", strings.Join(missing, ", "))
	}
	return nil
}

const maxSummarySourceRunes = 18000

// SummarizeArticle streams a concise Simplified Chinese summary of the article's
// title and full text, invoking onDelta for each streamed text chunk.
func (c *Client) SummarizeArticle(
	ctx context.Context,
	cfg Config,
	title string,
	content string,
	onDelta func(delta string) error,
) (summary string, modelName string, err error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return "", "", ErrMissingAPIKey
	}
	endpoint, err := ResolveCompletionsURL(cfg.APIURL)
	if err != nil {
		return "", "", err
	}
	modelName = EffectiveModel(cfg.Model)

	trimmedTitle := strings.TrimSpace(title)
	trimmedContent := strings.TrimSpace(content)
	if trimmedTitle == "" && trimmedContent == "" {
		return "", "", errors.New("empty summary content")
	}

	bodyText, truncated := truncateRunes(trimmedContent, maxSummarySourceRunes)
	var userPrompt strings.Builder
	if trimmedTitle != "" {
		userPrompt.WriteString("Title: ")
		userPrompt.WriteString(trimmedTitle)
		userPrompt.WriteString("\n\n")
	}
	if bodyText != "" {
		userPrompt.WriteString(bodyText)
		if truncated {
			userPrompt.WriteString("\n\n[Article body truncated for length]")
		}
	}

	const systemPrompt = `You are a concise, accurate reading assistant. Summarize the full article into natural Simplified Chinese (简体中文).
Rules:
1. Start with a 1-2 sentence core takeaway summarizing the central conclusion or news.
2. Follow with 3-5 concise Markdown bullet points (- ...) covering the key facts, arguments, numbers, or technical takeaways. Keep very short articles to 1-2 bullets.
3. Preserve technical terms, identifiers, product names, and numbers accurately; do not add filler preambles or speculate beyond the article text.`

	sumCtx, cancel := context.WithTimeout(ctx, perBatchTimeout)
	defer cancel()

	var full strings.Builder
	err = c.streamCompletion(sumCtx, endpoint, cfg.APIKey, chatCompletionRequest{
		Model: modelName,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt.String()},
		},
		Stream:      true,
		Temperature: 0.3,
	}, func(delta string) error {
		full.WriteString(delta)
		if onDelta != nil {
			return onDelta(delta)
		}
		return nil
	})
	if err != nil {
		return "", "", err
	}

	summary = strings.TrimSpace(full.String())
	if summary == "" {
		return "", "", errors.New("upstream returned empty summary")
	}
	return summary, modelName, nil
}

func truncateRunes(s string, maxRunes int) (string, bool) {
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s, false
	}
	count := 0
	for idx := range s {
		if count == maxRunes {
			return s[:idx], true
		}
		count++
	}
	return s, false
}

func (c *Client) streamCompletion(
	ctx context.Context,
	endpoint string,
	apiKey string,
	payload chatCompletionRequest,
	onDelta func(delta string) error,
) error {
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal completion request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("build completion request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("completion request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseUpstreamHTTPError(resp.StatusCode, resp.Body)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	sawDone := false
	sawStopReason := false

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			sawDone = true
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil && strings.TrimSpace(chunk.Error.Message) != "" {
			return fmt.Errorf("upstream error: %s", strings.TrimSpace(chunk.Error.Message))
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" && onDelta != nil {
				if err := onDelta(choice.Delta.Content); err != nil {
					return err
				}
			}
			if choice.FinishReason != nil {
				if fr := strings.TrimSpace(*choice.FinishReason); fr != "" {
					if !strings.EqualFold(fr, "stop") {
						if strings.EqualFold(fr, "length") {
							return errors.New("upstream response truncated (finish_reason: length)")
						}
						return fmt.Errorf("upstream completion stopped early (finish_reason: %s)", fr)
					}
					sawStopReason = true
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read completion stream: %w", err)
	}
	if !sawDone && !sawStopReason {
		return errors.New("upstream completion stream ended unexpectedly")
	}
	return nil
}

func parseUpstreamHTTPError(statusCode int, body io.Reader) error {
	raw, _ := io.ReadAll(io.LimitReader(body, 4096))
	var envelope upstreamErrorEnvelope
	if len(raw) > 0 && json.Unmarshal(raw, &envelope) == nil &&
		envelope.Error != nil && strings.TrimSpace(envelope.Error.Message) != "" {
		return fmt.Errorf("upstream HTTP %d: %s", statusCode, strings.TrimSpace(envelope.Error.Message))
	}
	snippet := strings.TrimSpace(string(raw))
	if snippet != "" && len(snippet) <= 200 {
		return fmt.Errorf("upstream HTTP %d: %s", statusCode, snippet)
	}
	return fmt.Errorf("upstream HTTP %d", statusCode)
}

type segmentStreamParser struct {
	expected  map[string]struct{}
	onSegment func(key, text string) error

	pendingLine strings.Builder
	activeKey   string
	activeBody  strings.Builder
	received    map[string]struct{}
}

func (p *segmentStreamParser) Feed(delta string) error {
	for _, r := range delta {
		if r == '\n' {
			if err := p.processLine(strings.TrimRight(p.pendingLine.String(), "\r")); err != nil {
				return err
			}
			p.pendingLine.Reset()
			continue
		}
		p.pendingLine.WriteRune(r)
	}
	return nil
}

func (p *segmentStreamParser) Finish() error {
	if p.pendingLine.Len() > 0 {
		if err := p.processLine(strings.TrimRight(p.pendingLine.String(), "\r")); err != nil {
			return err
		}
		p.pendingLine.Reset()
	}
	return p.flushActive()
}

func (p *segmentStreamParser) processLine(line string) error {
	trimmed := strings.TrimSpace(line)
	// Ignore optional markdown fence wrappers if a compatible model emits them.
	if strings.HasPrefix(trimmed, "```") {
		return nil
	}
	if matches := segmentMarkerRe.FindStringSubmatch(trimmed); len(matches) == 2 {
		key := matches[1]
		if _, ok := p.expected[key]; ok {
			if err := p.flushActive(); err != nil {
				return err
			}
			p.activeKey = key
			p.activeBody.Reset()
			return nil
		}
	}
	if p.activeKey == "" {
		return nil
	}
	if p.activeBody.Len() > 0 {
		p.activeBody.WriteByte('\n')
	}
	p.activeBody.WriteString(line)
	return nil
}

func (p *segmentStreamParser) flushActive() error {
	if p.activeKey == "" {
		return nil
	}
	text := strings.TrimSpace(p.activeBody.String())
	key := p.activeKey
	p.activeKey = ""
	p.activeBody.Reset()

	if text == "" {
		return nil
	}
	if p.received == nil {
		p.received = make(map[string]struct{}, len(p.expected))
	}
	p.received[key] = struct{}{}
	if p.onSegment != nil {
		return p.onSegment(key, text)
	}
	return nil
}
