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
		{"42 mil km, valor 74 mil", 7400000, true},
		{"1 mil curtidas, moto por 74 mil", 7400000, true},
		{"20 mil seguidores no insta, vendo por 74 mil", 7400000, true},
		{"3 mil curtidas no post, R$ 74.900", 7490000, true},
		{"10 mil likes! Road Glide R$ 72.000", 7200000, true},
		{"20 mil comentários, moto por 74 mil", 7400000, true},
		{"Entrada de R$ 20.000, moto R$ 74.900", 7490000, true},
		{"R$ 74.900, aceito entrada de R$ 20.000", 7490000, true},
		{"Parcelas de R$ 1.800, valor total R$ 74.900", 7490000, true},
		{"Moto R$ 74.900, troco por ate R$ 90.000", 7490000, true},
		{"Street Glide R$ 74.900, aceito troca ate R$ 60.000", 7490000, true},
		{"Road Glide R$ 72.000, avalio moto ate R$ 95.000", 7200000, true},
		{"Aceito troca, R$ 74.900", 7490000, true},
		{"Vendo ou troco, R$ 74.900", 7490000, true},
		{"Moto avaliada em R$ 74.900", 7490000, true},
		{"Street Glide 2015 até 2016, R$ 74.900", 7490000, true},
		{"Aceito troca ate R$ 60.000, moto R$ 74.900", 7490000, true},
		{"Entrada de R$ 20.000, valor total 74 mil", 7400000, true},
		{"Entrada 20 mil, moto 74 mil", 7400000, true},
		{"Sinal de R$ 15.000, restante 74 mil", 7400000, true},
		{"Entrada 20 mil, Street Glide R$ 74.900", 7490000, true},
		{"R$ 74.900, troco por ate 90 mil", 7490000, true},
		{"Aceito troca ate 90 mil, moto R$ 74.900", 7490000, true},
		{"Moto 74 mil, troco por ate 90 mil", 7400000, true},
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
