package notify

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const notifySendBin = "notify-send"

type Linux struct {
	binaryPath string
	run        func(ctx context.Context, name string, args ...string) error
}

func NewLinux() *Linux {
	path, err := exec.LookPath(notifySendBin)
	if err != nil {
		path = ""
	}
	return &Linux{
		binaryPath: path,
		run: func(ctx context.Context, name string, args ...string) error {
			return exec.CommandContext(ctx, name, args...).Run()
		},
	}
}

func (l *Linux) Send(ctx context.Context, alert Alert) error {
	if l.binaryPath == "" {
		return fmt.Errorf("%s is not installed: install libnotify-bin on Debian or Ubuntu, libnotify on Fedora or Arch, and the pending alerts appear on the next round", notifySendBin)
	}
	body := alert.Message
	if strings.HasPrefix(strings.ToLower(alert.URL), "https://") {
		body += "\n" + alert.URL
	}
	args := []string{"--app-name", alertTitle, "--urgency", "normal", alertTitle, body}
	if err := l.run(ctx, l.binaryPath, args...); err != nil {
		return fmt.Errorf("showing notification: %w", err)
	}
	return nil
}
