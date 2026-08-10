package store

import (
	"testing"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/model"
)

func runAt(t *testing.T, s *Store, source string, at time.Time, items int, status string) {
	t.Helper()
	if err := s.RecordRun(source, at, at.Add(time.Minute), items, status, ""); err != nil {
		t.Fatalf("RecordRun: %v", err)
	}
}

func productiveRounds(t *testing.T, s *Store, source string, base time.Time, count int) {
	t.Helper()
	for i := range count {
		runAt(t, s, source, base.Add(time.Duration(i)*time.Hour), 10, "ok")
	}
}

func statusOf(t *testing.T, s *Store, id int64) string {
	t.Helper()
	row, _, err := s.GetRow(id)
	if err != nil {
		t.Fatalf("GetRow: %v", err)
	}
	return row.Status
}

func TestExpireUnseenMarksListingMissingForThreeRounds(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	res, err := s.Upsert(sample(7200000), base)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	productiveRounds(t, s, "olx", base, 4)

	expired, err := s.ExpireUnseen(3)
	if err != nil {
		t.Fatalf("ExpireUnseen: %v", err)
	}
	if expired != 1 {
		t.Fatalf("expired = %d, want 1", expired)
	}
	if got := statusOf(t, s, res.ID); got != StatusGone {
		t.Errorf("status = %q, want %q", got, StatusGone)
	}

	rows, err := s.ListByVerdict(model.VerdictMatch)
	if err != nil {
		t.Fatalf("ListByVerdict: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("a gone listing must stay visible in its tab, got %d rows", len(rows))
	}
}

func TestExpireUnseenSparesListingMissingForTwoRounds(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	res, err := s.Upsert(sample(7200000), base.Add(time.Hour))
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	productiveRounds(t, s, "olx", base, 4)

	expired, err := s.ExpireUnseen(3)
	if err != nil {
		t.Fatalf("ExpireUnseen: %v", err)
	}
	if expired != 0 {
		t.Fatalf("expired = %d, want 0: two missed rounds are not three", expired)
	}
	if got := statusOf(t, s, res.ID); got != StatusActive {
		t.Errorf("status = %q, want %q", got, StatusActive)
	}
}

func TestExpireUnseenIgnoresRoundsTheSourceFailed(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	res, err := s.Upsert(sample(7200000), base)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	runAt(t, s, "olx", base, 10, "ok")
	for i := 1; i <= 3; i++ {
		runAt(t, s, "olx", base.Add(time.Duration(i)*time.Hour), 0, "error")
	}

	expired, err := s.ExpireUnseen(3)
	if err != nil {
		t.Fatalf("ExpireUnseen: %v", err)
	}
	if expired != 0 {
		t.Fatalf("expired = %d, want 0: a source that failed brought no evidence the ad is gone", expired)
	}
	if got := statusOf(t, s, res.ID); got != StatusActive {
		t.Errorf("status = %q, want %q", got, StatusActive)
	}
}

func TestExpireUnseenIgnoresRoundsThatCollectedNothing(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	res, err := s.Upsert(sample(7200000), base)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	runAt(t, s, "olx", base, 10, "ok")
	for i := 1; i <= 3; i++ {
		runAt(t, s, "olx", base.Add(time.Duration(i)*time.Hour), 0, "ok")
	}

	expired, err := s.ExpireUnseen(3)
	if err != nil {
		t.Fatalf("ExpireUnseen: %v", err)
	}
	if expired != 0 {
		t.Fatalf("expired = %d, want 0: an empty round proves nothing about a single ad", expired)
	}
	if got := statusOf(t, s, res.ID); got != StatusActive {
		t.Errorf("status = %q, want %q", got, StatusActive)
	}
}

func TestExpireUnseenWaitsForEnoughHistory(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	if _, err := s.Upsert(sample(7200000), base.Add(-time.Hour)); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	productiveRounds(t, s, "olx", base, 2)

	expired, err := s.ExpireUnseen(3)
	if err != nil {
		t.Fatalf("ExpireUnseen: %v", err)
	}
	if expired != 0 {
		t.Errorf("expired = %d, want 0: two rounds of history cannot prove three misses", expired)
	}
}

func TestExpireUnseenJudgesEachSourceOnItsOwnHistory(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	watched := sample(7200000)
	olx, err := s.Upsert(watched, base)
	if err != nil {
		t.Fatalf("Upsert olx: %v", err)
	}
	quiet := sample(7200000)
	quiet.Source = "mercadolivre"
	quiet.ExternalID = "ml-1"
	ml, err := s.Upsert(quiet, base)
	if err != nil {
		t.Fatalf("Upsert mercadolivre: %v", err)
	}

	productiveRounds(t, s, "olx", base, 4)
	productiveRounds(t, s, "mercadolivre", base, 1)

	expired, err := s.ExpireUnseen(3)
	if err != nil {
		t.Fatalf("ExpireUnseen: %v", err)
	}
	if expired != 1 {
		t.Fatalf("expired = %d, want 1", expired)
	}
	if got := statusOf(t, s, olx.ID); got != StatusGone {
		t.Errorf("olx listing status = %q, want %q", got, StatusGone)
	}
	if got := statusOf(t, s, ml.ID); got != StatusActive {
		t.Errorf("mercadolivre listing status = %q, want %q: its source never ran three rounds", got, StatusActive)
	}
}

func TestExpireUnseenIsIdempotent(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	if _, err := s.Upsert(sample(7200000), base); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	productiveRounds(t, s, "olx", base, 4)

	if _, err := s.ExpireUnseen(3); err != nil {
		t.Fatalf("first ExpireUnseen: %v", err)
	}
	again, err := s.ExpireUnseen(3)
	if err != nil {
		t.Fatalf("second ExpireUnseen: %v", err)
	}
	if again != 0 {
		t.Errorf("second call expired %d rows, want 0", again)
	}
}

func TestExpireUnseenComparesInstantsAcrossTimestampPrecisions(t *testing.T) {
	s := openTemp(t)
	cutoff := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	late := sample(7200000)
	late.ExternalID = "late"
	seen, err := s.Upsert(late, cutoff.Add(time.Microsecond))
	if err != nil {
		t.Fatalf("Upsert late: %v", err)
	}
	early := sample(7200000)
	early.ExternalID = "early"
	missed, err := s.Upsert(early, cutoff.Add(-time.Microsecond))
	if err != nil {
		t.Fatalf("Upsert early: %v", err)
	}

	runAt(t, s, "olx", cutoff.Add(-time.Hour), 10, "ok")
	runAt(t, s, "olx", cutoff, 10, "ok")
	runAt(t, s, "olx", cutoff.Add(time.Hour), 10, "ok")
	runAt(t, s, "olx", cutoff.Add(2*time.Hour), 10, "ok")

	expired, err := s.ExpireUnseen(3)
	if err != nil {
		t.Fatalf("ExpireUnseen: %v", err)
	}
	if expired != 1 {
		t.Fatalf("expired = %d, want 1: only the sight one microsecond before the cutoff is stale", expired)
	}
	if got := statusOf(t, s, missed.ID); got != StatusGone {
		t.Errorf("listing last seen before the cutoff has status %q, want %q", got, StatusGone)
	}
	if got := statusOf(t, s, seen.ID); got != StatusActive {
		t.Errorf("listing last seen after the cutoff has status %q, want %q", got, StatusActive)
	}
}

func TestUpsertRevivesAGoneListing(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	res, err := s.Upsert(sample(7200000), base)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	productiveRounds(t, s, "olx", base, 4)
	if _, err := s.ExpireUnseen(3); err != nil {
		t.Fatalf("ExpireUnseen: %v", err)
	}
	if got := statusOf(t, s, res.ID); got != StatusGone {
		t.Fatalf("status before the reappearance = %q, want %q", got, StatusGone)
	}

	again, err := s.Upsert(sample(7200000), base.Add(5*time.Hour))
	if err != nil {
		t.Fatalf("Upsert on reappearance: %v", err)
	}
	if again.ID != res.ID {
		t.Fatalf("reappearance created listing %d instead of reviving %d", again.ID, res.ID)
	}
	if got := statusOf(t, s, res.ID); got != StatusActive {
		t.Errorf("status after the reappearance = %q, want %q", got, StatusActive)
	}
}

func TestPendingNotificationsSkipsGoneMatches(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	res, err := s.Upsert(sample(7200000), base)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	pending, err := s.PendingNotifications(10)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("an active match should be pending, got %d rows", len(pending))
	}

	productiveRounds(t, s, "olx", base, 4)
	if _, err := s.ExpireUnseen(3); err != nil {
		t.Fatalf("ExpireUnseen: %v", err)
	}

	pending, err = s.PendingNotifications(10)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("a gone match must not be alerted, got %d rows", len(pending))
	}

	if _, err := s.Upsert(sample(7200000), base.Add(5*time.Hour)); err != nil {
		t.Fatalf("Upsert on reappearance: %v", err)
	}
	pending, err = s.PendingNotifications(10)
	if err != nil {
		t.Fatalf("PendingNotifications: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != res.ID {
		t.Errorf("a revived match should be pending again, got %d rows", len(pending))
	}
}

