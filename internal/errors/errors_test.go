package errors

import (
	"context"
	stderrors "errors"
	"net/http"
	"testing"
)

func TestErrorPreservesCauseAndFormatsCategory(t *testing.T) {
	cause := context.Canceled
	err := Wrap(KindCanceled, "流式请求已取消", cause)

	if !stderrors.Is(err, cause) {
		t.Fatal("classified error did not preserve its cause")
	}
	if got, want := err.Error(), "取消错误：流式请求已取消"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestKindOfReturnsUnknownForUnclassifiedError(t *testing.T) {
	if got := KindOf(stderrors.New("plain error")); got != KindUnknown {
		t.Fatalf("KindOf() = %q, want %q", got, KindUnknown)
	}
}

func TestHTTPErrorPreservesStatusCode(t *testing.T) {
	err := NewHTTPError(http.StatusTooManyRequests, stderrors.New("rate limited"))

	var classified *Error
	if !stderrors.As(err, &classified) {
		t.Fatal("HTTP error was not available through errors.As")
	}
	if classified.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status code = %d, want %d", classified.StatusCode, http.StatusTooManyRequests)
	}
}

func TestErrorKindLabels(t *testing.T) {
	for _, test := range []struct {
		kind Kind
		want string
	}{
		{KindConfig, "配置错误"},
		{KindNetwork, "网络错误"},
		{KindHTTP, "HTTP 错误"},
		{KindProtocol, "协议错误"},
		{KindCanceled, "取消错误"},
		{KindCallback, "输出错误"},
		{KindStorage, "存储错误"},
	} {
		t.Run(string(test.kind), func(t *testing.T) {
			if got := Wrap(test.kind, "detail", nil).Error(); got != test.want+"：detail" {
				t.Fatalf("Error() = %q, want %q", got, test.want+"：detail")
			}
		})
	}
}
