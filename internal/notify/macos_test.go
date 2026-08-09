package notify

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type recordedCommand struct {
	ctx  context.Context
	name string
	args []string
}

func recorder() (*[]recordedCommand, func(ctx context.Context, name string, args ...string) error) {
	var calls []recordedCommand
	return &calls, func(ctx context.Context, name string, args ...string) error {
		calls = append(calls, recordedCommand{ctx: ctx, name: name, args: args})
		return nil
	}
}

func scriptFor(t *testing.T, message string) string {
	t.Helper()
	calls, run := recorder()
	n := NewMacOS()
	n.run = run

	if err := n.Send(context.Background(), message); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("ran %d commands, want 1", len(*calls))
	}
	c := (*calls)[0]
	if c.name != "osascript" {
		t.Errorf("command = %q, want osascript", c.name)
	}
	if len(c.args) != 2 || c.args[0] != "-e" {
		t.Fatalf("args = %q, want -e followed by one script", c.args)
	}
	return c.args[1]
}

func TestMacOSSendBuildsTheNotificationScript(t *testing.T) {
	got := scriptFor(t, "Harley-Davidson Street Glide 2014 - R$ 72.000 - curitiba/PR [olx] https://olx.com.br/abc")

	want := `display notification "Harley-Davidson Street Glide 2014 - R$ 72.000 - curitiba/PR [olx] https://olx.com.br/abc" with title "Harley Hunter" sound name "Glass"`
	if got != want {
		t.Errorf("script =\n%s\nwant\n%s", got, want)
	}
}

func TestMacOSSendEscapesDoubleQuotes(t *testing.T) {
	got := scriptFor(t, `Harley "Street Glide" 2014`)

	want := `display notification "Harley \"Street Glide\" 2014" with title "Harley Hunter" sound name "Glass"`
	if got != want {
		t.Errorf("script =\n%s\nwant\n%s", got, want)
	}
}

func TestMacOSSendEscapesBackslashesBeforeQuotes(t *testing.T) {
	got := scriptFor(t, `back\slash and "quote"`)

	want := `display notification "back\\slash and \"quote\"" with title "Harley Hunter" sound name "Glass"`
	if got != want {
		t.Errorf("script =\n%s\nwant\n%s", got, want)
	}
}

func TestMacOSSendKeepsAppleScriptOutOfTheMessage(t *testing.T) {
	injection := `" & (do shell script "touch /tmp/pwned") & "`
	got := scriptFor(t, injection)

	body := strings.TrimSuffix(strings.TrimPrefix(got, `display notification "`), `" with title "Harley Hunter" sound name "Glass"`)
	if strings.Contains(strings.ReplaceAll(body, `\"`, ""), `"`) {
		t.Errorf("body %q still carries an unescaped quote, so the message can close the literal and run code", body)
	}
}

func TestMacOSSendFlattensLineBreaks(t *testing.T) {
	got := scriptFor(t, "first line\nsecond\rthird")

	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("script %q carries a raw line break, which osascript rejects as a syntax error", got)
	}
	for _, want := range []string{"first line", "second", "third"} {
		if !strings.Contains(got, want) {
			t.Errorf("script %q dropped %q", got, want)
		}
	}
}

func TestMacOSSendReportsRunnerFailure(t *testing.T) {
	n := NewMacOS()
	n.run = func(ctx context.Context, name string, args ...string) error {
		return errors.New("no gui session")
	}

	err := n.Send(context.Background(), "teste")
	if err == nil {
		t.Fatal("Send should surface an osascript failure so the round can retry next time")
	}
	if !strings.Contains(err.Error(), "no gui session") {
		t.Errorf("error %q loses the underlying cause", err)
	}
}

func TestMacOSSendPassesTheContextThrough(t *testing.T) {
	calls, run := recorder()
	n := NewMacOS()
	n.run = run

	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "carried")
	if err := n.Send(ctx, "teste"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if (*calls)[0].ctx.Value(key{}) != "carried" {
		t.Error("Send must hand its context to the command so a cancelled round stops the notifier")
	}
}

func TestNewMacOSRunsARealCommandByDefault(t *testing.T) {
	if NewMacOS().run == nil {
		t.Fatal("NewMacOS must ship a runner, otherwise Send panics outside tests")
	}
}
