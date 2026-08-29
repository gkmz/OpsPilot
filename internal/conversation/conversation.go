package conversation

import (
	"strings"

	"github.com/gkmz/opspilot/internal/llm"
)

// Conversation 保存当前进程内的多轮聊天消息。
type Conversation struct {
	messages []llm.Message
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
