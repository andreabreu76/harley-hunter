package normalize

import "testing"

func TestParseKm(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"12.000 km", 12000, true},
		{"12000km", 12000, true},
		{"12 mil km", 12000, true},
		{"45.320 KM", 45320, true},
		{"", 0, false},
		{"R$ 74.900", 0, false},
		{"900.000 km", 0, false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, ok := ParseKm(c.in)
			if ok != c.ok {
				t.Fatalf("ParseKm(%q) ok = %v, want %v", c.in, ok, c.ok)
			}
			if ok && got != c.want {
				t.Errorf("ParseKm(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}
