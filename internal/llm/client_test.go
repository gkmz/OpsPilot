package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected authorization: %q", got)
		}
		var request chatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Model != "test-model" || len(request.Messages) != 1 {
			t.Fatalf("unexpected request payload: %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"初步分析结果"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL+"/v1", "test-key", "test-model", time.Second)
	response, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "接口延迟升高"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if response.Content != "初步分析结果" {
		t.Fatalf("unexpected content: %q", response.Content)
	}
	if !response.Usage.Known || response.Usage.TotalTokens != 15 {
		t.Fatalf("unexpected usage: %+v", response.Usage)
	}
}

func TestClientChatReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"rate limited"}`, http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key", "test-model", time.Second)
	if _, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "test"}}); err == nil {
		t.Fatal("Chat() expected HTTP error")
	}
}
