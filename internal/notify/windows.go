package notify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const toastScript = `$ErrorActionPreference = 'Stop'
[void][Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime]
$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$text = $template.GetElementsByTagName('text')
[void]$text.Item(0).AppendChild($template.CreateTextNode($env:HUNTER_ALERT_TITLE))
[void]$text.Item(1).AppendChild($template.CreateTextNode($env:HUNTER_ALERT_BODY))
$toast = [Windows.UI.Notifications.ToastNotification]::new($template)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($env:HUNTER_ALERT_TITLE).Show($toast)`

type Windows struct {
	run func(ctx context.Context, name string, env []string, args ...string) error
}

func NewWindows() *Windows {
	return &Windows{
		run: func(ctx context.Context, name string, env []string, args ...string) error {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Env = env
			return cmd.Run()
		},
	}
}

func (w *Windows) Send(ctx context.Context, alert Alert) error {
	body := alert.Message
	if strings.HasPrefix(strings.ToLower(alert.URL), "https://") {
		body += "\n" + alert.URL
	}
	env := append(os.Environ(),
		"HUNTER_ALERT_TITLE="+alertTitle,
		"HUNTER_ALERT_BODY="+body,
	)
	args := []string{"-NoProfile", "-NonInteractive", "-Command", toastScript}
	if err := w.run(ctx, "powershell.exe", env, args...); err != nil {
		return fmt.Errorf("showing notification: %w", err)
	}
	return nil
}
