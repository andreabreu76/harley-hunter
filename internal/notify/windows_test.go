package notify

import (
	"context"
	"errors"
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

func alertBodyIn(env []string) (string, bool) {
	const prefix = "HUNTER_ALERT_BODY="
	body, found := "", false
	for _, pair := range env {
		if strings.HasPrefix(pair, prefix) {
			body, found = strings.TrimPrefix(pair, prefix), true
		}
	}
	return body, found
}

func TestWindowsKeepsNonHTTPSURLsOutOfTheBody(t *testing.T) {
	for _, url := range []string{
		"javascript:alert(document.cookie)",
		"file:///etc/passwd",
		"http://olx.com.br/abc",
		"data:text/html,<script>x</script>",
	} {
		var calls []recordedShell
		n := NewWindows()
		n.run = func(ctx context.Context, name string, env []string, args ...string) error {
			calls = append(calls, recordedShell{name: name, env: env, args: args})
			return nil
		}

		if err := n.Send(context.Background(), Alert{Message: "teste", URL: url}); err != nil {
			t.Fatalf("Send: %v", err)
		}
		body, found := alertBodyIn(calls[0].env)
		if !found {
			t.Fatalf("url %q: no HUNTER_ALERT_BODY in the environment", url)
		}
		if body != "teste" {
			t.Errorf("url %q reached the body as %q, want the message alone: the toast turns the body into a clickable link", url, body)
		}
	}
}

func TestWindowsSendReportsRunnerFailure(t *testing.T) {
	n := NewWindows()
	n.run = func(ctx context.Context, name string, env []string, args ...string) error {
		return errors.New("powershell.exe not found")
	}

	err := n.Send(context.Background(), Alert{Message: "teste"})
	if err == nil {
		t.Fatal("Send should surface a failure so crawl.Notify leaves the listing pending for the next round")
	}
	if !strings.Contains(err.Error(), "powershell.exe not found") {
		t.Errorf("error %q loses the underlying cause", err)
	}
}
