package diagnosis

import "testing"

func TestBuildMessages(t *testing.T) {
	messages := BuildMessages("接口延迟升高")
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Role != "system" || messages[1].Role != "user" {
		t.Fatalf("unexpected roles: %+v", messages)
	}
	if messages[1].Content != "接口延迟升高" {
		t.Fatalf("unexpected user content: %q", messages[1].Content)
	}
}
