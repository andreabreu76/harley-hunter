package source

import (
	"testing"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/normalize"
)

func TestFixturePhoneCoverageMatchesWhatTheAdsActuallyPublish(t *testing.T) {
	cases := []struct {
		source   string
		listings []model.RawListing
		want     map[string]string
	}{
		{
			source:   model.SourceWebmotors,
			listings: parseWebmotorsFixture(t, "testdata/webmotors-search.html"),
			want: map[string]string{
				"2290026": "15981122787",
				"2977981": "11982413574",
			},
		},
		{
			source:   model.SourceOLX,
			listings: parseFixture(t, "testdata/olx-search.html"),
			want:     map[string]string{},
		},
		{
			source:   model.SourceMercadoLivre,
			listings: parseMercadoLivreFixture(t, "testdata/mercadolivre-search.html"),
			want:     map[string]string{},
		},
		{
			source:   model.SourceMobiauto,
			listings: parseMobiautoFixture(t, "testdata/mobiauto-search.html"),
			want:     map[string]string{},
		},
	}

	for _, c := range cases {
		got := make(map[string]string)
		for _, raw := range c.listings {
			if phone := normalize.Normalize(raw).Phone; phone != nil {
				got[raw.ExternalID] = *phone
			}
		}
		if len(got) != len(c.want) {
			t.Errorf("%s: found %d phones %v, want %d %v", c.source, len(got), got, len(c.want), c.want)
			continue
		}
		for id, want := range c.want {
			if got[id] != want {
				t.Errorf("%s listing %s: phone = %q, want %q", c.source, id, got[id], want)
			}
		}
	}
}
