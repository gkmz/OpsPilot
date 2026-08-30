package conversation

import (
	"testing"

	"github.com/gkmz/opspilot/internal/llm"
	"github.com/gkmz/opspilot/internal/session"
)

func TestConversationKeepsCommittedMessages(t *testing.T) {
	chat := New(llm.Message{Role: "system", Content: "system prompt"})
	chat.CommitUser("第一轮")
	chat.CommitAssistant("第一轮回答")

	messages := chat.Messages()
	if len(messages) != 3 {
		t.Fatalf("message count = %d, want 3", len(messages))
	}
	if messages[0].Role != "system" || messages[1].Role != "user" || messages[2].Role != "assistant" {
		t.Fatalf("unexpected messages: %+v", messages)
	}
}

func TestConversationMessagesReturnsCopy(t *testing.T) {
	chat := New(llm.Message{Role: "system", Content: "system prompt"})
	messages := chat.Messages()
	messages[0].Content = "changed"

	if chat.Messages()[0].Content != "system prompt" {
		t.Fatal("Messages() exposed internal conversation state")
	}
}

func TestConversationIgnoresEmptyMessages(t *testing.T) {
	chat := New(llm.Message{Role: "system", Content: "system prompt"})
	chat.CommitUser("  ")
	chat.CommitAssistant("  ")

	if got := len(chat.Messages()); got != 1 {
		t.Fatalf("message count = %d, want 1", got)
	}
}

func TestConversationAccumulatesKnownUsage(t *testing.T) {
	chat := New(llm.Message{Role: "system", Content: "system prompt"})
	chat.CommitUsage(session.TurnUsage{
		PromptTokens:     10,
		CompletionTokens: 5,
		TotalTokens:      15,
		Known:            true,
	})
	chat.CommitUsage(session.TurnUsage{
		PromptTokens:     20,
		CompletionTokens: 8,
		TotalTokens:      28,
		Known:            true,
	})

	usage := chat.Usage()
	if usage.PromptTokens != 30 || usage.CompletionTokens != 13 || usage.TotalTokens != 43 || !usage.Known {
		t.Fatalf("unexpected accumulated usage: %+v", usage)
	}
}

func TestConversationMarksAccumulatedUsageUnknown(t *testing.T) {
	chat := New(llm.Message{Role: "system", Content: "system prompt"})
	chat.CommitUsage(session.TurnUsage{
		PromptTokens:     10,
		CompletionTokens: 5,
		TotalTokens:      15,
		Known:            true,
	})
	chat.CommitUsage(session.TurnUsage{})

	usage := chat.Usage()
	if usage.Known {
		t.Fatalf("usage = %+v, want unknown after an unknown turn", usage)
	}
}

func TestConversationReturnsUsageCopy(t *testing.T) {
	chat := New(llm.Message{Role: "system", Content: "system prompt"})
	chat.CommitUsage(session.TurnUsage{TotalTokens: 15, Known: true})

	usage := chat.Usage()
	usage.TotalTokens = 999

	if got := chat.Usage().TotalTokens; got != 15 {
		t.Fatalf("Usage() exposed internal state: got %d, want 15", got)
	}
}

func TestConversationReturnsTurnUsageSnapshot(t *testing.T) {
	chat := New(llm.Message{Role: "system", Content: "system prompt"})
	chat.CommitUsage(session.TurnUsage{TotalTokens: 15, Known: true})
	chat.CommitUsage(session.TurnUsage{TotalTokens: 28, Known: true})

	usages := chat.TurnUsages()
	if len(usages) != 2 || usages[0].TotalTokens != 15 || usages[1].TotalTokens != 28 {
		t.Fatalf("unexpected turn usages: %+v", usages)
	}

	usages[0].TotalTokens = 999
	if got := chat.TurnUsages()[0].TotalTokens; got != 15 {
		t.Fatalf("TurnUsages() exposed internal state: got %d, want 15", got)
	}
}

func TestConversationReturnsSnapshot(t *testing.T) {
	chat := New(llm.Message{Role: "system", Content: "system prompt"})
	chat.CommitUser("第一轮")
	chat.CommitAssistant("第一轮回答")
	chat.CommitUsage(session.TurnUsage{TotalTokens: 15, Known: true})

	snapshot := chat.Snapshot()
	if len(snapshot.Messages) != 3 {
		t.Fatalf("snapshot message count = %d, want 3", len(snapshot.Messages))
	}
	if len(snapshot.TurnUsages) != 1 || snapshot.TurnUsages[0].TotalTokens != 15 {
		t.Fatalf("snapshot turn usages = %+v", snapshot.TurnUsages)
	}
	if snapshot.Usage.TotalTokens != 15 || !snapshot.Usage.Known {
		t.Fatalf("snapshot usage = %+v", snapshot.Usage)
	}
}

func TestConversationSnapshotDoesNotExposeInternalState(t *testing.T) {
	chat := New(llm.Message{Role: "system", Content: "system prompt"})
	chat.CommitUsage(session.TurnUsage{TotalTokens: 15, Known: true})

	snapshot := chat.Snapshot()
	snapshot.Messages[0].Content = "changed"
	snapshot.TurnUsages[0].TotalTokens = 999

	current := chat.Snapshot()
	if current.Messages[0].Content != "system prompt" || current.TurnUsages[0].TotalTokens != 15 {
		t.Fatalf("snapshot exposed internal state: %+v", current)
	}
}
