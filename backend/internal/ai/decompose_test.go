package ai

import (
	"context"
	"testing"
	"time"
)

func TestValidateRequest_RejectsExtraFields(t *testing.T) {
	_, err := ValidateRequest([]byte(`{"title":"整理书桌","reward":"奶茶"}`))
	if err == nil {
		t.Fatal("expected reject")
	}
}

func TestValidateRequest_OK(t *testing.T) {
	req, err := ValidateRequest([]byte(`{"title":"整理书桌","notes":"先清桌面"}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Title != "整理书桌" {
		t.Fatalf("got %q", req.Title)
	}
}

func TestValidateStages(t *testing.T) {
	if err := ValidateStages([]StageDraft{{Title: "a"}, {Title: "b"}}); err == nil {
		t.Fatal("too few")
	}
	ok := []StageDraft{{Title: "先准备"}, {Title: "做一小段"}, {Title: "收个尾"}}
	if err := ValidateStages(ok); err != nil {
		t.Fatal(err)
	}
}

func TestLocalProvider_OK(t *testing.T) {
	p := LocalProvider{}
	stages, err := p.Decompose(context.Background(), DecomposeRequest{Title: "整理书桌", Notes: "先清桌面"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateStages(stages); err != nil {
		t.Fatal(err)
	}
}

func TestRunDecompose_AISuccess(t *testing.T) {
	resp := RunDecompose(context.Background(), LocalProvider{}, DecomposeRequest{Title: "寄出材料"})
	if resp.Source != SourceAI {
		t.Fatalf("source=%s", resp.Source)
	}
	if err := ValidateStages(resp.Stages); err != nil {
		t.Fatal(err)
	}
}

func TestRunDecompose_TimeoutFallsBack(t *testing.T) {
	start := time.Now()
	resp := RunDecompose(context.Background(), SlowProvider{Delay: 12 * time.Second}, DecomposeRequest{Title: "写完实验报告"})
	elapsed := time.Since(start)
	if resp.Source != SourceTemplate {
		t.Fatalf("source=%s", resp.Source)
	}
	if len(resp.Stages) != 3 || resp.Stages[0].Title != "先准备" {
		t.Fatalf("stages=%v", resp.Stages)
	}
	if elapsed > 9*time.Second {
		t.Fatalf("took too long: %v", elapsed)
	}
	if elapsed < 7*time.Second {
		t.Fatalf("returned too early: %v", elapsed)
	}
}

func TestRunDecompose_InvalidFallsBack(t *testing.T) {
	resp := RunDecompose(context.Background(), SlowProvider{
		Delay:  10 * time.Millisecond,
		Stages: []StageDraft{{Title: "只有一步"}},
	}, DecomposeRequest{Title: "测试"})
	if resp.Source != SourceTemplate {
		t.Fatalf("source=%s", resp.Source)
	}
}

func TestRunDecompose_ForbiddenFallsBack(t *testing.T) {
	resp := RunDecompose(context.Background(), SlowProvider{
		Delay: 10 * time.Millisecond,
		Stages: []StageDraft{
			{Title: "先准备"},
			{Title: "寻求治疗建议"},
			{Title: "收个尾"},
		},
	}, DecomposeRequest{Title: "测试"})
	if resp.Source != SourceTemplate {
		t.Fatalf("source=%s", resp.Source)
	}
}

func TestFallbackTemplate_Valid(t *testing.T) {
	if err := ValidateStages(FallbackTemplate()); err != nil {
		t.Fatal(err)
	}
}
