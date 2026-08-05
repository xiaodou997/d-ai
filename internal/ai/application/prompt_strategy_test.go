package application

import (
	"errors"
	"testing"
)

func TestResolveBoundExactPromptWithChineseNames(t *testing.T) {
	resolved, err := ResolvePrompt(PromptStrategyBoundExact, PromptResolveInput{
		Input: "客户想了解交付周期，请结合 {{ 客户背景 }}，遵循 {{售前规范}}。",
		Bindings: []RuntimePromptBinding{
			{PromptID: "p1", PromptName: "客户背景", PromptRevision: 3, TemplateText: "客户是 {{客户名称}}", Role: PromptBindingSystem},
			{PromptID: "p2", PromptName: "售前规范", PromptRevision: 2, TemplateText: "回答需要准确", Role: PromptBindingSystem},
		},
		Variables: map[string]string{"客户名称": "小豆科技"},
	})
	if err != nil {
		t.Fatalf("ResolvePrompt: %v", err)
	}
	want := "客户想了解交付周期，请结合 客户是 小豆科技，遵循 回答需要准确。"
	if resolved.Input != want {
		t.Fatalf("input = %q, want %q", resolved.Input, want)
	}
	if len(resolved.Snapshots) != 2 {
		t.Fatalf("snapshots = %#v", resolved.Snapshots)
	}
}

func TestResolveBoundExactPromptRejectsMissingAndFullWidthBraces(t *testing.T) {
	_, err := ResolvePrompt(PromptStrategyBoundExact, PromptResolveInput{Input: "{{未知提示词}}"})
	if !errors.Is(err, ErrPromptPlaceholderMissing) {
		t.Fatalf("missing error = %v", err)
	}
	_, err = ResolvePrompt(PromptStrategyBoundExact, PromptResolveInput{Input: "｛｛客户背景｝｝"})
	if !errors.Is(err, ErrPromptPlaceholderInvalid) {
		t.Fatalf("full-width braces error = %v", err)
	}
}

func TestResolveBoundExactPromptOnlyRendersSelectedBindings(t *testing.T) {
	resolved, err := ResolvePrompt(PromptStrategyBoundExact, PromptResolveInput{
		Input: "请结合 {{客户背景}} 回答。",
		Bindings: []RuntimePromptBinding{
			{PromptID: "p1", PromptName: "客户背景", PromptRevision: 2, TemplateText: "客户是小豆科技"},
			{PromptID: "p2", PromptName: "未使用资料", PromptRevision: 1, TemplateText: "{{未传变量}}"},
		},
	})
	if err != nil {
		t.Fatalf("ResolvePrompt: %v", err)
	}
	if resolved.Input != "请结合 客户是小豆科技 回答。" {
		t.Fatalf("input = %q", resolved.Input)
	}
	if len(resolved.Snapshots) != 1 || resolved.Snapshots[0].PromptID != "p1" {
		t.Fatalf("snapshots = %#v", resolved.Snapshots)
	}
}

func TestResolveCallerVariablesUsesUnicodeKeys(t *testing.T) {
	resolved, err := ResolvePrompt(PromptStrategyCallerVariables, PromptResolveInput{
		Input:     "介绍交付周期",
		Variables: map[string]string{"客户名称": "小豆科技"},
		Bindings: []RuntimePromptBinding{{
			PromptID: "p1", PromptName: "售前助手", PromptRevision: 4,
			TemplateText: "你是 {{客户名称}} 的售前助手。", Role: PromptBindingSystem,
		}},
	})
	if err != nil {
		t.Fatalf("ResolvePrompt: %v", err)
	}
	if resolved.Instruction != "你是 小豆科技 的售前助手。" {
		t.Fatalf("instruction = %q", resolved.Instruction)
	}
}

func TestResolvePromptRejectsAmbiguousNormalizedVariableNames(t *testing.T) {
	_, err := ResolvePrompt(PromptStrategyCallerVariables, PromptResolveInput{
		Input: "介绍客户",
		Variables: map[string]string{
			"Cafe\u0301": "first",
			"Café":       "second",
		},
		Bindings: []RuntimePromptBinding{{
			PromptID: "p1", PromptName: "客户背景", PromptRevision: 1,
			TemplateText: "客户是 {{Café}}", Role: PromptBindingSystem,
		}},
	})
	if !errors.Is(err, ErrPromptPlaceholderInvalid) {
		t.Fatalf("error = %v, want invalid placeholder", err)
	}
}
