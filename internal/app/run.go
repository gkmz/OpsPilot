// Package app 负责组装配置、模型客户端和 CLI 用例。
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gkmz/opspilot/internal/config"
	"github.com/gkmz/opspilot/internal/diagnosis"
	"github.com/gkmz/opspilot/internal/llm"
)

// Run 执行 OpsPilot v0.1 的单次故障分析命令。
//
// 当前命令只负责一次非流式请求；多轮、流式和会话持久化会在后续学习单元中加入。
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	// 读取控制台输入的症状内容（文本）
	symptom, err := readSymptom(args, stdin)
	if err != nil {
		return err
	}

	// 从环境变量读取大模型配置
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	// 创建大模型client
	client := llm.NewClient(cfg.BaseURL, cfg.APIKey, cfg.Model, cfg.Timeout)
	result, err := client.Chat(ctx, diagnosis.BuildMessages(symptom))
	if err != nil {
		return err
	}

	fmt.Fprintln(stdout, result.Content)
	if result.Usage.Known {
		fmt.Fprintf(stderr, "usage: prompt=%d completion=%d total=%d\n", result.Usage.PromptTokens, result.Usage.CompletionTokens, result.Usage.TotalTokens)
	}
	return nil
}

// 从启动参数读取症状描述
func readSymptom(args []string, stdin io.Reader) (string, error) {
	symptom := strings.TrimSpace(strings.Join(args, " "))
	if symptom != "" {
		return symptom, nil
	}

	// 从输入读取数据，最大1M
	data, err := io.ReadAll(io.LimitReader(stdin, 1<<20))
	if err != nil {
		return "", fmt.Errorf("读取故障描述失败: %w", err)
	}
	symptom = strings.TrimSpace(string(data))
	if symptom == "" {
		return "", errors.New("请通过参数或标准输入提供故障描述")
	}
	return symptom, nil
}
