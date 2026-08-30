package session

import (
	"testing"

	"github.com/gkmz/opspilot/internal/llm"
)

func TestNewTurnUsageCopiesLLMUsage(t *testing.T) {
	got := NewTurnUsage(llm.Usage{
		PromptTokens:     10,
		CompletionTokens: 5,
		TotalTokens:      15,
		Known:            true,
	})

	want := TurnUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15, Known: true}
	if got != want {
		t.Fatalf("NewTurnUsage() = %+v, want %+v", got, want)
	}
}

func TestUsageSummaryAddsTurnUsages(t *testing.T) {
	var summary UsageSummary
	summary.Add(TurnUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15, Known: true})
	summary.Add(TurnUsage{PromptTokens: 20, CompletionTokens: 8, TotalTokens: 28, Known: true})

	want := UsageSummary{TurnCount: 2, PromptTokens: 30, CompletionTokens: 13, TotalTokens: 43, Known: true}
	if summary != want {
		t.Fatalf("UsageSummary = %+v, want %+v", summary, want)
	}
}

func TestUsageSummaryBecomesUnknownWhenAnyTurnIsUnknown(t *testing.T) {
	var summary UsageSummary
	summary.Add(TurnUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15, Known: true})
	summary.Add(TurnUsage{})

	if summary.Known {
		t.Fatalf("UsageSummary = %+v, want unknown", summary)
	}
}
