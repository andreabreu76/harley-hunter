package normalize

import (
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

func TestDetectBike(t *testing.T) {
	cases := []struct {
		in          string
		wantBike    string
		wantVariant string
	}{
		{"Harley Davidson Sportster 1200 Custom 2016", model.BikeSportster1200, model.VariantCustom},
		{"HD XL1200C 2017", model.BikeSportster1200, model.VariantCustom},
		{"Harley-Davidson Sportster XL 1200 Custom 2016", model.BikeSportster1200, model.VariantCustom},
		{"Harley-Davidson Forty Eight 2018", model.BikeSportster1200, model.VariantFortyEight},
		{"HARLEY FORTY-EIGHT 2016 IMPECAVEL", model.BikeSportster1200, model.VariantFortyEight},
		{"fortyeight 2017", model.BikeSportster1200, model.VariantFortyEight},
		{"Harley XL1200X 2016", model.BikeSportster1200, model.VariantFortyEight},
		{"Harley 48 2017", model.BikeSportster1200, model.VariantFortyEight},
		{"Harley Davidson Iron 1200 2019", model.BikeSportster1200, model.VariantIron},
		{"HD XL1200NS 2020", model.BikeSportster1200, model.VariantIron},
		{"Harley Roadster 1200 2017", model.BikeSportster1200, model.VariantRoadster},
		{"Harley-Davidson XL1200CX Roadster 2018", model.BikeSportster1200, model.VariantRoadster},
		{"Harley Davidson Sportster 1200 2016", model.BikeSportster1200, model.VariantBase},
		{"harley-davidson sportster-1200 2016", model.BikeSportster1200, model.VariantBase},
		{"Harley Davidson Seventy Two 2016", model.BikeSportster1200, model.VariantBase},
		{"Harley Davidson Iron 883 2019", model.BikeSportster883, model.VariantUnknown},
		{"HD XL883N 2018", model.BikeSportster883, model.VariantUnknown},
		{"Sportster 883 2017", model.BikeSportster883, model.VariantUnknown},
		{"Harley Davidson Sportster S 2023", model.BikeSportsterS, model.VariantUnknown},
		{"Harley Davidson Sportster 2018", model.BikeSportsterUnknown, model.VariantUnknown},
		{"Harley Davidson Iron", model.BikeSportsterUnknown, model.VariantUnknown},
		{"Harley Davidson XR1200 2010", model.BikeSportsterUnknown, model.VariantUnknown},
		{"Honda Gold Wing 2015", model.BikeOther, model.VariantUnknown},
		{"Harley Davidson Street Glide 2015", model.BikeOther, model.VariantUnknown},
		{"BMW R 1200 GS 2016", model.BikeOther, model.VariantUnknown},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			bike, variant := DetectBike(c.in)
			if bike != c.wantBike {
				t.Errorf("DetectBike(%q) bike = %q, want %q", c.in, bike, c.wantBike)
			}
			if variant != c.wantVariant {
				t.Errorf("DetectBike(%q) variant = %q, want %q", c.in, variant, c.wantVariant)
			}
		})
	}
}

func TestDetectBikeDoesNotReadAPriceAsTheFortyEight(t *testing.T) {
	cases := []string{
		"Harley Davidson Sportster 1200 por R$ 48.000,00",
		"Harley Davidson Sportster 1200 - 48.000 km rodados",
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			_, variant := DetectBike(in)
			if variant == model.VariantFortyEight {
				t.Errorf("variant = forty_eight: the 48 here is money or mileage, not the model")
			}
		})
	}
}

func TestDetectBikeNeedsTheDisplacementToTellIronApart(t *testing.T) {
	if bike, _ := DetectBike("Harley Davidson Iron 883 2020"); bike != model.BikeSportster883 {
		t.Errorf("bike = %q, want sportster_883", bike)
	}
	if bike, variant := DetectBike("Harley Davidson Iron 1200 2020"); bike != model.BikeSportster1200 || variant != model.VariantIron {
		t.Errorf("bike/variant = %q/%q, want sportster_1200/iron", bike, variant)
	}
	if bike, _ := DetectBike("Harley Davidson Iron seminova"); bike != model.BikeSportsterUnknown {
		t.Errorf("bike = %q, want sportster_unknown: Iron alone is 883 or 1200", bike)
	}
}

func TestFoldRemovesAccents(t *testing.T) {
	if got := Fold("SÃO JOSÉ DOS PINHAIS"); got != "sao jose dos pinhais" {
		t.Errorf("Fold = %q, want %q", got, "sao jose dos pinhais")
	}
}

const poisonedIron1200 = `Harley-Davidson IRON 1200 - 2019 - R$ 43.990,00

Apenas 9.800 KM

ÚNICO DONO
TODAS REVISÕES NA HARLEY-DAVIDSON

#motosite
#harleydavidson
#sportster883
#iron883
#fortyeight
#harleysportster`

