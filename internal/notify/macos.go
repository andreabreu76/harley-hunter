package notify

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const (
	alertTitle  = "Harley Hunter"
	alertSound  = "Glass"
	notifierBin = "terminal-notifier"
)

type MacOS struct {
	binaryPath string
	run        func(ctx context.Context, name string, args ...string) error
}

func NewMacOS() *MacOS {
	return newMacOS(exec.LookPath)
}

func newMacOS(lookPath func(string) (string, error)) *MacOS {
	path, err := lookPath(notifierBin)
	if err != nil {
		path = ""
	}
	return &MacOS{
		binaryPath: path,
		run: func(ctx context.Context, name string, args ...string) error {
			return exec.CommandContext(ctx, name, args...).Run()
		},
	}
}

func (m *MacOS) Send(ctx context.Context, alert Alert) error {
	if m.binaryPath != "" {
		args := []string{"-title", alertTitle, "-message", alert.Message}
		if alert.URL != "" {
			args = append(args, "-open", alert.URL)
		}
		args = append(args, "-sound", alertSound)
		if err := m.run(ctx, m.binaryPath, args...); err != nil {
			return fmt.Errorf("showing notification: %w", err)
		}
		return nil
	}

	script := fmt.Sprintf(`display notification "%s" with title "%s" sound name "%s"`,
		escapeAppleScript(alert.Message), alertTitle, alertSound)
	if err := m.run(ctx, "osascript", "-e", script); err != nil {
		return fmt.Errorf("showing notification: %w", err)
	}
	return nil
}

func escapeAppleScript(s string) string {
	s = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(s)
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}
