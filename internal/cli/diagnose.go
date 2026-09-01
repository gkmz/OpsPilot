package cli

import (
	"context"
	"io"

	"github.com/chzyer/readline"
	"github.com/gkmz/opspilot/internal/app"
	"github.com/spf13/cobra"
)

func newDiagnoseCommand(stdin io.Reader, stdout, stderr io.Writer) *cobra.Command {
	var noSession bool
	command := &cobra.Command{
		Use:   "diagnose [symptom]",
		Short: "开始一轮交互式故障诊断",
		Args:  cobra.ArbitraryArgs,
	}
	command.RunE = func(command *cobra.Command, args []string) error {
		return runDiagnose(command.Context(), args, stdin, stdout, stderr, noSession)
	}
	command.Flags().BoolVar(&noSession, "no-session", false, "不保存本次会话")
	return command
}

func runDiagnose(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, noSession bool) error {
	input, err := newReadline(stdin, stdout, stderr)
	if err != nil {
		return err
	}
	defer input.Close()

	go closeReadlineOnCancel(ctx, input)
	if noSession {
		return runDiagnoseWithoutSession(ctx, args, input, stdout, stderr)
	}
	return app.RunWithLineReader(ctx, args, input.Readline, stdout, stderr)
}

func runDiagnoseWithoutSession(ctx context.Context, args []string, input *readline.Instance, stdout, stderr io.Writer) error {
	return app.RunWithLineReaderAndStore(ctx, args, input.Readline, stdout, stderr, nil)
}

func newReadline(stdin io.Reader, stdout, stderr io.Writer) (*readline.Instance, error) {
	input, ok := stdin.(io.ReadCloser)
	if !ok {
		input = io.NopCloser(stdin)
	}
	return readline.NewEx(&readline.Config{
		Prompt:                 "> ",
		Stdin:                  input,
		Stdout:                 stdout,
		Stderr:                 stderr,
		HistoryLimit:           -1,
		DisableAutoSaveHistory: true,
		InterruptPrompt:        "^C",
		EOFPrompt:              "",
	})
}

func closeReadlineOnCancel(ctx context.Context, input *readline.Instance) {
	<-ctx.Done()
	_ = input.Close()
}
