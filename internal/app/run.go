// Package app 负责组装配置、模型客户端和 CLI 用例。
package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gkmz/opspilot/internal/config"
	"github.com/gkmz/opspilot/internal/conversation"
	"github.com/gkmz/opspilot/internal/diagnosis"
	"github.com/gkmz/opspilot/internal/llm"
	"github.com/gkmz/opspilot/internal/session"
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

	client := llm.NewClient(cfg)
	store, err := session.NewDefaultStore()
	if err != nil {
		return err
	}
	return RunInteractiveWithStore(ctx, client, args, stdin, stdout, store)
}

// RunInteractive 在同一个进程内执行多轮流式诊断。
func RunInteractive(ctx context.Context, client llm.Client, args []string, stdin io.Reader, output io.Writer) error {
	return runInteractive(ctx, client, args, stdin, output, nil)
}

// RunInteractiveWithStore 在同一个进程内执行多轮流式诊断，并保存成功会话。
func RunInteractiveWithStore(ctx context.Context, client llm.Client, args []string, stdin io.Reader, output io.Writer, store *session.Store) error {
	return runInteractive(ctx, client, args, stdin, output, store)
}

func runInteractive(ctx context.Context, client llm.Client, args []string, stdin io.Reader, output io.Writer, store *session.Store) error {
	chat := conversation.New(diagnosis.SystemMessage())
	sessionID, err := session.NewID()
	if err != nil {
		return err
	}
	createdAt := time.Now().UTC()

	// 只从命令行参数中读取第一轮问题，不读取 stdin。
	initialSymptom := strings.TrimSpace(strings.Join(args, " "))
	hasUserInput := false

	// 如果命令行带了参数，立即执行第一轮。
	if initialSymptom != "" {
		hasUserInput = true
		fmt.Fprintf(output, "> %s\n", initialSymptom)

		if usage, err := runTurn(
			ctx,
			client,
			chat,
			initialSymptom,
			output,
		); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			fmt.Fprintf(output, "\n请求失败: %v\n", err)
		} else {
			writeUsage(output, usage)
			writeSessionSaveWarning(output, saveConversation(store, sessionID, createdAt, chat))
		}

		fmt.Fprintln(output)
	}

	scanner := bufio.NewScanner(stdin)

	for {
		fmt.Fprint(output, "> ")

		var userInput string
		result := scanLine(scanner)

		// 同时监听context和scan的用户输入
		select {
		case <-ctx.Done():
			return ctx.Err()
		case scan := <-result:
			if scan.err != nil {
				return scan.err
			}
			if !scan.ok {
				if !hasUserInput {
					return errors.New("请通过参数或标准输入提供故障描述")
				}
				return nil
			}

			userInput = strings.TrimSpace(scan.line)
		}

		if userInput == "" {
			continue
		}

		if userInput == "/exit" || userInput == "/quit" {
			return nil
		}

		hasUserInput = true
		if usage, err := runTurn(
			ctx,
			client,
			chat,
			userInput,
			output,
		); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			fmt.Fprintf(output, "\n请求失败: %v\n", err)
			continue
		} else {
			writeUsage(output, usage)
			writeSessionSaveWarning(output, saveConversation(store, sessionID, createdAt, chat))
		}

		fmt.Fprintln(output)
	}
}

func saveConversation(store *session.Store, id string, createdAt time.Time, chat *conversation.Conversation) error {
	if store == nil {
		return nil
	}
	snapshot := chat.Snapshot()
	record := session.NewRecord(id, createdAt, snapshot.Messages, snapshot.TurnUsages, snapshot.Usage)
	record.UpdatedAt = time.Now().UTC()
	return store.Save(record)
}

func writeSessionSaveWarning(output io.Writer, err error) {
	if err != nil {
		fmt.Fprintf(output, "\n会话保存失败：%v", err)
	}
}

func writeUsage(output io.Writer, usage llm.Usage) {
	if usage.Known {
		fmt.Fprintf(output, "\nToken 使用：输入 %d，输出 %d，总计 %d", usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
	} else {
		fmt.Fprint(output, "\nToken 使用：未知")
	}
}

func runTurn(
	ctx context.Context,
	client llm.Client,
	conversation *conversation.Conversation,
	userInput string,
	output io.Writer,
) (llm.Usage, error) {
	conversation.CommitUser(userInput)

	var content strings.Builder

	usage, err := client.Stream(
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
		return llm.Usage{}, err
	}

	// 只有 Stream 成功完成后才提交完整回答。
	conversation.CommitAssistant(content.String())
	conversation.CommitUsage(session.NewTurnUsage(usage))

	return usage, nil
}

type scanResult struct {
	line string
	err  error
	ok   bool
}

func scanLine(scanner *bufio.Scanner) <-chan scanResult {
	result := make(chan scanResult, 1)

	go func() {
		// 把阻塞的Scan方法放到goroutine，以便外层可以监听Context
		if scanner.Scan() {
			result <- scanResult{line: scanner.Text(), ok: true}
			return
		}
		result <- scanResult{err: scanner.Err()}
	}()
	return result
}
