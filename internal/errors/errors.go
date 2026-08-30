// Package errors 定义 OpsPilot 对外稳定的错误类别。
package errors

import (
	stderrors "errors"
	"fmt"
)

// Kind 表示错误所属的业务类别。
type Kind string

const (
	KindUnknown  Kind = "unknown"
	KindConfig   Kind = "config"
	KindNetwork  Kind = "network"
	KindHTTP     Kind = "http"
	KindProtocol Kind = "protocol"
	KindCanceled Kind = "canceled"
	KindCallback Kind = "callback"
	KindStorage  Kind = "storage"
)

// Error 表示带有稳定类别、用户提示和底层原因的项目错误。
type Error struct {
	Kind       Kind
	Message    string
	Cause      error
	StatusCode int
}

// Error 返回不泄露底层请求细节的用户可读错误信息。
func (e *Error) Error() string {
	return fmt.Sprintf("%s：%s", label(e.Kind), e.Message)
}

// Unwrap 返回底层原因，供 errors.Is 和 errors.As 使用。
func (e *Error) Unwrap() error {
	return e.Cause
}

// Wrap 将已有错误包装为指定类别，并保留底层原因。
func Wrap(kind Kind, message string, cause error) error {
	return &Error{Kind: kind, Message: message, Cause: cause}
}

// NewHTTPError 创建带 HTTP 状态码的 HTTP 类错误。
func NewHTTPError(statusCode int, cause error) error {
	return &Error{
		Kind:       KindHTTP,
		Message:    fmt.Sprintf("模型返回 HTTP %d", statusCode),
		Cause:      cause,
		StatusCode: statusCode,
	}
}

// KindOf 返回错误链中项目错误的类别；未分类时返回 KindUnknown。
func KindOf(err error) Kind {
	var classified *Error
	if stderrors.As(err, &classified) {
		return classified.Kind
	}
	return KindUnknown
}

func label(kind Kind) string {
	switch kind {
	case KindConfig:
		return "配置错误"
	case KindNetwork:
		return "网络错误"
	case KindHTTP:
		return "HTTP 错误"
	case KindProtocol:
		return "协议错误"
	case KindCanceled:
		return "取消错误"
	case KindCallback:
		return "输出错误"
	case KindStorage:
		return "存储错误"
	default:
		return "未知错误"
	}
}
