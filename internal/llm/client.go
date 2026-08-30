// Package llm 提供与具体模型 SDK 解耦的通用对话客户端。
package llm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/gkmz/opspilot/internal/config"
	opserrors "github.com/gkmz/opspilot/internal/errors"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// Client 定义 OpsPilot 使用的通用对话客户端能力。
type Client interface {
	Chat(context.Context, []Message) (Response, error)
	Stream(context.Context, []Message, func(string) error) (Usage, error)
}

// Message 表示一次聊天请求中的一条消息。
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Usage 表示模型端点返回的 Token 使用量。
//
// 某些兼容端点可能不返回 usage，此时 Known 为 false，调用方不能把零值当成真实统计。
type Usage struct {
	PromptTokens     int  `json:"prompt_tokens"`
	CompletionTokens int  `json:"completion_tokens"`
	TotalTokens      int  `json:"total_tokens"`
	Known            bool `json:"-"`
}

// Response 表示一次非流式聊天调用的结果。
type Response struct {
	Content string
	Usage   Usage
}

// ChatCompletionClient 使用 OpenAI 官方 Go SDK 实现通用对话客户端。
type ChatCompletionClient struct {
	client openai.Client
	model  string
}

// NewClient 根据应用配置创建 OpenAI 官方 SDK 客户端。
func NewClient(cfg config.Config) Client {
	httpClient := &http.Client{Timeout: cfg.Timeout}
	sdkClient := openai.NewClient(
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(strings.TrimRight(cfg.BaseURL, "/")+"/"),
		option.WithHTTPClient(httpClient),
		option.WithMaxRetries(0),
	)

	return &ChatCompletionClient{
		client: sdkClient,
		model:  cfg.Model,
	}
}

// Chat 发起一次非流式聊天请求并返回模型文本和 usage。
//
// 取消由 ctx 控制；SDK 和模型端点错误会保留在错误链中，不在当前阶段自动重试。
func (c *ChatCompletionClient) Chat(ctx context.Context, messages []Message) (Response, error) {
	params, err := c.newChatParams(messages)
	if err != nil {
		return Response{}, opserrors.Wrap(opserrors.KindProtocol, "构造模型请求失败", err)
	}

	completion, err := c.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return Response{}, wrapSDKError("请求模型失败", err)
	}
	if len(completion.Choices) == 0 {
		return Response{}, opserrors.Wrap(opserrors.KindProtocol, "模型响应缺少 choices", nil)
	}

	return Response{
		Content: completion.Choices[0].Message.Content,
		Usage:   usageFromCompletion(completion.Usage, completion.JSON.Usage.Valid()),
	}, nil
}

// Stream 发起流式聊天请求，并通过 onChunk 按增量返回模型文本。
//
// 只有官方 SDK 完整消费到服务端的 [DONE] 标记后，当前请求才会被视为成功。
func (c *ChatCompletionClient) Stream(ctx context.Context, messages []Message, onChunk func(string) error) (Usage, error) {
	if onChunk == nil {
		return Usage{}, opserrors.Wrap(opserrors.KindCallback, "流式输出回调函数不能为空", nil)
	}

	params, err := c.newChatParams(messages)
	if err != nil {
		return Usage{}, opserrors.Wrap(opserrors.KindProtocol, "构造模型请求失败", err)
	}
	params.StreamOptions = openai.ChatCompletionStreamOptionsParam{
		// 请求服务端在最后一个业务 chunk 中返回完整 usage；若流被中断，该 chunk 可能不会到达。
		IncludeUsage: openai.Bool(true),
	}

	// OpenAI SDK 会在内部识别并吞掉 [DONE]，但当前公开 API 只有 Next、Current、Err 和 Close。
	// 正常收到 [DONE] 与服务端未发送 [DONE] 就直接 EOF，都会表现为 Next=false 且 Err=nil。
	// 因此在传输层旁路记录完成标记；SSE 分帧、JSON 解码和错误处理仍完全交给官方 SDK。
	tracker := &streamCompletionTracker{}
	stream := c.client.Chat.Completions.NewStreaming(
		ctx,
		params,
		option.WithMiddleware(tracker.middleware),
	)
	defer stream.Close()

	var usage Usage
	// Next 只表示“是否还有可消费的业务 chunk”，不能单独证明服务端已完整结束本次流。
	for stream.Next() {
		chunk := stream.Current()
		// include_usage 返回的最后一个 chunk 没有 choices，只携带本次请求的完整 token 统计。
		if chunk.JSON.Usage.Valid() {
			usage = usageFromCompletion(chunk.Usage, true)
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content == "" {
				continue
			}
			if err := onChunk(choice.Delta.Content); err != nil {
				return usage, opserrors.Wrap(opserrors.KindCallback, "处理流式输出失败", err)
			}
		}
	}

	// 优先返回调用方取消，避免底层连接关闭错误掩盖真正的取消原因。
	if ctx.Err() != nil {
		return usage, opserrors.Wrap(opserrors.KindCanceled, "流式请求已取消", ctx.Err())
	}
	// SDK 能报告 HTTP、SSE 扫描和 JSON 反序列化错误，这些错误应保留在错误链中。
	if err := stream.Err(); err != nil {
		return usage, wrapSDKError("解析流式响应失败", err)
	}
	// Err=nil 只说明读取过程没有底层错误；还必须确认协议级终止标记确实到达。
	if !tracker.completed() {
		return usage, opserrors.Wrap(opserrors.KindProtocol, "流式响应提前结束，未收到 [DONE]", nil)
	}

	return usage, nil
}

