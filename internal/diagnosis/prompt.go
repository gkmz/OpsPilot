// Package diagnosis 定义故障分析场景的输入和最小 Prompt。
package diagnosis

import "github.com/gkmz/OpsPilot/internal/llm"

const systemPrompt = `你是一个 Go HTTP 微服务故障分析助手。

当前只能基于用户提供的故障现象进行初步分析，不要把猜测写成事实。
请按以下结构回答：
1. 现象摘要
2. 可能原因（标注不确定性）
3. 建议补充的证据
4. 下一步排查顺序

如果缺少日志、指标、代码变更或运行环境信息，请明确指出缺口。`

// BuildMessages 将一条故障描述包装成当前版本的诊断消息。
func BuildMessages(symptom string) []llm.Message {
	return []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: symptom},
	}
}
