package web

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/store"
)

var cardPattern = regexp.MustCompile(`(?s)<article class="card[^"]*">.*?</article>`)

func cardWith(t *testing.T, body, title string) string {
	t.Helper()
	for _, block := range cardPattern.FindAllString(body, -1) {
		if strings.Contains(block, title) {
			return block
		}
	}
	t.Fatalf("no card carries %q, body:\n%s", title, body)
	return ""
}

func glide(id, title string, km int, cents int64, fingerprint string) model.Listing {
	year := 2015
	return model.Listing{
		Source: "olx", ExternalID: id, URL: "https://example.com/" + id,
		Title: title, Bike: model.BikeStreetGlide, Year: &year, Km: &km,
		PriceCents: &cents, City: "curitiba", State: "PR",
		Verdict: model.VerdictMatch, Fingerprint: fingerprint,
	}
}

func from(l model.Listing, source string) model.Listing {
	l.Source = source
	return l
}

func expireEverythingUnseen(t *testing.T, s *store.Store, base time.Time) {
	t.Helper()
	for i := range 4 {
		at := base.Add(time.Duration(i+1) * time.Hour)
		if err := s.RecordRun("olx", at, at, 10, "ok", ""); err != nil {
			t.Fatalf("RecordRun: %v", err)
		}
	}
	if _, err := s.ExpireUnseen(3); err != nil {
		t.Fatalf("ExpireUnseen: %v", err)
	}
}

