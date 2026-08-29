package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gkmz/opspilot/internal/llm"
)

func TestRunReadsSymptomFromArgument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"argument result\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()

	t.Setenv("OPSPILOT_API_KEY", "test-key")
	t.Setenv("OPSPILOT_BASE_URL", server.URL)
	t.Setenv("OPSPILOT_MODEL", "test-model")

	var stdout strings.Builder
	if err := Run(context.Background(), []string{"服务", "延迟"}, strings.NewReader(""), &stdout, io.Discard); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "> 服务 延迟\nargument result") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
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

	client := llm.NewClient(server.URL+"/v1", "test-key", "test-model", time.Second)
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

	client := llm.NewClient(server.URL, "test-key", "test-model", time.Second)
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
