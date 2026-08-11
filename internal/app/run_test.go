package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunReadsSymptomFromArgument(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"argument result"}}]}`))
	}))
	defer server.Close()

	t.Setenv("OPSPILOT_API_KEY", "test-key")
	t.Setenv("OPSPILOT_BASE_URL", server.URL)
	t.Setenv("OPSPILOT_MODEL", "test-model")

	var stdout, stderr strings.Builder
	if err := Run(context.Background(), []string{"服务", "延迟"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if stdout.String() != "argument result\n" {
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
