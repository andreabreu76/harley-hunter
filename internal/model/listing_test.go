package model

import "testing"

func TestCombineVerdicts(t *testing.T) {
	cases := []struct {
		name string
		axes map[string]Verdict
		want Verdict
	}{
		{"all match", map[string]Verdict{"model": VerdictMatch, "year": VerdictMatch}, VerdictMatch},
		{"one maybe", map[string]Verdict{"model": VerdictMatch, "year": VerdictMaybe}, VerdictMaybe},
		{"reject wins over maybe", map[string]Verdict{"model": VerdictReject, "year": VerdictMaybe}, VerdictReject},
		{"empty is reject", map[string]Verdict{}, VerdictReject},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CombineVerdicts(c.axes); got != c.want {
				t.Errorf("CombineVerdicts(%v) = %q, want %q", c.axes, got, c.want)
			}
		})
	}
}
