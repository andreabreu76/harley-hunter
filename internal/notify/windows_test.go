package notify

import (
	"context"
	"strings"
	"testing"
)

type recordedShell struct {
	name string
	env  []string
	args []string
}

func TestWindowsPassesTheMessageThroughTheEnvironment(t *testing.T) {
	var calls []recordedShell
	n := NewWindows()
	n.run = func(ctx context.Context, name string, env []string, args ...string) error {
		calls = append(calls, recordedShell{name: name, env: env, args: args})
		return nil
	}

	alert := Alert{
		Message: `Harley "Street Glide" 2014 - R$ 72.000 [olx]`,
		URL:     "https://pr.olx.com.br/motos/harley-1509210244",
	}
	if err := n.Send(context.Background(), alert); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("ran %d commands, want 1", len(calls))
	}

	got := calls[0]
	if !strings.HasPrefix(got.name, "powershell") {
		t.Errorf("command = %q, want powershell", got.name)
	}
	script := strings.Join(got.args, " ")
	if strings.Contains(script, "Street Glide") {
		t.Errorf("the listing title was interpolated into the script:\n%s", script)
	}
	if !strings.Contains(script, "HUNTER_ALERT_BODY") {
		t.Errorf("the script does not read the message from the environment:\n%s", script)
	}

	var carried bool
	for _, pair := range got.env {
		if pair == "HUNTER_ALERT_BODY="+alert.Message+"\n"+alert.URL {
			carried = true
		}
	}
	if !carried {
		t.Errorf("the message never reached the environment:\n%q", got.env)
	}
}
