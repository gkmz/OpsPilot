package conversation

import (
	"testing"

	"github.com/gkmz/opspilot/internal/llm"
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
