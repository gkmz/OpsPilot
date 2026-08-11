// Package llm 提供当前阶段使用的 OpenAI 兼容模型端点客户端。
package llm

import (
	"bytes"
	"context"
	"encoding/json"
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

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
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
