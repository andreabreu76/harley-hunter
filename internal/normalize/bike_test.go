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
		{"Harley Davidson Street Glide 2015", model.BikeStreetGlide, model.VariantBase},
		{"HD STREET GLIDE ESPECIAL 15/15 IMPECAVEL", model.BikeStreetGlide, model.VariantSpecial},
		{"Street Glide Special 2014", model.BikeStreetGlide, model.VariantSpecial},
		{"streetglide 2015", model.BikeStreetGlide, model.VariantBase},
		{"Harley FLHXS 2015", model.BikeStreetGlide, model.VariantSpecial},
		{"CVO Street Glide 2015", model.BikeStreetGlide, model.VariantCVO},
		{"Road Glide Special 2015 - valor a combinar", model.BikeRoadGlide, model.VariantSpecial},
		{"roadglide 2015", model.BikeRoadGlide, model.VariantBase},
		{"Harley FLTRXSE 2015", model.BikeRoadGlide, model.VariantCVO},
		{"Electra Glide Ultra Limited 2015", model.BikeElectraGlide, model.VariantUnknown},
		{"Harley Davidson Ultra Limited 2014", model.BikeUltra, model.VariantUnknown},
		{"Harley Davidson Touring 1690 2015", model.BikeTouringUnknown, model.VariantUnknown},
		{"Honda Gold Wing 2015", model.BikeOther, model.VariantUnknown},
		{"Harley Davidson Iron 883", model.BikeOther, model.VariantUnknown},
		{"harley-davidson street-glide 2015", model.BikeStreetGlide, model.VariantBase},
		{"Harley-Davidson Road-Glide Special 2015", model.BikeRoadGlide, model.VariantSpecial},
		{"H-D Street Glide 2014", model.BikeStreetGlide, model.VariantBase},
		{"Harley-Davidson Electra-Glide 2015", model.BikeElectraGlide, model.VariantUnknown},
		{"Harley FLHX Street Glide 2015", model.BikeStreetGlide, model.VariantBase},
		{"Harley FLTRX Road Glide 2015", model.BikeRoadGlide, model.VariantBase},
		{"Harley FLTRX-SE 2015", model.BikeRoadGlide, model.VariantCVO},
		{"Harley FLTRX SE 2015", model.BikeRoadGlide, model.VariantCVO},
		{"Harley FLHXSE 2015", model.BikeStreetGlide, model.VariantCVO},
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

func TestFoldRemovesAccents(t *testing.T) {
	if got := Fold("SÃO JOSÉ DOS PINHAIS"); got != "sao jose dos pinhais" {
		t.Errorf("Fold = %q, want %q", got, "sao jose dos pinhais")
	}
}

const poisonedStreetGlideSpecial = `Harley-Davidson STREET GLIDE SPECIAL 114 - 2022 - R$ 124.990,00

Apenas 13.800 KM

ÚNICO DONO
TODAS REVISÕES NA HARLEY-DAVIDSON

#motosite
#harleydavidson
#harleydavidsonstreetglide
#streetglide
#roadglide
#harleytouring
#cvo
#specialstreetglide`

const poisonedStreetGlide = `Alerta de pão quente.
🏍️ Street Glide
📅ANO: 2020/20
📌KM: 18.000
💰R$ 107.000,00

🏪Estamos disponíveis 24 horas.

#streetglide #harleydavidson #roadglide #roadking #harley bagger softail sportster dyna motorcycle hd harleylife baggernation streetglidespecial harleydavidsonmotorcycles harleydavidsondaily motorcycles custom performancebagger baggers bikelife cvo roadglidespecial fatboy harleysofinstagram harleys electraglide harleydavidsonindonesia vicla roadglidenation`

func TestDetectBikeIgnoresTheHashtagFooter(t *testing.T) {
	bike, variant := DetectBike(poisonedStreetGlideSpecial)
	if bike != model.BikeStreetGlide {
		t.Errorf("bike = %q, want street_glide: #roadglide in the footer is not the bike being sold", bike)
	}
	if variant != model.VariantSpecial {
		t.Errorf("variant = %q, want special: #cvo in the footer is not the trim being sold", variant)
	}
}

func TestDetectBikeIgnoresTheKeywordTailAfterTheHashtags(t *testing.T) {
	bike, _ := DetectBike(poisonedStreetGlide)
	if bike != model.BikeStreetGlide {
		t.Errorf("bike = %q, want street_glide: the caption body says Street Glide and the tail is keyword stuffing", bike)
	}
}

func TestDetectBikeStillReadsACaptionThatIsOnlyHashtags(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"#streetglide", model.BikeStreetGlide},
		{"#harleydavidson #roadglide", model.BikeRoadGlide},
		{"vendo barata #electraglide", model.BikeElectraGlide},
	}
	for _, c := range cases {
		if bike, _ := DetectBike(c.text); bike != c.want {
			t.Errorf("DetectBike(%q) = %q, want %q: hashtags are the only signal here", c.text, bike, c.want)
		}
	}
}

func TestDetectBikeKeepsTheBodyWhenItNamesTheFamily(t *testing.T) {
	text := "Harley Davidson Electra Glide 2014 impecável #streetglide #harley"
	if bike, _ := DetectBike(text); bike != model.BikeElectraGlide {
		t.Errorf("bike = %q, want electra_glide: the body wins over the footer", bike)
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
	if bike, variant := DetectBike("𝐒𝐭𝐫𝐞𝐞𝐭 𝐆𝐥𝐢𝐝𝐞 𝐒𝐩𝐞𝐜𝐢𝐚𝐥 2015"); bike != model.BikeStreetGlide || variant != model.VariantSpecial {
		t.Errorf("DetectBike = %q/%q, want street_glide/special", bike, variant)
	}
}
