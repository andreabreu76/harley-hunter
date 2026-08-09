package normalize

import "testing"

func TestParsePrice(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"R$ 74.900", 7490000, true},
		{"R$ 74.900,00", 7490000, true},
		{"74900", 7490000, true},
		{"74,9 mil", 7490000, true},
		{"75 mil", 7500000, true},
		{"R$ 68.500,50", 6850050, true},
		{"R$ 74.900 negociável", 7490000, true},
		{"R$ 74,9 mil", 7490000, true},
		{"Vendo Street Glide 15/15, 42.000 km, R$ 74.900", 7490000, true},
		{"a combinar", 0, false},
		{"Consulte", 0, false},
		{"", 0, false},
		{"R$ 1,00", 0, false},
		{"12.000 km", 0, false},
		{"12 mil km", 0, false},
		{"12 mil kms", 0, false},
		{"42 mil kms rodados", 0, false},
		{"12 mil quilometros", 0, false},
		{"Vendo Road Glide, 42 mil kms rodados, aceito troca", 0, false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, ok := ParsePrice(c.in)
			if ok != c.ok {
				t.Fatalf("ParsePrice(%q) ok = %v, want %v", c.in, ok, c.ok)
			}
			if ok && got != c.want {
				t.Errorf("ParsePrice(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}
