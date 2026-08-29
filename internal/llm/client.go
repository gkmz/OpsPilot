// Package llm 提供当前阶段使用的 OpenAI 兼容模型端点客户端。
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

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

// Client 调用一个 OpenAI 兼容的 chat completions 端点。
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	model      string
}

// NewClient 创建单次调用阶段使用的模型客户端。
func NewClient(baseURL, apiKey, model string, timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
	}
}

// Chat 发起一次非流式聊天请求并返回模型文本和 usage。
//
// 取消由 ctx 控制；HTTP 错误、协议错误和模型业务错误都返回给上层，不在本阶段自动重试。
func (c *Client) Chat(ctx context.Context, messages []Message) (Response, error) {
	payload := chatRequest{Model: c.model, Messages: messages}
	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, fmt.Errorf("编码模型请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("创建模型请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("请求模型失败: %w", err)
	}
	defer resp.Body.Close()

	// 8M
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return Response{}, fmt.Errorf("读取模型响应失败: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return Response{}, fmt.Errorf("模型返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var decoded chatResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return Response{}, fmt.Errorf("解析模型响应失败: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return Response{}, fmt.Errorf("模型响应缺少 choices")
	}

	usage := Usage{}
	if decoded.Usage != nil {
		usage = Usage{
			PromptTokens:     decoded.Usage.PromptTokens,
			CompletionTokens: decoded.Usage.CompletionTokens,
			TotalTokens:      decoded.Usage.TotalTokens,
			Known:            true,
		}
	}
	return Response{Content: decoded.Choices[0].Message.Content, Usage: usage}, nil
}

// Stream 发起流式聊天请求，并通过 onChunk 按增量返回模型文本。
//
// 只有收到服务端的 [DONE] 标记后，当前请求才会被视为完整成功。
// 如果请求被取消、服务端提前断开或响应格式错误，则不会返回成功结果。
func (c *Client) Stream(ctx context.Context, messages []Message, onChunk func(string) error) (Usage, error) {
	if onChunk == nil {
		return Usage{}, errors.New("流式输出回调函数不能为空")
	}
	payload := chatRequest{Model: c.model, Messages: messages, Stream: true}
	body, err := json.Marshal(payload)
	if err != nil {
		return Usage{}, fmt.Errorf("编码模型请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Usage{}, fmt.Errorf("创建模型请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	// 1. 设置 SSE 必需的 Header
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Usage{}, fmt.Errorf("请求模型失败: %w", err)
	}
	defer resp.Body.Close()

	// 开始流式接收
	// 流式请求也必须先检查 HTTP 状态码。
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if readErr != nil {
			return Usage{}, fmt.Errorf("读取模型错误响应失败: %w", readErr)
		}

		return Usage{}, fmt.Errorf(
			"模型返回 HTTP %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}

	var usage Usage
	receivedDone := false

	// Scanner 按行读取 SSE 响应，而不是一次性读取整个响应体。
	scanner := bufio.NewScanner(resp.Body)

	// 增大单行限制，避免模型返回较长事件时触发 Scanner 默认限制。
	scanner.Buffer(make([]byte, 64*1024), 1<<20)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 忽略 SSE 的空行和注释行。
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		if !strings.HasPrefix(line, "data:") {
			return Usage{}, fmt.Errorf("流式响应格式错误: %q", line)
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

		// [DONE] 表示服务端已经完整发送本次回答。
		if data == "[DONE]" {
			receivedDone = true
			break
		}

		var chunk chatStreamResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return Usage{}, fmt.Errorf("解析流式响应失败: %w", err)
		}

		// 某些兼容端点会在最后一个事件中返回 usage。
		if chunk.Usage != nil {
			usage = Usage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
				Known:            true,
			}
		}

		// 逐个处理当前事件中的增量文本。
		for _, choice := range chunk.Choices {
			content := choice.Delta.Content
			if content == "" {
				continue
			}

			if err := onChunk(content); err != nil {
				return usage, fmt.Errorf("处理流式输出失败: %w", err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return usage, fmt.Errorf("流式请求已取消: %w", ctx.Err())
		}
		return usage, fmt.Errorf("读取流式响应失败: %w", err)
	}

	// 如果上下文已经取消，即使 Scanner 没有返回底层错误，也要优先报告取消。
	if ctx.Err() != nil {
		return usage, fmt.Errorf("流式请求已取消: %w", ctx.Err())
	}

	// 没有收到 [DONE]，说明响应可能被服务端提前截断。
	if !receivedDone {
		return usage, errors.New("流式响应提前结束，未收到 [DONE]")
	}

	return usage, nil
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"` // openai官方参数，开启流式输出
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type chatStreamResponse struct {
	Choices []struct {
		Delta struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`

	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}
