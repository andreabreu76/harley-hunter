package normalize

import "testing"

func TestParsePhoneAcceptsTheShapesRealAdsUse(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"Interessados chame nesse contato: 11 98241-3574 Wilson", "11982413574"},
		{"personalidade e excelente estado de conservação.  15 981122787 ", "15981122787"},
		{"(21) 99799-0964", "21997990964"},
		{"(21)99799-0964", "21997990964"},
		{"whats +55 21 99799-0964", "21997990964"},
		{"55 21 99799-0964", "21997990964"},
		{"21999790964", "21999790964"},
		{"5521999790964", "21999790964"},
		{"21.99799.0964", "21997990964"},
		{"tel (11) 9 8241-3574", "11982413574"},
		{"fone fixo (11) 3255-1234", "1132551234"},
		{"ligar no 11 3255 1234", "1132551234"},
		{"ligue 0 21 99799-0964", "21997990964"},
	}
	for _, c := range cases {
		got, ok := ParsePhone(c.text)
		if !ok || got != c.want {
			t.Errorf("ParsePhone(%q) = %q, %v, want %q, true", c.text, got, ok, c.want)
		}
	}
}

func TestParsePhoneRejectsNumbersThatAreNotPhones(t *testing.T) {
	cases := []string{
		"",
		"Moto em excelente estado",
		"R$ 185.000,00 à vista",
		"Exatamente com 18600km",
		"motor Milwaukee-Eight 117",
		"parcelamos em até 24x sem juros",
		"2014/2015 Street Glide",
		"chassi 1HD1KBM19FB612345",
		"moto 2014 com 43.500 km por R$ 74.900",
		"CNPJ 12.345.678/0001-90",
		"CEP 18087190",
		"0800 123 4567",
		"código do anúncio: 1499153946",
		"tel 2199790964",
		"01 99799-0964",
		"11 19799-0964",
		"11 1255-1234",
		"021999790964",
	}
	for _, text := range cases {
		if got, ok := ParsePhone(text); ok {
			t.Errorf("ParsePhone(%q) = %q, true, want no phone", text, got)
		}
	}
}

func TestParsePhoneTakesTheFirstUsableNumber(t *testing.T) {
	text := "loja aberta desde 2011, chame no 11 91234-5678 ou 11 3255-1234"
	got, ok := ParsePhone(text)
	if !ok || got != "11912345678" {
		t.Errorf("ParsePhone(%q) = %q, %v, want 11912345678, true", text, got, ok)
	}
}
