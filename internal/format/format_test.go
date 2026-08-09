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
