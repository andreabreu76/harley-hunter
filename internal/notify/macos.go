package notify

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const (
	alertTitle = "Harley Hunter"
	alertSound = "Glass"
)

type MacOS struct {
	run func(ctx context.Context, name string, args ...string) error
}

func NewMacOS() *MacOS {
	return &MacOS{
		run: func(ctx context.Context, name string, args ...string) error {
			return exec.CommandContext(ctx, name, args...).Run()
		},
	}
}

func (m *MacOS) Send(ctx context.Context, message string) error {
	script := fmt.Sprintf(`display notification "%s" with title "%s" sound name "%s"`,
		escapeAppleScript(message), alertTitle, alertSound)
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
