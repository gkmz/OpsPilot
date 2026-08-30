package session

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gkmz/opspilot/internal/llm"
)

func TestNewRecordCopiesConversationData(t *testing.T) {
	createdAt := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	messages := []llm.Message{{Role: "user", Content: "问题"}}
	turnUsages := []TurnUsage{{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15, Known: true}}
	usage := UsageSummary{TurnCount: 1, PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15, Known: true}

	record := NewRecord("session-001", createdAt, messages, turnUsages, usage)
	messages[0].Content = "changed"
	turnUsages[0].TotalTokens = 999

	if record.ID != "session-001" || !record.CreatedAt.Equal(createdAt) || !record.UpdatedAt.Equal(createdAt) {
		t.Fatalf("unexpected record metadata: %+v", record)
	}
	if record.Messages[0].Content != "问题" || record.TurnUsages[0].TotalTokens != 15 {
		t.Fatalf("NewRecord() did not copy input data: %+v", record)
	}
}

func TestRecordJSONContainsOnlySessionData(t *testing.T) {
	record := Record{
		ID:       "session-001",
		Messages: []llm.Message{{Role: "user", Content: "问题"}},
	}

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	for _, forbidden := range []string{"api_key", "authorization", "base_url"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("record JSON contains forbidden field %q: %s", forbidden, data)
		}
	}
}
