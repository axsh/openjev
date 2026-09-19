package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"openjev/features/decision-test/internal/logger"
	"openjev/features/decision-test/internal/prompt"
)

type Client struct {
	baseURL string
	http    *http.Client
	sem     chan struct{}
	log     *logger.Logger
}

func NewClient(baseURL string, log *logger.Logger, parallel int) *Client {
	if parallel < 1 {
		parallel = 1
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Minute},
		sem:     make(chan struct{}, parallel),
		log:     log,
	}
}

func (c *Client) acquire(ctx context.Context) error {
	select {
	case c.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) release() {
	<-c.sem
}

type directBody struct {
	Messages           []prompt.Message   `json:"messages"`
	MaxTokens          int                `json:"max_tokens"`
	Temperature        float64            `json:"temperature"`
	TopK               int                `json:"top_k"`
	TopP               float64            `json:"top_p"`
	Logprobs           bool               `json:"logprobs"`
	TopLogprobs        int                `json:"top_logprobs"`
	LogitBias          map[string]float64 `json:"logit_bias"`
	Grammar            string             `json:"grammar"`
	CachePrompt        bool               `json:"cache_prompt"`
	ChatTemplateKwargs map[string]any     `json:"chat_template_kwargs"`
}

type genBody struct {
	Messages           []prompt.Message `json:"messages"`
	Stream             bool             `json:"stream"`
	MaxTokens          int              `json:"max_tokens"`
	Temperature        float64          `json:"temperature"`
	CachePrompt        bool             `json:"cache_prompt"`
	Grammar            string           `json:"grammar,omitempty"`
	ChatTemplateKwargs map[string]any   `json:"chat_template_kwargs"`
}

func (c *Client) ReadLabelLogprobs(ctx context.Context, messages []prompt.Message, labels []Label) (ReadResult, error) {
	if err := c.acquire(ctx); err != nil {
		return ReadResult{}, err
	}
	defer c.release()
	start := time.Now()
	bias := map[string]float64{}
	for _, label := range labels {
		bias[strconv.Itoa(label.TokenID)] = 100
	}
	body := directBody{
		Messages:           messages,
		MaxTokens:          1,
		Temperature:        1,
		TopK:               0,
		TopP:               1,
		Logprobs:           true,
		TopLogprobs:        topLogprobs(len(labels)),
		LogitBias:          bias,
		Grammar:            grammar(labels),
		CachePrompt:        false,
		ChatTemplateKwargs: map[string]any{"enable_thinking": false},
	}
	var parsed chatResponse
	if err := c.post(ctx, "/v1/chat/completions", body, &parsed); err != nil {
		return ReadResult{}, err
	}
	if len(parsed.Choices) == 0 || parsed.Choices[0].Logprobs == nil || len(parsed.Choices[0].Logprobs.Content) == 0 {
		return ReadResult{}, fmt.Errorf("llama response has no logprobs")
	}
	var top []TokenLogprob
	for _, item := range parsed.Choices[0].Logprobs.Content[0].TopLogprobs {
		if item.ID == nil {
			continue
		}
		top = append(top, TokenLogprob{ID: *item.ID, Logprob: item.Logprob})
	}
	return ReadResult{
		Top:              top,
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
		ElapsedMs:        float64(time.Since(start).Microseconds()) / 1000,
	}, nil
}

func (c *Client) Generate(ctx context.Context, messages []prompt.Message) (GenRaw, error) {
	if err := c.acquire(ctx); err != nil {
		return GenRaw{}, err
	}
	defer c.release()
	start := time.Now()
	body := genBody{
		Messages:           messages,
		Stream:             true,
		MaxTokens:          512,
		Temperature:        0,
		CachePrompt:        false,
		Grammar:            generationGrammar(messages),
		ChatTemplateKwargs: map[string]any{"enable_thinking": false},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return GenRaw{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return GenRaw{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return GenRaw{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logStatus(resp, "/v1/chat/completions")
		return GenRaw{}, fmt.Errorf("llama /v1/chat/completions status %d", resp.StatusCode)
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var text strings.Builder
	var ttft time.Time
	var promptTokens, completionTokens int
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 {
			content := chunk.Choices[0].Delta.Content
			if content != "" && ttft.IsZero() {
				ttft = time.Now()
			}
			text.WriteString(content)
		}
		if chunk.Usage != nil {
			promptTokens = chunk.Usage.PromptTokens
			completionTokens = chunk.Usage.CompletionTokens
		}
	}
	if err := scanner.Err(); err != nil {
		return GenRaw{}, err
	}
	var ttftMs float64
	if !ttft.IsZero() {
		ttftMs = float64(ttft.Sub(start).Microseconds()) / 1000
	}
	return GenRaw{
		Text:             text.String(),
		TTFTMs:           ttftMs,
		TotalMs:          float64(time.Since(start).Microseconds()) / 1000,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
	}, nil
}

func (c *Client) Health(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}

func (c *Client) Warmup(ctx context.Context) error {
	if err := c.acquire(ctx); err != nil {
		return err
	}
	defer c.release()
	body := genBody{
		Messages:           []prompt.Message{{Role: "user", Content: "Reply with the single word ready."}},
		MaxTokens:          1,
		Temperature:        0,
		CachePrompt:        false,
		ChatTemplateKwargs: map[string]any{"enable_thinking": false},
	}
	var parsed chatResponse
	if err := c.post(ctx, "/v1/chat/completions", body, &parsed); err != nil {
		return err
	}
	if len(parsed.Choices) == 0 {
		return fmt.Errorf("warmup returned no completion")
	}
	return nil
}

func ResolveLabels(ctx context.Context, c *Client) ([]Label, error) {
	labels := make([]Label, 0, 20)
	for i := 0; i < 20; i++ {
		letter := string(rune('A' + i))
		var parsed tokenizeResponse
		if err := c.post(ctx, "/tokenize", tokenizeRequest{Content: letter, WithPieces: true}, &parsed); err != nil {
			return nil, err
		}
		if len(parsed.Tokens) != 1 {
			return nil, fmt.Errorf("label %s is not a single token", letter)
		}
		labels = append(labels, Label{Letter: letter, TokenID: parsed.Tokens[0].ID})
	}
	return labels, nil
}

func (c *Client) post(ctx context.Context, path string, body any, dest any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logStatusBody(path, resp.StatusCode, respBody)
		return fmt.Errorf("llama %s status %d", path, resp.StatusCode)
	}
	if dest == nil {
		return nil
	}
	return json.Unmarshal(respBody, dest)
}

func (c *Client) logStatus(resp *http.Response, path string) {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
	c.logStatusBody(path, resp.StatusCode, body)
}

func (c *Client) logStatusBody(path string, status int, body []byte) {
	if c.log == nil {
		return
	}
	prefix := body
	if len(prefix) > 500 {
		prefix = prefix[:500]
	}
	c.log.Error("llama request failed", "path", path, "status", status, "body_prefix", string(prefix), "error", "non-success status")
}

func grammar(labels []Label) string {
	parts := make([]string, len(labels))
	for i, label := range labels {
		parts[i] = `"` + label.Letter + `"`
	}
	return "root ::= " + strings.Join(parts, " | ")
}

func generationGrammar(messages []prompt.Message) string {
	var keys []string
	for _, message := range messages {
		if message.Role != "user" {
			continue
		}
		for _, line := range strings.Split(message.Content, "\n") {
			if len(line) < 4 || line[1] != '.' || line[2] != ' ' {
				continue
			}
			if line[0] < 'A' || line[0] > 'T' {
				continue
			}
			keys = append(keys, line[:1]+": "+line[3:])
		}
	}
	if len(keys) == 0 {
		return ""
	}
	pairs := make([]string, len(keys))
	for i, key := range keys {
		pairs[i] = gbnfQuote(key) + ` ws ":" ws num`
	}
	return "ws ::= [ \t\n]*\n" +
		"num ::= \"0\" | \"1\" | \"0.\" [0-9]+ | \"1.\" \"0\"+\n" +
		"root ::= \"{\" ws " + strings.Join(pairs, " \",\" ws ") + " \"}\""
}

func gbnfQuote(key string) string {
	encoded, err := json.Marshal(key)
	if err != nil {
		encoded = []byte(`""`)
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range string(encoded) {
		if r == '\\' || r == '"' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

func topLogprobs(n int) int {
	if 4*n > 64 {
		return 4 * n
	}
	return 64
}

type chatResponse struct {
	Choices []struct {
		Logprobs *struct {
			Content []struct {
				TopLogprobs []struct {
					ID      *int    `json:"id"`
					Logprob float64 `json:"logprob"`
				} `json:"top_logprobs"`
			} `json:"content"`
		} `json:"logprobs"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type tokenizeRequest struct {
	Content    string `json:"content"`
	WithPieces bool   `json:"with_pieces"`
}

type tokenizeResponse struct {
	Tokens []struct {
		ID int
	}
}

func (r *tokenizeResponse) UnmarshalJSON(raw []byte) error {
	var body struct {
		Tokens []json.RawMessage `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	r.Tokens = nil
	for _, item := range body.Tokens {
		var id int
		if err := json.Unmarshal(item, &id); err == nil {
			r.Tokens = append(r.Tokens, struct{ ID int }{ID: id})
			continue
		}
		var obj struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(item, &obj); err != nil {
			return err
		}
		r.Tokens = append(r.Tokens, struct{ ID int }{ID: obj.ID})
	}
	return nil
}
