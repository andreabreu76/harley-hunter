package format

import "testing"

func TestThousands(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1.000"},
		{72000, "72.000"},
		{4000, "4.000"},
		{1234567, "1.234.567"},
		{-1234, "-1.234"},
		{-123456, "-123.456"},
	}
	for _, c := range cases {
		if got := Thousands(c.in); got != c.want {
			t.Errorf("Thousands(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPhone(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"11982413574", "(11) 98241-3574"},
		{"1132551234", "(11) 3255-1234"},
		{"", ""},
		{"1198241357", "(11) 9824-1357"},
		{"119824135741", "119824135741"},
	}
	for _, c := range cases {
		if got := Phone(c.in); got != c.want {
			t.Errorf("Phone(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPhoneLink(t *testing.T) {
	if got, want := PhoneLink("11982413574"), "+5511982413574"; got != want {
		t.Errorf("PhoneLink = %q, want %q", got, want)
	}
	if got := PhoneLink(""); got != "" {
		t.Errorf("PhoneLink(%q) = %q, want empty", "", got)
	}
}

func TestPhoneLinkOnlyBuildsATelTargetFromRealDigits(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"11982413574", "+5511982413574"},
		{"4133334444", "+554133334444"},
		{"", ""},
		{"123", ""},
		{"119824135741234", ""},
		{"(11) 98241-3574", ""},
		{"11982413574 ", ""},
		{"1198241357a", ""},
		{"+5511982413574", ""},
	}
	for _, c := range cases {
		if got := PhoneLink(c.in); got != c.want {
			t.Errorf("PhoneLink(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
