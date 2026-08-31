package notify

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestLinuxSendsThroughNotifySend(t *testing.T) {
	calls, run := recorder()
	n := NewLinux()
	n.binaryPath = "/usr/bin/notify-send"
	n.run = run

	alert := Alert{
		Message: "Harley-Davidson Street Glide 2014 - R$ 72.000 - curitiba/PR [olx]",
		URL:     "https://pr.olx.com.br/motos/harley-1509210244",
	}
	if err := n.Send(context.Background(), alert); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("ran %d commands, want 1", len(*calls))
	}

	got := (*calls)[0]
	if got.name != "/usr/bin/notify-send" {
		t.Errorf("command = %q, want the cached notify-send path", got.name)
	}
	want := []string{
		"--app-name", "Harley Hunter",
		"--urgency", "normal",
		"Harley Hunter",
		alert.Message + "\n" + alert.URL,
	}
	if !reflect.DeepEqual(got.args, want) {
		t.Errorf("args =\n%q\nwant\n%q", got.args, want)
	}
}

func TestLinuxFailsLoudlyWithoutNotifySend(t *testing.T) {
	n := NewLinux()
	n.binaryPath = ""

	err := n.Send(context.Background(), Alert{Message: "any"})
	if err == nil {
		t.Fatal("Send reported success without notify-send installed")
	}
	if !strings.Contains(err.Error(), "notify-send") {
		t.Errorf("error = %q, want it to name the missing program", err)
	}
}

func TestLinuxKeepsNonHTTPSURLsOutOfTheBody(t *testing.T) {
	for _, url := range []string{
		"javascript:alert(document.cookie)",
		"file:///etc/passwd",
		"http://olx.com.br/abc",
		"data:text/html,<script>x</script>",
	} {
		calls, run := recorder()
		n := NewLinux()
		n.binaryPath = "/usr/bin/notify-send"
		n.run = run

		if err := n.Send(context.Background(), Alert{Message: "teste", URL: url}); err != nil {
			t.Fatalf("Send: %v", err)
		}
		body := (*calls)[0].args[len((*calls)[0].args)-1]
		if body != "teste" {
			t.Errorf("url %q reached the body as %q, want the message alone: notify-send turns the body into a clickable link", url, body)
		}
	}
}

func TestLinuxSendReportsRunnerFailure(t *testing.T) {
	n := NewLinux()
	n.binaryPath = "/usr/bin/notify-send"
	n.run = func(ctx context.Context, name string, args ...string) error {
		return errors.New("no dbus session")
	}

	err := n.Send(context.Background(), Alert{Message: "teste"})
	if err == nil {
		t.Fatal("Send should surface a failure so the listing stays pending for the next round")
	}
	if !strings.Contains(err.Error(), "no dbus session") {
		t.Errorf("error %q loses the underlying cause", err)
	}
}