func repostSample(id string, km int, cents int64, fingerprint string) model.Listing {
	l := sample(cents)
	l.ExternalID = id
	l.Km = &km
	l.Fingerprint = fingerprint
	return l
}

func TestRepostsOfFindsTheSiblingsSharingAFingerprint(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	first, err := s.Upsert(repostSample("old", 31000, 7500000, "abc"), base)
	if err != nil {
		t.Fatalf("Upsert old: %v", err)
	}
	second, err := s.Upsert(repostSample("new", 31200, 7100000, "abc"), base.Add(72*time.Hour))
	if err != nil {
		t.Fatalf("Upsert new: %v", err)
	}
	if _, err := s.Upsert(repostSample("other", 52000, 6900000, "zzz"), base); err != nil {
		t.Fatalf("Upsert other: %v", err)
	}

	siblings, err := s.RepostsOf("abc", second.ID)
	if err != nil {
		t.Fatalf("RepostsOf: %v", err)
	}
	if len(siblings) != 1 {
		t.Fatalf("got %d siblings, want 1", len(siblings))
	}
	if siblings[0].ID != first.ID {
		t.Errorf("sibling = %d, want %d", siblings[0].ID, first.ID)
	}
	if siblings[0].PriceCents == nil || *siblings[0].PriceCents != 7500000 {
		t.Errorf("sibling price = %v, want the earlier price", siblings[0].PriceCents)
	}
}

