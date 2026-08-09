package normalize

import "testing"

func TestParseYear(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"2015", 2015, true},
		{"2014/2015", 2015, true},
		{"15/15", 2015, true},
		{"14/15", 2015, true},
		{"HD STREET GLIDE ESPECIAL 15/15 IMPECAVEL", 2015, true},
		{"Harley Street Glide 2014", 2014, true},
		{"mod. 2015", 2015, true},
		{"", 0, false},
		{"Harley 1690 Rushmore", 0, false},
		{"3000", 0, false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, ok := ParseYear(c.in)
			if ok != c.ok {
				t.Fatalf("ParseYear(%q) ok = %v, want %v", c.in, ok, c.ok)
			}
			if ok && got != c.want {
				t.Errorf("ParseYear(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}
