// Package cli 提供 OpsPilot 的命令行入口和交互式输入适配。
package cli

import (
	"context"
	"io"

	"github.com/spf13/cobra"
)

// NewRootCommand 创建 OpsPilot 根命令及其子命令。
func NewRootCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:          "opspilot",
		Short:        "面向运维场景的智能诊断助手",
		SilenceUsage: true,
	}
	root.AddCommand(newDiagnoseCommand(stdin, stdout, stderr))
	return root
}

// Execute 使用指定上下文执行根命令。
func Execute(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error {
	command := NewRootCommand(stdin, stdout, stderr)
	command.SetOut(stdout)
	command.SetErr(stderr)
	return command.ExecuteContext(ctx)
}
