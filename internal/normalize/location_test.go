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
		{"Curitiba-PR", "curitiba", "PR"},
		{"Niterói-RJ", "niteroi", "RJ"},
		{"Mogi das Cruzes-SP", "mogi das cruzes", "SP"},
		{"Curitiba PR", "curitiba", "PR"},
		{"Sao Jose dos Pinhais PR", "sao jose dos pinhais", "PR"},
		{"Campinas - São Paulo", "campinas", "SP"},
		{"Volta Redonda - Rio de Janeiro", "volta redonda", "RJ"},
		{"Cabo Frio, Rio de Janeiro", "cabo frio", "RJ"},
		{"Rio de Janeiro", "rio de janeiro", "RJ"},
		{"São Paulo", "sao paulo", "SP"},
		{"Rio de Janeiro, Copacabana", "rio de janeiro", ""},
		{"São Paulo, Moema", "sao paulo", ""},
		{"Vila Mariana, São Paulo", "vila mariana", "SP"},
		{"São Paulo Zona Sul", "sao paulo", "SP"},
		{"Rio de Janeiro Zona Oeste", "rio de janeiro", "RJ"},
		{"Curitiba Centro", "curitiba", "PR"},
		{"Campinas - São Paulo - Brasil", "campinas", "SP"},
		{"Volta Redonda - Rio de Janeiro - Brasil", "volta redonda", "RJ"},
		{"Santos, São Paulo (Zona Leste)", "santos", "SP"},
		{"Vila Isabel, Volta Redonda - RJ", "volta redonda", "RJ"},
		{"Centro, Campinas - SP", "campinas", "SP"},
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
		{"campinas", "SP", "state"},
		{"volta redonda", "RJ", "state"},
	}
	for _, c := range cases {
		t.Run(c.city+"/"+c.state, func(t *testing.T) {
			if got := LocationTier(c.city, c.state); got != c.want {
				t.Errorf("LocationTier(%q, %q) = %q, want %q", c.city, c.state, got, c.want)
			}
		})
	}
}

func TestParseLocationReadsAStyledCity(t *testing.T) {
	city, state := ParseLocation("𝐂𝐮𝐫𝐢𝐭𝐢𝐛𝐚 - 𝐏𝐑")
	if city != "curitiba" || state != "PR" {
		t.Errorf("ParseLocation = %q/%q, want curitiba/PR", city, state)
	}
}
