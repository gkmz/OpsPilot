package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gkmz/opspilot/internal/config"
	"github.com/openai/openai-go/v3"
)

type chatRequestPayload struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

func newTestClient(baseURL string, timeout time.Duration) Client {
	return NewClient(config.Config{
		APIKey:  "test-key",
		BaseURL: baseURL,
		Model:   "test-model",
		Timeout: timeout,
	})
}

func TestClientChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected authorization: %q", got)
		}
		var request chatRequestPayload
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

	client := newTestClient(server.URL+"/v1", time.Second)
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
		writeOpenAIError(w, http.StatusTooManyRequests, "rate limited")
	}))
	defer server.Close()

	client := newTestClient(server.URL, time.Second)
	_, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "test"}})
	if err == nil {
		t.Fatal("Chat() expected HTTP error")
	}

	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("Chat() error = %T %v, want *openai.Error in error chain", err, err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("Chat() status = %d, want %d", apiErr.StatusCode, http.StatusTooManyRequests)
	}
}

func TestClientChatMarksMissingUsageUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()

	client := newTestClient(server.URL, time.Second)
	response, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "test"}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if response.Usage.Known {
		t.Fatalf("Chat() usage = %+v, want unknown usage", response.Usage)
	}
}

func TestClientChatRejectsEmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer server.Close()

	client := newTestClient(server.URL, time.Second)
	_, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "test"}})
	if err == nil || !strings.Contains(err.Error(), "模型响应缺少 choices") {
		t.Fatalf("Chat() error = %v, want missing choices error", err)
	}
}

func TestClientStreamReceivesChunksAndSendsStreamFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request chatRequestPayload
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if !request.Stream {
			t.Fatal("Stream request did not set stream=true")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("unexpected Content-Type: %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("unexpected Accept: %q", got)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"，世界\"}}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	client := newTestClient(server.URL, time.Second)
	var chunks []string
	usage, err := client.Stream(context.Background(), []Message{{Role: "user", Content: "test"}}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if got := strings.Join(chunks, ""); got != "你好，世界" {
		t.Fatalf("stream content = %q, want %q", got, "你好，世界")
	}
	if !usage.Known || usage.TotalTokens != 15 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

func TestClientStreamRejectsUnexpectedEOF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
	}))
	defer server.Close()

	client := newTestClient(server.URL, time.Second)
	_, err := client.Stream(context.Background(), nil, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "未收到 [DONE]") {
		t.Fatalf("Stream() error = %v, want unexpected EOF error", err)
	}
}

func TestClientStreamReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeOpenAIError(w, http.StatusTooManyRequests, "rate limited")
	}))
	defer server.Close()

	client := newTestClient(server.URL, time.Second)
	_, err := client.Stream(context.Background(), nil, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "模型返回 HTTP 429") {
		t.Fatalf("Stream() error = %v, want HTTP 429 error", err)
	}
}

func writeOpenAIError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = fmt.Fprintf(
		w,
		`{"error":{"message":%q,"type":"rate_limit_error","param":null,"code":"rate_limit"}}`,
		message,
	)
}

func TestClientStreamRejectsInvalidEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: not-json\n\n")
	}))
	defer server.Close()

	client := newTestClient(server.URL, time.Second)
	_, err := client.Stream(context.Background(), nil, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "解析流式响应失败") {
		t.Fatalf("Stream() error = %v, want invalid event error", err)
	}
}

func TestClientStreamReturnsCallbackError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"chunk\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	callbackErr := errors.New("output closed")
	client := newTestClient(server.URL, time.Second)
	_, err := client.Stream(context.Background(), nil, func(string) error { return callbackErr })
	if !errors.Is(err, callbackErr) {
		t.Fatalf("Stream() error = %v, want callback error", err)
	}
}

func TestClientStreamCanBeCanceled(t *testing.T) {
	firstChunk := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("test server does not support flushing")
		}
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"chunk\"}}]}\n\n")
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := newTestClient(server.URL, 5*time.Second)
	result := make(chan error, 1)
	go func() {
		_, err := client.Stream(ctx, nil, func(string) error {
			close(firstChunk)
			return nil
		})
		result <- err
	}()

	select {
	case <-firstChunk:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first stream chunk")
	}

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Stream() error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Stream() did not return after cancellation")
	}
}
