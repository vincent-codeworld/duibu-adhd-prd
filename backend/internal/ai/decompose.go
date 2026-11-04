package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	SourceAI       = "ai"
	SourceTemplate = "template"
	Budget         = 8 * time.Second
)

type DecomposeRequest struct {
	Title string `json:"title"`
	Notes string `json:"notes,omitempty"`
}

type StageDraft struct {
	Title string `json:"title"`
}

type DecomposeResponse struct {
	Stages []StageDraft `json:"stages"`
	Source string       `json:"source"` // ai | template
}

// Provider drafts stages from title+notes only. Must respect ctx cancellation.
type Provider interface {
	Decompose(ctx context.Context, req DecomposeRequest) ([]StageDraft, error)
}

// FallbackTemplate is the PRD §11.5 three-step editable template.
func FallbackTemplate() []StageDraft {
	return []StageDraft{
		{Title: "先准备"},
		{Title: "做一小段"},
		{Title: "收个尾"},
	}
}

// ValidateRequest enforces PRD: only title+notes; title 1–40; notes ≤500.
func ValidateRequest(raw []byte) (DecomposeRequest, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return DecomposeRequest{}, fmt.Errorf("invalid json")
	}
	for k := range probe {
		if k != "title" && k != "notes" {
			return DecomposeRequest{}, fmt.Errorf("field %q not allowed", k)
		}
	}
	var req DecomposeRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return DecomposeRequest{}, fmt.Errorf("invalid json")
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Notes = strings.TrimSpace(req.Notes)
	n := utf8.RuneCountInString(req.Title)
	if n < 1 || n > 40 {
		return DecomposeRequest{}, fmt.Errorf("title must be 1–40 characters")
	}
	if utf8.RuneCountInString(req.Notes) > 500 {
		return DecomposeRequest{}, fmt.Errorf("notes must be ≤500 characters")
	}
	return req, nil
}

// ValidateStages checks 3–7 non-empty titles ≤40 runes after trim.
func ValidateStages(stages []StageDraft) error {
	if len(stages) < 3 || len(stages) > 7 {
		return fmt.Errorf("need 3–7 stages")
	}
	for i, s := range stages {
		t := strings.TrimSpace(s.Title)
		if t == "" || utf8.RuneCountInString(t) > 40 {
			return fmt.Errorf("stage %d title invalid", i+1)
		}
	}
	return nil
}

func NormalizeStages(stages []StageDraft) []StageDraft {
	out := make([]StageDraft, 0, len(stages))
	for _, s := range stages {
		out = append(out, StageDraft{Title: strings.TrimSpace(s.Title)})
	}
	return out
}

// ForbiddenContent reports medical / reward / diagnosis wording that must degrade.
func ForbiddenContent(stages []StageDraft) bool {
	banned := []string{
		"治疗", "治愈", "诊断", "处方", "用药", "药物", "ADHD 治疗",
		"奖品", "积分", "商城", "支付",
	}
	for _, s := range stages {
		t := s.Title
		for _, b := range banned {
			if strings.Contains(t, b) {
				return true
			}
		}
	}
	return false
}

// RunDecompose calls provider within Budget; on timeout/error/invalid/forbidden returns template.
func RunDecompose(parent context.Context, p Provider, req DecomposeRequest) DecomposeResponse {
	ctx, cancel := context.WithTimeout(parent, Budget)
	defer cancel()

	type outcome struct {
		stages []StageDraft
		err    error
	}
	ch := make(chan outcome, 1)
	go func() {
		stages, err := p.Decompose(ctx, req)
		ch <- outcome{stages, err}
	}()

	select {
	case <-ctx.Done():
		return DecomposeResponse{Stages: FallbackTemplate(), Source: SourceTemplate}
	case o := <-ch:
		if o.err != nil {
			return DecomposeResponse{Stages: FallbackTemplate(), Source: SourceTemplate}
		}
		stages := NormalizeStages(o.stages)
		if err := ValidateStages(stages); err != nil || ForbiddenContent(stages) {
			return DecomposeResponse{Stages: FallbackTemplate(), Source: SourceTemplate}
		}
		return DecomposeResponse{Stages: stages, Source: SourceAI}
	}
}
