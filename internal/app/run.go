// Package app 负责组装配置、模型客户端和 CLI 用例。
package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gkmz/opspilot/internal/config"
	"github.com/gkmz/opspilot/internal/conversation"
	"github.com/gkmz/opspilot/internal/diagnosis"
	"github.com/gkmz/opspilot/internal/llm"
)

// Run 读取配置并执行带有初始命令行问题的交互式流式诊断。
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	client := llm.NewClient(cfg.BaseURL, cfg.APIKey, cfg.Model, cfg.Timeout)
	return RunInteractive(ctx, client, args, stdin, stdout)
}

// RunInteractive 在同一个进程内执行多轮流式诊断。
func RunInteractive(ctx context.Context, client *llm.Client, args []string, stdin io.Reader, output io.Writer) error {
	chat := conversation.New(diagnosis.SystemMessage())

	// 只从命令行参数中读取第一轮问题，不读取 stdin。
	initialSymptom := strings.TrimSpace(strings.Join(args, " "))
	hasUserInput := false

	// 如果命令行带了参数，立即执行第一轮。
	if initialSymptom != "" {
		hasUserInput = true
		fmt.Fprintf(output, "> %s\n", initialSymptom)

		if err := runTurn(
			ctx,
			client,
			chat,
			initialSymptom,
			output,
		); err != nil {
			fmt.Fprintf(output, "\n请求失败: %v\n", err)
		}

		fmt.Fprintln(output)
	}

	scanner := bufio.NewScanner(stdin)

	for {
		fmt.Fprint(output, "> ")

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("读取交互输入失败: %w", err)
			}
			if !hasUserInput {
				return errors.New("请通过参数或标准输入提供故障描述")
			}

			return nil
		}

		userInput := strings.TrimSpace(scanner.Text())
		if userInput == "" {
			continue
		}

		if userInput == "/exit" || userInput == "/quit" {
			return nil
		}

		hasUserInput = true
		if err := runTurn(
			ctx,
			client,
			chat,
			userInput,
			output,
		); err != nil {
			fmt.Fprintf(output, "\n请求失败: %v\n", err)
			continue
		}

		fmt.Fprintln(output)
	}
}

func runTurn(
	ctx context.Context,
	client *llm.Client,
	conversation *conversation.Conversation,
	userInput string,
	output io.Writer,
) error {
	conversation.CommitUser(userInput)

	var content strings.Builder

	_, err := client.Stream(
		ctx,
		conversation.Messages(),
		func(chunk string) error {
			if _, err := fmt.Fprint(output, chunk); err != nil {
				return err
			}

			content.WriteString(chunk)
			return nil
		},
	)
	if err != nil {
		// 流式失败时不提交不完整的 assistant 消息。
		return err
	}

	// 只有 Stream 成功完成后才提交完整回答。
	conversation.CommitAssistant(content.String())

	return nil
}
