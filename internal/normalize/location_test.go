package normalize

import "testing"

func TestParseLocation(t *testing.T) {
	cases := []struct {
		in        string
		wantCity  string
		wantState string
	}{
		{"Curitiba - PR", "curitiba", "PR"},
		{"São Paulo, SP", "sao paulo", "SP"},
		{"Rio de Janeiro / RJ", "rio de janeiro", "RJ"},
		{"São José dos Pinhais - PR", "sao jose dos pinhais", "PR"},
		{"Niterói", "niteroi", ""},
		{"", "", ""},
		{"Rio de Janeiro - RJ - Brasil", "rio de janeiro", "RJ"},
		{"Guarulhos - SP (Cumbica)", "guarulhos", "SP"},
		{"São Paulo (SP)", "sao paulo", "SP"},
		{"Curitiba - Paraná", "curitiba", "PR"},
		{"Copacabana, Rio de Janeiro - RJ", "rio de janeiro", "RJ"},
		{"Embu-Guaçu - SP", "embu guacu", "SP"},
		{"Embu Guaçu - SP", "embu guacu", "SP"},
		{"Lapa, São Paulo - SP", "sao paulo", "SP"},
		{"Curitiba- PR", "curitiba", "PR"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			city, state := ParseLocation(c.in)
			if city != c.wantCity || state != c.wantState {
				t.Errorf("ParseLocation(%q) = (%q, %q), want (%q, %q)", c.in, city, state, c.wantCity, c.wantState)
			}
		})
	}
}

func TestLocationTier(t *testing.T) {
	cases := []struct {
		city  string
		state string
		want  string
	}{
		{"sao paulo", "SP", "metro"},
		{"guarulhos", "SP", "metro"},
		{"niteroi", "RJ", "metro"},
		{"sao jose dos pinhais", "PR", "metro"},
		{"campinas", "SP", "state"},
		{"londrina", "PR", "state"},
		{"belo horizonte", "MG", "outside"},
		{"", "", "outside"},
		{"niteroi", "", "metro"},
		{"embu guacu", "SP", "metro"},
		{"lapa", "SP", "state"},
	}
	for _, c := range cases {
		t.Run(c.city+"/"+c.state, func(t *testing.T) {
			if got := LocationTier(c.city, c.state); got != c.want {
				t.Errorf("LocationTier(%q, %q) = %q, want %q", c.city, c.state, got, c.want)
			}
		})
	}
}
