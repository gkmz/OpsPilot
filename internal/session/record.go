package session

import (
	"time"

	"github.com/gkmz/opspilot/internal/llm"
)

const currentRecordVersion = 1

// Record 表示可以保存到本地并用于复盘的一次会话记录。
//
// 记录只包含消息和 usage，不包含 API Key、Authorization Header 或 Base URL 等配置凭据。
type Record struct {
	Version    int           `json:"version"`
	ID         string        `json:"session_id"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
	Messages   []llm.Message `json:"messages"`
	TurnUsages []TurnUsage   `json:"turn_usages"`
	Usage      UsageSummary  `json:"usage"`
}

// NewRecord 根据会话快照创建一份独立的持久化记录。
//
// 输入切片会被复制，调用方后续修改原始快照不会影响记录内容。
func NewRecord(id string, now time.Time, messages []llm.Message, turnUsages []TurnUsage, usage UsageSummary) Record {
	return Record{
		Version:    currentRecordVersion,
		ID:         id,
		CreatedAt:  now,
		UpdatedAt:  now,
		Messages:   cloneMessages(messages),
		TurnUsages: cloneTurnUsages(turnUsages),
		Usage:      usage,
	}
}

func cloneMessages(messages []llm.Message) []llm.Message {
	cloned := make([]llm.Message, len(messages))
	copy(cloned, messages)
	return cloned
}

func cloneTurnUsages(usages []TurnUsage) []TurnUsage {
	cloned := make([]TurnUsage, len(usages))
	copy(cloned, usages)
	return cloned
}