func TestGoneListingStaysVisibleAndIsMarkedClosed(t *testing.T) {
	s := emptyStore(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	id := upsert(t, s, glide("closed", "Glide Encerrada", 31000, 7200000, "abc"), base)
	expireEverythingUnseen(t, s, base)

	_, body := get(t, NewServer(s, []string{"olx"}), "/")
	if !strings.Contains(body, "Glide Encerrada") {
		t.Fatal("a closed listing must stay in its tab")
	}
	block := cardWith(t, body, "Glide Encerrada")
	if !strings.Contains(block, "encerrado") {
		t.Errorf("a closed listing should say so, card:\n%s", block)
	}
	if !strings.Contains(block, `class="card encerrada"`) {
		t.Errorf("a closed listing should render muted, card:\n%s", block)
	}

	_, detail := get(t, NewServer(s, []string{"olx"}), "/listing/"+strconv.FormatInt(id, 10))
	if !strings.Contains(detail, "encerrado") {
		t.Errorf("the detail page should say the listing is closed, body:\n%s", detail)
	}
}

func TestActiveListingCarriesNoClosedMarker(t *testing.T) {
	s := emptyStore(t)
	upsert(t, s, glide("live", "Glide Ativa", 31000, 7200000, "abc"), time.Now())

	_, body := get(t, NewServer(s, []string{"olx"}), "/")
	block := cardWith(t, body, "Glide Ativa")
	if strings.Contains(block, "encerrad") {
		t.Errorf("an active listing must not be marked closed, card:\n%s", block)
	}
}

func TestCheaperRepostIsFlaggedAndLinksToItsPredecessor(t *testing.T) {
	s := emptyStore(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	older := upsert(t, s, glide("first", "Glide Original", 31000, 7500000, "abc"), base)
	newer := upsert(t, s, glide("second", "Glide Reanunciada", 31200, 7100000, "abc"), base.Add(72*time.Hour))

	srv := NewServer(s, []string{"olx"})
	_, body := get(t, srv, "/")

	repost := cardWith(t, body, "Glide Reanunciada")
	if !strings.Contains(repost, "possível reanúncio") {
		t.Errorf("the repost should be flagged, card:\n%s", repost)
	}
	if !strings.Contains(repost, `href="/listing/`+strconv.FormatInt(older, 10)+`"`) {
		t.Errorf("the flag should link to the earlier listing, card:\n%s", repost)
	}
	if !strings.Contains(repost, "selo-baixa") {
		t.Errorf("a repost cheaper than its predecessor should get the accent, card:\n%s", repost)
	}
	if !strings.Contains(repost, "4.000 a menos") {
		t.Errorf("the flag should say how much cheaper the repost is, card:\n%s", repost)
	}

	original := cardWith(t, body, "Glide Original")
	if !strings.Contains(original, "possível reanúncio") {
		t.Errorf("the predecessor should point at its twin too, card:\n%s", original)
	}
	if strings.Contains(original, "selo-baixa") {
		t.Errorf("the earlier listing is not the cheaper repost, card:\n%s", original)
	}

	_, detail := get(t, srv, "/listing/"+strconv.FormatInt(newer, 10))
	if !strings.Contains(detail, "possível reanúncio") {
		t.Errorf("the detail page should flag the repost, body:\n%s", detail)
	}
	if !strings.Contains(detail, `href="/listing/`+strconv.FormatInt(older, 10)+`"`) {
		t.Errorf("the detail flag should link to the earlier listing, body:\n%s", detail)
	}
}

func TestTwinOnAnotherSourceSaysWhereElseItIsListed(t *testing.T) {
	s := emptyStore(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	older := upsert(t, s, glide("first", "Glide No Ml", 37234, 7500000, "abc"), base)
	newer := upsert(t, s, from(glide("second", "Glide Na Webmotors", 37234, 7100000, "abc"), "webmotors"),
		base.Add(72*time.Hour))

	_, body := get(t, NewServer(s, []string{"olx", "webmotors"}), "/")

	twin := cardWith(t, body, "Glide Na Webmotors")
	if !strings.Contains(twin, "anunciada também em olx") {
		t.Errorf("the same ad on another site is not a repost, card:\n%s", twin)
	}
	if strings.Contains(twin, "possível reanúncio") {
		t.Errorf("a cross-source twin must not claim a repost, card:\n%s", twin)
	}
	if !strings.Contains(twin, `href="/listing/`+strconv.FormatInt(older, 10)+`"`) {
		t.Errorf("the flag should link to the twin, card:\n%s", twin)
	}
	if !strings.Contains(twin, "selo-baixa") || !strings.Contains(twin, "4.000 a menos") {
		t.Errorf("the cheaper accent should survive the other label, card:\n%s", twin)
	}

	original := cardWith(t, body, "Glide No Ml")
	if !strings.Contains(original, "anunciada também em webmotors") {
		t.Errorf("the older listing should name the other site too, card:\n%s", original)
	}
	if !strings.Contains(original, `href="/listing/`+strconv.FormatInt(newer, 10)+`"`) {
		t.Errorf("the older listing should link to its twin, card:\n%s", original)
	}
}

func TestTwinOnTheSameSourceIsCalledARepost(t *testing.T) {
	s := emptyStore(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	upsert(t, s, glide("first", "Glide Original", 31000, 7500000, "abc"), base)
	upsert(t, s, glide("second", "Glide Reanunciada", 31200, 7100000, "abc"), base.Add(72*time.Hour))

	_, body := get(t, NewServer(s, []string{"olx"}), "/")
	repost := cardWith(t, body, "Glide Reanunciada")
	if !strings.Contains(repost, "possível reanúncio") {
		t.Errorf("a second ad on the same site is the repost case, card:\n%s", repost)
	}
	if strings.Contains(repost, "anunciada também em") {
		t.Errorf("a same-source twin is not a cross posting, card:\n%s", repost)
	}
}

func TestCheaperDeltaRoundsInsteadOfTruncating(t *testing.T) {
	s := emptyStore(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	upsert(t, s, glide("first", "Glide Original", 31000, 7550000, "abc"), base)
	upsert(t, s, glide("second", "Glide Reanunciada", 31200, 7100050, "abc"), base.Add(72*time.Hour))

	_, body := get(t, NewServer(s, []string{"olx"}), "/")
	repost := cardWith(t, body, "Glide Reanunciada")
	if !strings.Contains(repost, "4.500 a menos") {
		t.Errorf("a delta of R$ 4.499,50 should read as 4.500, card:\n%s", repost)
	}
}

func TestRepostThatIsNotCheaperKeepsTheQuietFlag(t *testing.T) {
	s := emptyStore(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	upsert(t, s, glide("first", "Glide Original", 31000, 7100000, "abc"), base)
	upsert(t, s, glide("second", "Glide Reanunciada", 31200, 7500000, "abc"), base.Add(72*time.Hour))

	_, body := get(t, NewServer(s, []string{"olx"}), "/")
	repost := cardWith(t, body, "Glide Reanunciada")
	if !strings.Contains(repost, "possível reanúncio") {
		t.Errorf("the repost should still be flagged, card:\n%s", repost)
	}
	if strings.Contains(repost, "selo-baixa") {
		t.Errorf("a dearer repost is not the seller in a hurry, card:\n%s", repost)
	}
}

func TestGoneListingIsStillOfferedAsAPredecessor(t *testing.T) {
	s := emptyStore(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	older := upsert(t, s, glide("first", "Glide Original", 31000, 7500000, "abc"), base)
	expireEverythingUnseen(t, s, base)
	upsert(t, s, glide("second", "Glide Reanunciada", 31200, 7100000, "abc"), base.Add(72*time.Hour))

	_, body := get(t, NewServer(s, []string{"olx"}), "/")
	repost := cardWith(t, body, "Glide Reanunciada")
	if !strings.Contains(repost, `href="/listing/`+strconv.FormatInt(older, 10)+`"`) {
		t.Errorf("a closed listing is exactly the predecessor worth linking, card:\n%s", repost)
	}
	if !strings.Contains(repost, "selo-baixa") {
		t.Errorf("the cheaper repost of a closed listing should get the accent, card:\n%s", repost)
	}
}

func TestListingsWithoutMileageNeverClaimARepost(t *testing.T) {
	s := emptyStore(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	first := glide("first", "Glide Sem Km", 0, 7500000, "abc")
	first.Km = nil
	second := glide("second", "Glide Sem Km Dois", 0, 7100000, "abc")
	second.Km = nil
	upsert(t, s, first, base)
	upsert(t, s, second, base.Add(72*time.Hour))

	_, body := get(t, NewServer(s, []string{"olx"}), "/")
	if strings.Contains(body, "reanúncio") {
		t.Errorf("without mileage the fingerprint cannot claim a repost, body:\n%s", body)
	}
}

func TestUniqueListingCarriesNoRepostFlag(t *testing.T) {
	s := emptyStore(t)
	upsert(t, s, glide("only", "Glide Sozinha", 31000, 7200000, "abc"), time.Now())

	_, body := get(t, NewServer(s, []string{"olx"}), "/")
	if strings.Contains(body, "reanúncio") {
		t.Errorf("a listing without twins must not be flagged, body:\n%s", body)
	}
}