const poisonedFortyEight = `Alerta de pão quente.
🏍️ Forty Eight
📅ANO: 2018/18
📌KM: 12.000
💰R$ 44.000,00

🏪Estamos disponíveis 24 horas.

#fortyeight #harleydavidson #iron1200 #sportster883 #harley sportster dyna motorcycle hd harleylife roadster nightster streetglide softail fatboy harleysofinstagram sportsters custom`

func TestDetectBikeIgnoresTheHashtagFooter(t *testing.T) {
	bike, variant := DetectBike(poisonedIron1200)
	if bike != model.BikeSportster1200 {
		t.Errorf("bike = %q, want sportster_1200: #sportster883 in the footer is not the bike being sold", bike)
	}
	if variant != model.VariantIron {
		t.Errorf("variant = %q, want iron: #fortyeight in the footer is not the trim being sold", variant)
	}
}

func TestDetectBikeIgnoresTheKeywordTailAfterTheHashtags(t *testing.T) {
	bike, variant := DetectBike(poisonedFortyEight)
	if bike != model.BikeSportster1200 {
		t.Errorf("bike = %q, want sportster_1200: the caption body says Forty Eight and the tail is keyword stuffing", bike)
	}
	if variant != model.VariantFortyEight {
		t.Errorf("variant = %q, want forty_eight", variant)
	}
}

func TestDetectBikeStillReadsACaptionThatIsOnlyHashtags(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"#iron1200", model.BikeSportster1200},
		{"#harleydavidson #fortyeight", model.BikeSportster1200},
		{"vendo barata #sportster883", model.BikeSportster883},
	}
	for _, c := range cases {
		if bike, _ := DetectBike(c.text); bike != c.want {
			t.Errorf("DetectBike(%q) = %q, want %q: hashtags are the only signal here", c.text, bike, c.want)
		}
	}
}

func TestDetectBikeKeepsTheBodyWhenItNamesTheFamily(t *testing.T) {
	text := "Harley Davidson Sportster 883 2018 impecável #fortyeight #harley"
	if bike, _ := DetectBike(text); bike != model.BikeSportster883 {
		t.Errorf("bike = %q, want sportster_883: the body wins over the footer", bike)
	}
}

func TestFoldFlattensStyledUnicodeToPlainLetters(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"✅ 𝐕𝐄𝐍𝐃𝐈𝐃𝐎", "✅ vendido"},
		{"𝗕𝗜𝗚 𝗧𝗪𝗜𝗡", "big twin"},
		{"𝐄𝐬𝐩𝐞𝐜𝐢𝐚𝐥𝐢𝐳𝐚𝐝𝐚 𝐇𝐀𝐑𝐋𝐄𝐘-𝐃𝐀𝐕𝐈𝐃𝐒𝐎𝐍 𝐞𝐦 𝐉𝐨𝐚̃𝐨 𝐏𝐞𝐬𝐬𝐨𝐚.", "especializada harley-davidson em joao pessoa."},
		{"São Paulo", "sao paulo"},
		{"HARLEY-DAVIDSON", "harley-davidson"},
	}
	for _, c := range cases {
		if got := Fold(c.in); got != c.want {
			t.Errorf("Fold(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDetectBikeReadsAStyledCaption(t *testing.T) {
	if bike, variant := DetectBike("𝐈𝐫𝐨𝐧 𝟏𝟐𝟎𝟎 2019"); bike != model.BikeSportster1200 || variant != model.VariantIron {
		t.Errorf("DetectBike = %q/%q, want sportster_1200/iron", bike, variant)
	}
}

func TestDetectBikeInReadsTheTrimFromTheTitleNotTheAccessoryList(t *testing.T) {
	cases := []struct {
		id    int
		title string
		body  string
	}{
		{224, "HARLEY-DAVIDSON SPORTSTER 1200 CUSTOM", "Harley-Davidson Sportster 1200 Custom XL1200C 2017. Guidao no estilo Forty Eight. Escapamento esportivo."},
		{254, "HARLEY-DAVIDSON IRON 1200", "IRON 1200 2019 , MOTO SEM DETALHES , REVISADA NA CONCESSIONARIA. Banco solo igual ao da Roadster."},
	}
	for _, c := range cases {
		_, variant := DetectBikeIn(c.title, c.title+" "+c.body)
		if variant == model.VariantFortyEight || variant == model.VariantRoadster {
			t.Errorf("listing %d: variant = %q, want the trim in the title: the other name is an accessory in the body", c.id, variant)
		}
	}
}

func TestDetectBikeInReadsTheTrimCodeFromAnywhere(t *testing.T) {
	_, variant := DetectBikeIn("HARLEY-DAVIDSON SPORTSTER", "HARLEY-DAVIDSON SPORTSTER Harley-Davidson XL1200NS 2019")
	if variant != model.VariantIron {
		t.Errorf("variant = %q, want iron: the trim code is unambiguous wherever it sits", variant)
	}
}

func TestDetectBikeTreatsABareNameAsItsOwnTitle(t *testing.T) {
	if bike, variant := DetectBike("Forty Eight 2016"); bike != model.BikeSportster1200 || variant != model.VariantFortyEight {
		t.Errorf("DetectBike = %q/%q, want sportster_1200/forty_eight: a bare model name is a title", bike, variant)
	}
}
