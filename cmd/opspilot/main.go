// Package main 提供 OpsPilot CLI 的进程入口。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gkmz/opspilot/internal/app"
)

// main 将命令行参数和标准输入交给应用层处理，并将错误转换为非零退出码。
func main() {
	ctx, stop := newRunContext(context.Background())
	defer stop()

	if err := app.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// newRunContext 创建响应 Ctrl+C 和终止信号的运行上下文。
func newRunContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}