// newChatParams 把项目消息模型转换为 SDK 参数，避免 SDK 类型泄漏到应用层。
func (c *ChatCompletionClient) newChatParams(messages []Message) (openai.ChatCompletionNewParams, error) {
	sdkMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case "system":
			sdkMessages = append(sdkMessages, openai.SystemMessage(message.Content))
		case "user":
			sdkMessages = append(sdkMessages, openai.UserMessage(message.Content))
		case "assistant":
			sdkMessages = append(sdkMessages, openai.AssistantMessage(message.Content))
		default:
			return openai.ChatCompletionNewParams{}, fmt.Errorf("不支持的消息角色: %q", message.Role)
		}
	}

	return openai.ChatCompletionNewParams{
		Model:    c.model,
		Messages: sdkMessages,
	}, nil
}

// usageFromCompletion 统一转换 SDK usage，并显式保留未知状态。
func usageFromCompletion(value openai.CompletionUsage, known bool) Usage {
	if !known {
		return Usage{}
	}

	return Usage{
		PromptTokens:     int(value.PromptTokens),
		CompletionTokens: int(value.CompletionTokens),
		TotalTokens:      int(value.TotalTokens),
		Known:            true,
	}
}

// wrapSDKError 增加面向用户的上下文，同时保留官方 SDK 的底层错误类型。
func wrapSDKError(operation string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return opserrors.Wrap(opserrors.KindCanceled, operation, err)
	}
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return opserrors.NewHTTPError(apiErr.StatusCode, err)
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) {
		return opserrors.Wrap(opserrors.KindNetwork, operation, err)
	}
	return opserrors.Wrap(opserrors.KindProtocol, operation, err)
}

type streamCompletionTracker struct {
	// tail 保存上一次读取末尾的少量字节，用于处理 [DONE] 被拆到两次网络读取中的情况。
	tail []byte
	// done 表示原始响应体中已经观察到 Chat Completions 的协议终止标记。
	done bool
}

// middleware 在 SDK 创建 SSE decoder 前包装响应体，只观察原始字节，不消费或修改响应内容。
func (t *streamCompletionTracker) middleware(
	request *http.Request,
	next option.MiddlewareNext,
) (*http.Response, error) {
	response, err := next(request)
	if err != nil || response == nil || response.Body == nil {
		return response, err
	}

	response.Body = &trackingReadCloser{
		ReadCloser: response.Body,
		tracker:    t,
	}
	return response, nil
}

func (t *streamCompletionTracker) observe(data []byte) {
	if t.done || len(data) == 0 {
		return
	}

	const marker = "data: [DONE]"
	// 拼接上一次读取的尾部，确保终止标记跨网络分片时仍然能够被识别。
	combined := append(append([]byte(nil), t.tail...), data...)
	if bytes.Contains(combined, []byte(marker)) {
		t.done = true
		return
	}

	// 未命中时最多保留 marker 长度减一的尾部，既支持跨分片匹配，也避免累计整个响应体。
	keep := len(marker) - 1
	if len(combined) > keep {
		combined = combined[len(combined)-keep:]
	}
	// 保存尾部字节，最终用来校验流式输出内容是否结束
	t.tail = append(t.tail[:0], combined...)
}

func (t *streamCompletionTracker) completed() bool {
	return t.done
}

// trackingReadCloser 把 SDK 对响应体的每次读取同步转交给 tracker 观察。
// 原始字节和读取错误都会原样返回给 SDK，不改变官方 SDK 的解析行为。
type trackingReadCloser struct {
	io.ReadCloser
	tracker *streamCompletionTracker
}

func (r *trackingReadCloser) Read(data []byte) (int, error) {
	read, err := r.ReadCloser.Read(data)
	// 只观察本次真正读取到的字节，不能把缓冲区中未写入的区域交给 tracker。
	r.tracker.observe(data[:read])
	return read, err
}

func (r *trackingReadCloser) Close() error {
	return r.ReadCloser.Close()
}