func TestRepostsOfIncludesGoneSiblings(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	first, err := s.Upsert(repostSample("old", 31000, 7500000, "abc"), base)
	if err != nil {
		t.Fatalf("Upsert old: %v", err)
	}
	productiveRounds(t, s, "olx", base, 4)
	if _, err := s.ExpireUnseen(3); err != nil {
		t.Fatalf("ExpireUnseen: %v", err)
	}
	second, err := s.Upsert(repostSample("new", 31200, 7100000, "abc"), base.Add(72*time.Hour))
	if err != nil {
		t.Fatalf("Upsert new: %v", err)
	}

	siblings, err := s.RepostsOf("abc", second.ID)
	if err != nil {
		t.Fatalf("RepostsOf: %v", err)
	}
	if len(siblings) != 1 || siblings[0].ID != first.ID {
		t.Fatalf("a closed ad is exactly the interesting predecessor, got %v", idsOf(siblings))
	}
	if siblings[0].Status != StatusGone {
		t.Errorf("sibling status = %q, want %q", siblings[0].Status, StatusGone)
	}
}

func TestRepostsOfIgnoresListingsWithoutMileage(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	bare := sample(7500000)
	bare.ExternalID = "bare-old"
	bare.Km = nil
	bare.Fingerprint = "kmless"
	if _, err := s.Upsert(bare, base); err != nil {
		t.Fatalf("Upsert bare: %v", err)
	}
	other := bare
	other.ExternalID = "bare-new"
	newer, err := s.Upsert(other, base.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("Upsert other: %v", err)
	}

	siblings, err := s.RepostsOf("kmless", newer.ID)
	if err != nil {
		t.Fatalf("RepostsOf: %v", err)
	}
	if len(siblings) != 0 {
		t.Errorf("got %d siblings, want 0: without mileage the fingerprint does not discriminate", len(siblings))
	}
}

func TestRepostsOfWithoutAFingerprintFindsNothing(t *testing.T) {
	s := openTemp(t)
	blank := sample(7500000)
	blank.Fingerprint = ""
	first, err := s.Upsert(blank, time.Now())
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	blank.ExternalID = "blank-2"
	if _, err := s.Upsert(blank, time.Now()); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	siblings, err := s.RepostsOf("", first.ID)
	if err != nil {
		t.Fatalf("RepostsOf: %v", err)
	}
	if len(siblings) != 0 {
		t.Errorf("got %d siblings, want 0: an empty fingerprint is not evidence", len(siblings))
	}
}

func TestRepostGroupsOnlyCarriesFingerprintsSeenMoreThanOnce(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	for _, l := range []model.Listing{
		repostSample("old", 31000, 7500000, "abc"),
		repostSample("new", 31200, 7100000, "abc"),
		repostSample("alone", 52000, 6900000, "zzz"),
	} {
		if _, err := s.Upsert(l, base); err != nil {
			t.Fatalf("Upsert %s: %v", l.ExternalID, err)
		}
	}
	bare := sample(7500000)
	bare.ExternalID = "bare"
	bare.Km = nil
	bare.Fingerprint = "kmless"
	if _, err := s.Upsert(bare, base); err != nil {
		t.Fatalf("Upsert bare: %v", err)
	}
	bare.ExternalID = "bare-2"
	if _, err := s.Upsert(bare, base); err != nil {
		t.Fatalf("Upsert bare-2: %v", err)
	}

	groups, err := s.RepostGroups()
	if err != nil {
		t.Fatalf("RepostGroups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1: %v", len(groups), groups)
	}
	if len(groups["abc"]) != 2 {
		t.Errorf("group abc has %d rows, want 2", len(groups["abc"]))
	}
	if _, ok := groups["kmless"]; ok {
		t.Error("a group without mileage must not be reported")
	}
}
