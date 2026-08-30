package conversation

import (
	"strings"

	"github.com/gkmz/opspilot/internal/llm"
	"github.com/gkmz/opspilot/internal/session"
)

// Conversation 保存当前进程内的多轮聊天消息。
type Conversation struct {
	messages   []llm.Message
	turnUsages []session.TurnUsage
	usage      session.UsageSummary
}

// ConversationSnapshot 表示某一时刻的会话完整快照。
//
// 快照同时包含消息历史、每轮 usage 和会话累计 usage，后续可直接转换为持久化记录。
type ConversationSnapshot struct {
	Messages   []llm.Message
	TurnUsages []session.TurnUsage
	Usage      session.UsageSummary
}

// New 创建一个包含初始系统消息的会话。
func New(systemMessage llm.Message) *Conversation {
	return &Conversation{messages: []llm.Message{systemMessage}}
}

// CommitAssistant 将完整的 assistant 回复提交到历史消息中。
func (c *Conversation) CommitAssistant(content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	c.messages = append(c.messages, llm.Message{
		Role:    "assistant",
		Content: content,
	})
}

// CommitUser 将用户输入提交到历史消息中。
func (c *Conversation) CommitUser(content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	c.messages = append(c.messages, llm.Message{
		Role:    "user",
		Content: content,
	})
}

// Messages 返回会话历史的副本，避免调用方修改内部状态。
func (c *Conversation) Messages() []llm.Message {
	messages := make([]llm.Message, len(c.messages))
	copy(messages, c.messages)
	return messages
}

// CommitUsage 记录一轮已完成调用的 usage，并更新当前会话累计摘要。
func (c *Conversation) CommitUsage(usage session.TurnUsage) {
	c.turnUsages = append(c.turnUsages, usage)
	c.usage.Add(usage)
}

// Usage 返回当前会话累计 usage 的副本。
func (c *Conversation) Usage() session.UsageSummary {
	return c.usage
}

// TurnUsages 返回每一轮 usage 的副本，避免调用方修改会话内部状态。
func (c *Conversation) TurnUsages() []session.TurnUsage {
	usages := make([]session.TurnUsage, len(c.turnUsages))
	copy(usages, c.turnUsages)
	return usages
}

// Snapshot 返回当前会话的完整快照，并复制其中的切片数据以隔离内部状态。
func (c *Conversation) Snapshot() ConversationSnapshot {
	return ConversationSnapshot{
		Messages:   c.Messages(),
		TurnUsages: c.TurnUsages(),
		Usage:      c.Usage(),
	}
}
