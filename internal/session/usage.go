// Package session 定义会话复盘所需的数据模型和持久化边界。
package session

import "github.com/gkmz/opspilot/internal/llm"

// TurnUsage 表示一轮已经完成的模型调用所消耗的 Token。
//
// Known=false 表示 Provider 没有返回可信的完整统计，数值字段不能被当作准确值展示。
type TurnUsage struct {
	PromptTokens     int  `json:"prompt_tokens"`
	CompletionTokens int  `json:"completion_tokens"`
	TotalTokens      int  `json:"total_tokens"`
	Known            bool `json:"known"`
}

// UsageSummary 表示当前会话所有已记录轮次的累计 Token 使用量。
//
// 只要任意一轮 usage 未知，Known 就为 false，调用方不能把累计数字当作完整统计。
type UsageSummary struct {
	TurnCount        int  `json:"turn_count"`
	PromptTokens     int  `json:"prompt_tokens"`
	CompletionTokens int  `json:"completion_tokens"`
	TotalTokens      int  `json:"total_tokens"`
	Known            bool `json:"known"`
}

// NewTurnUsage 将模型客户端返回的 usage 转换为会话层的单轮记录。
func NewTurnUsage(usage llm.Usage) TurnUsage {
	return TurnUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		Known:            usage.Known,
	}
}

// Add 将一轮 usage 累加到会话摘要中，并传播未知状态。
func (s *UsageSummary) Add(usage TurnUsage) {
	if s.TurnCount == 0 {
		s.Known = usage.Known
	} else {
		s.Known = s.Known && usage.Known
	}
	s.TurnCount++
	s.PromptTokens += usage.PromptTokens
	s.CompletionTokens += usage.CompletionTokens
	s.TotalTokens += usage.TotalTokens
}
