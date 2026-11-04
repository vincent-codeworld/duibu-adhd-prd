package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// LocalProvider returns a deterministic 3–5 step draft without calling an LLM.
// Used when AI_API_KEY is empty (local stub / 联调).
type LocalProvider struct{}

func (LocalProvider) Decompose(ctx context.Context, req DecomposeRequest) ([]StageDraft, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	title := truncateRunes(req.Title, 18)
	stages := []StageDraft{
		{Title: "先准备好材料和时间"},
		{Title: truncateRunes("动手做：「"+title+"」的一小步", 40)},
		{Title: "检查一遍有没有漏掉的"},
	}
	if req.Notes != "" {
		stages = append(stages, StageDraft{Title: "按备注再收口一次"})
	}
	stages = append(stages, StageDraft{Title: "收个尾"})
	if len(stages) > 7 {
		stages = stages[:7]
	}
	return stages, nil
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max])
}

// HTTPProvider calls an OpenAI-compatible chat completions endpoint.
type HTTPProvider struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

func NewHTTPProvider(baseURL, apiKey, model string) *HTTPProvider {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &HTTPProvider{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Model:   model,
		HTTPClient: &http.Client{
			Timeout: Budget,
		},
	}
}

func (p *HTTPProvider) Decompose(ctx context.Context, req DecomposeRequest) ([]StageDraft, error) {
	system := `你是兑步任务拆解助手。只根据用户给出的任务标题和备注，起草 3 到 7 个短步骤。
规则：
- 每步是可执行的小动作，标题去掉首尾空格后 1–40 字。
- 不要写奖品、积分、支付、商城。
- 不要给出医疗、诊断、治疗、用药建议。
- 只输出 JSON：{"stages":[{"title":"..."},...]}，不要其它文字。`

	user := "标题：" + req.Title
	if req.Notes != "" {
		user += "\n备注：" + req.Notes
	}

	body := map[string]any{
		"model": p.Model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": 0.4,
		"response_format": map[string]string{
			"type": "json_object",
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)

	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: Budget}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("provider status %d", resp.StatusCode)
	}

	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Choices) == 0 {
		return nil, fmt.Errorf("empty provider choices")
	}
	content := strings.TrimSpace(envelope.Choices[0].Message.Content)
	var parsed struct {
		Stages []StageDraft `json:"stages"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, fmt.Errorf("provider json: %w", err)
	}
	return parsed.Stages, nil
}

// NewProvider picks HTTP when API key is set, otherwise LocalProvider.
func NewProvider(baseURL, apiKey, model string) Provider {
	if strings.TrimSpace(apiKey) == "" {
		return LocalProvider{}
	}
	return NewHTTPProvider(baseURL, apiKey, model)
}

// SlowProvider is for tests: sleeps then returns stages or errs if cancelled.
type SlowProvider struct {
	Delay  time.Duration
	Stages []StageDraft
	Err    error
}

func (p SlowProvider) Decompose(ctx context.Context, _ DecomposeRequest) ([]StageDraft, error) {
	timer := time.NewTimer(p.Delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		if p.Err != nil {
			return nil, p.Err
		}
		if p.Stages != nil {
			return p.Stages, nil
		}
		return FallbackTemplate(), nil
	}
}
