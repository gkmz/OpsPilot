package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gkmz/opspilot/internal/config"
	"github.com/gkmz/opspilot/internal/llm"
	"github.com/gkmz/opspilot/internal/session"
)

func TestRunReadsSymptomFromArgument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"argument result\"}}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()

	t.Setenv("OPSPILOT_API_KEY", "test-key")
	t.Setenv("OPSPILOT_BASE_URL", server.URL)
	t.Setenv("OPSPILOT_MODEL", "test-model")
	sessionDirectory := t.TempDir()
	t.Setenv("OPSPILOT_SESSION_DIR", sessionDirectory)

	var stdout strings.Builder
	if err := Run(context.Background(), []string{"服务", "延迟"}, strings.NewReader(""), &stdout, io.Discard); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "> 服务 延迟\nargument result") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	entries, err := os.ReadDir(sessionDirectory)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].IsDir() {
		t.Fatalf("saved session entries = %+v, want one file", entries)
	}
	store := session.NewStore(sessionDirectory)
	record, err := store.Load(strings.TrimSuffix(entries[0].Name(), ".json"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(record.Messages) != 3 || record.Messages[1].Content != "服务 延迟" || record.Messages[2].Content != "argument result" {
		t.Fatalf("saved messages = %+v", record.Messages)
	}
	if len(record.TurnUsages) != 1 || record.TurnUsages[0].TotalTokens != 15 || record.Usage.TotalTokens != 15 {
		t.Fatalf("saved usage = %+v, summary = %+v", record.TurnUsages, record.Usage)
	}
}

func TestRunRequiresSymptom(t *testing.T) {
	err := func() error {
		return Run(context.Background(), nil, strings.NewReader("  "), io.Discard, io.Discard)
	}()
	if err == nil {
		t.Fatal("Run() expected missing symptom error")
	}
}

func TestWriteUsageDisplaysKnownTokens(t *testing.T) {
	var output strings.Builder
	writeUsage(&output, llm.Usage{
		PromptTokens:     10,
		CompletionTokens: 5,
		TotalTokens:      15,
		Known:            true,
	})

	if got, want := output.String(), "\nToken 使用：输入 10，输出 5，总计 15"; got != want {
		t.Fatalf("writeUsage() = %q, want %q", got, want)
	}
}

func TestWriteUsageDisplaysUnknownTokens(t *testing.T) {
	var output strings.Builder
	writeUsage(&output, llm.Usage{})

	if got, want := output.String(), "\nToken 使用：未知"; got != want {
		t.Fatalf("writeUsage() = %q, want %q", got, want)
	}
}

func TestRunInteractiveSendsFollowUpWithHistory(t *testing.T) {
	var requests [][]llm.Message
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []llm.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, request.Messages)

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()

	client := llm.NewClient(config.Config{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1",
		Model:   "test-model",
		Timeout: time.Second,
	})
	var output strings.Builder
	err := RunInteractive(context.Background(), client, []string{"第一轮"}, strings.NewReader("第二轮\n/exit\n"), &output)
	if err != nil {
		t.Fatalf("RunInteractive() error = %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
	if len(requests[1]) != 4 || requests[1][1].Content != "第一轮" || requests[1][2].Role != "assistant" || requests[1][3].Content != "第二轮" {
		t.Fatalf("unexpected follow-up history: %+v", requests[1])
	}
}

func TestRunInteractiveDoesNotCommitPartialAssistant(t *testing.T) {
	var requests [][]llm.Message
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []llm.Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, request.Messages)

		w.Header().Set("Content-Type", "text/event-stream")
		if len(requests) == 1 {
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"))
			return
		}
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"recovered\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()

	client := llm.NewClient(config.Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test-model",
		Timeout: time.Second,
	})
	var output strings.Builder
	err := RunInteractive(context.Background(), client, []string{"第一轮"}, strings.NewReader("第二轮\n/exit\n"), &output)
	if err != nil {
		t.Fatalf("RunInteractive() error = %v", err)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d, want 2", len(requests))
	}
	if len(requests[1]) != 3 || requests[1][1].Content != "第一轮" || requests[1][2].Content != "第二轮" {
		t.Fatalf("partial assistant was committed: %+v", requests[1])
	}
}
