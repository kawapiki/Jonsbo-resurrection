package aisubscriptions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	data "github.com/kawapiki/Jonsbo-resurrection/pkg/aisubscriptions"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
)

func fixture(t *testing.T) (*Module, string, time.Time) {
	t.Helper()
	dir := t.TempDir()
	m, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	return m, dir, now
}
func apply(t *testing.T, m *Module, o data.Observation) {
	t.Helper()
	if err := m.Observe(context.Background(), o); err != nil {
		t.Fatal(err)
	}
}
func account(now time.Time, key string) data.Observation {
	return data.Observation{Kind: "account", Provider: data.OpenAI, AccountKey: key, AuthMode: "subscription", Connection: "connected", Source: "openai-app-server", ObservedAt: now}
}
func usage(now time.Time, key, session, request string, n int64) data.Observation {
	return data.Observation{Kind: "usage", Provider: data.OpenAI, AccountKey: key, SessionID: session, RequestID: request, Source: "codex-otel", ObservedAt: now, Tokens: &data.TokenUsage{Total: n}}
}
func TestRequestReplaySurvivesRestart(t *testing.T) {
	m, dir, now := fixture(t)
	apply(t, m, account(now, "a"))
	u := usage(now, "a", "s", "r1", 50)
	apply(t, m, u)
	apply(t, m, u)
	reopened, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	reopened.now = m.now
	apply(t, reopened, u)
	s := reopened.Snapshot()
	if s.Providers[0].WeeklyTokens == nil || *s.Providers[0].WeeklyTokens != 50 {
		t.Fatalf("weekly = %+v", s.Providers[0])
	}
	if *s.Sessions[0].ConsumedTokens != 50 {
		t.Fatal("session replay inflated")
	}
}
func TestAccountsAndUnattributedSessionsStayIsolated(t *testing.T) {
	m, _, now := fixture(t)
	apply(t, m, account(now, "a"))
	apply(t, m, usage(now, "a", "same", "r1", 50))
	apply(t, m, account(now, "b"))
	apply(t, m, usage(now, "b", "same", "r1", 80))
	apply(t, m, usage(now, "", "local", "r1", 200))
	s := m.Snapshot()
	if *s.Providers[0].WeeklyTokens != 80 || len(s.Sessions) != 2 {
		t.Fatalf("mixed accounts: %+v", s)
	}
	apply(t, m, account(now, "a"))
	if *m.Snapshot().Providers[0].WeeklyTokens != 50 {
		t.Fatal("account a lost isolated total")
	}
}
func TestContextCompactionAndMissingTokens(t *testing.T) {
	m, _, now := fixture(t)
	apply(t, m, account(now, "a"))
	n := int64(123)
	o := data.Observation{Kind: "context", Provider: data.OpenAI, AccountKey: "a", SessionID: "s", Source: "codex-hook", ObservedAt: now, Context: &data.ContextUsage{Used: &n}}
	apply(t, m, o)
	apply(t, m, o)
	o.Context = nil
	apply(t, m, o)
	s := m.Snapshot()
	if s.Providers[0].WeeklyTokens != nil || s.Sessions[0].ConsumedTokens != nil || s.Sessions[0].Context != nil {
		t.Fatalf("invented data: %+v", s)
	}
	s.Providers[0].Connection = "mutated"
	if m.Snapshot().Providers[0].Connection != "connected" {
		t.Fatal("snapshot aliases state")
	}
}
func TestBudapestWeekUsesLocalMondayAcrossDST(t *testing.T) {
	m, _, _ := fixture(t)
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	apply(t, m, account(now, "a"))
	apply(t, m, usage(time.Date(2026, 10, 25, 22, 59, 0, 0, time.UTC), "a", "s", "sun", 10))
	apply(t, m, usage(time.Date(2026, 10, 25, 23, 0, 0, 0, time.UTC), "a", "s", "mon", 20))
	if *m.Snapshot().Providers[0].WeeklyTokens != 20 {
		t.Fatal("week boundary used UTC or fixed DST offset")
	}
}
func TestAuthoritativeBucketsReplaceObservedTotals(t *testing.T) {
	m, _, now := fixture(t)
	apply(t, m, account(now, "a"))
	apply(t, m, usage(now, "a", "s", "r", 50))
	o := account(now, "a")
	o.DailyUsage = []data.DailyBucket{{Date: "2026-10-07", Tokens: 900}}
	o.Complete = true
	apply(t, m, o)
	apply(t, m, o)
	p := m.Snapshot().Providers[0]
	if *p.WeeklyTokens != 900 || p.UsageLabel != "Provider-day tokens" {
		t.Fatalf("double counted or invented timezone: %+v", p)
	}
}
func TestSilenceExpiresBrowserAndNativeEvidence(t *testing.T) {
	m, _, now := fixture(t)
	apply(t, m, account(now, "a"))
	apply(t, m, data.Observation{Kind: "session", Provider: data.OpenAI, SessionID: "web", Source: "browser", ObservedAt: now, Activity: "running"})
	m.now = func() time.Time { return now.Add(46 * time.Second) }
	s := m.Snapshot()
	if !s.Sessions[0].Stale || s.Sessions[0].Activity != "unknown" {
		t.Fatal("suspended browser remains live")
	}
	m.now = func() time.Time { return now.Add(181 * time.Second) }
	if !m.Snapshot().Providers[0].Stale {
		t.Fatal("account feed remains fresh")
	}
}
func TestInvalidInputAndCancelledWorkDoNotMutate(t *testing.T) {
	m, _, now := fixture(t)
	before := m.Snapshot()
	for _, o := range []data.Observation{{Kind: "usage", Provider: "other", Source: "browser", ObservedAt: now}, usage(now, "", "s", "r", -1), {Kind: "session", Provider: data.OpenAI, Source: "invented", ObservedAt: now}} {
		if err := m.Observe(context.Background(), o); !errors.Is(err, module.ErrInvalid) {
			t.Fatalf("invalid accepted: %v", err)
		}
	}
	if !reflect.DeepEqual(before, m.Snapshot()) {
		t.Fatal("invalid event mutated state")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Observe(ctx, account(now, "a")); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	payload := json.RawMessage(`{"kind":"account","provider":"openai","source":"openai-app-server","observed_at":"2026-10-07T12:00:00Z","secret":"x"}`)
	if err := m.HandleEvent(context.Background(), module.Event{Type: "observation", Payload: payload}); !errors.Is(err, module.ErrInvalid) {
		t.Fatal("unknown JSON field accepted")
	}
}
func TestSessionBoundRejectsConsumptionAndMarksPartial(t *testing.T) {
	m, _, now := fixture(t)
	apply(t, m, account(now, "a"))
	for i := 0; i < 128; i++ {
		o := usage(now, "a", time.Duration(i).String(), time.Duration(i).String(), 1)
		apply(t, m, o)
	}
	if err := m.Observe(context.Background(), usage(now, "a", "overflow", "overflow", 100)); err == nil {
		t.Fatal("overflow accepted")
	}
	s := m.Snapshot()
	if len(s.Sessions) != 128 || *s.Providers[0].WeeklyTokens != 128 || !s.Partial {
		t.Fatalf("bound inflated consumption: %+v", s.Providers[0])
	}
}
func TestRenderingOwnsFramesAndHonorsCancellation(t *testing.T) {
	m, _, now := fixture(t)
	for _, v := range m.Descriptor().Views {
		im, err := m.Render(context.Background(), v.ID)
		if err != nil {
			t.Fatal(err)
		}
		if im.Bounds() != image.Rect(0, 0, v.Width, v.Height) {
			t.Fatal("wrong native size")
		}
	}
	a := DrawWidget(m.Snapshot(), data.OpenAI, 300, 180, now, true).(*image.RGBA)
	b := DrawWidget(m.Snapshot(), data.OpenAI, 300, 180, now, true).(*image.RGBA)
	a.Pix[0] = 99
	if b.Pix[0] == 99 {
		t.Fatal("shared frame")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Render(ctx, "openai"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestCloseStopsPublishingAndRejectsObservations(t *testing.T) {
	m, _, now := fixture(t)
	published := make(chan struct{}, 2)
	finished := make(chan error, 1)
	go func() {
		finished <- m.Run(context.Background(), func(any) error { published <- struct{}{}; return nil })
	}()
	<-published
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("closed module kept publishing")
	}
	if err := m.Observe(context.Background(), account(now, "a")); !errors.Is(err, module.ErrUnavailable) {
		t.Fatal("closed module accepted event")
	}
}
func TestDedupBoundPreservesReplayProtection(t *testing.T) {
	m, _, now := fixture(t)
	m.dir = ""
	apply(t, m, account(now, "a"))
	apply(t, m, usage(now, "a", "s", "real", 50))
	for i := 1; i < 200000; i++ {
		m.state.Dedup[fmt.Sprint("synthetic", i)] = now
	}
	if err := m.Observe(context.Background(), usage(now, "a", "s", "new", 100)); err == nil {
		t.Fatal("dedup capacity accepted new consumption")
	}
	apply(t, m, usage(now, "a", "s", "real", 50))
	s := m.Snapshot()
	if !s.Partial || *s.Providers[0].WeeklyTokens != 50 {
		t.Fatal("dedup limit lost replay safety")
	}
}
func TestPersistenceFailureDoesNotPublishUncommittedUsage(t *testing.T) {
	m, dir, now := fixture(t)
	apply(t, m, account(now, "a"))
	before := m.Snapshot()
	m.dir = filepath.Join(dir, "missing")
	if err := m.Observe(context.Background(), usage(now, "a", "s", "r", 50)); err == nil {
		t.Fatal("failed storage accepted observation")
	}
	if !reflect.DeepEqual(before, m.Snapshot()) {
		t.Fatal("uncommitted state leaked")
	}
}
func TestRejectsUnknownStorageSchema(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "state-v1.json"), []byte(`{"version":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(dir); !errors.Is(err, module.ErrInvalid) {
		t.Fatal("unknown storage schema accepted", err)
	}
}
func TestProviderCategoryConventionsAndUnknownAttribution(t *testing.T) {
	m, _, now := fixture(t)
	apply(t, m, account(now, "a"))
	o := usage(now, "a", "s", "r", 0)
	o.Tokens = &data.TokenUsage{Input: 10, Output: 5, CachedInput: 7, ReasoningOutput: 3}
	apply(t, m, o)
	if *m.Snapshot().Providers[0].WeeklyTokens != 15 {
		t.Fatal("OpenAI cached/reasoning subcategories counted twice")
	}
	c := account(now, "b")
	c.Provider = data.Claude
	c.Source = "claude-auth"
	apply(t, m, c)
	o.Provider = data.Claude
	o.AccountKey = "b"
	o.Source = "claude-otel"
	o.Tokens.CacheWrite = 2
	apply(t, m, o)
	if *m.Snapshot().Providers[1].WeeklyTokens != 24 {
		t.Fatal("Claude additive cache categories omitted")
	}
	invalid := usage(now, "unverified", "s", "r", 10)
	if err := m.Observe(context.Background(), invalid); !errors.Is(err, module.ErrInvalid) {
		t.Fatal("unverified attribution accepted")
	}
}
func TestDisconnectClearsDisplayButKeepsRequestTombstones(t *testing.T) {
	m, _, now := fixture(t)
	apply(t, m, account(now, "a"))
	u := usage(now, "a", "s", "r", 50)
	apply(t, m, u)
	apply(t, m, data.Observation{Kind: "disconnect", Provider: data.OpenAI, Source: "openai-app-server", ObservedAt: now})
	s := m.Snapshot()
	if s.Providers[0].WeeklyTokens != nil || len(s.Sessions) != 0 || s.Providers[0].Connection != "disconnected" {
		t.Fatal("disconnect retained display cache")
	}
	apply(t, m, account(now, "a"))
	apply(t, m, u)
	if m.Snapshot().Providers[0].WeeklyTokens != nil {
		t.Fatal("reconnect counted replay")
	}
}
func TestObservationTooOldCannotReenterPrunedHistory(t *testing.T) {
	m, _, now := fixture(t)
	apply(t, m, account(now, "a"))
	if err := m.Observe(context.Background(), usage(now.AddDate(0, 0, -36), "a", "s", "old", 50)); !errors.Is(err, module.ErrInvalid) {
		t.Fatal("expired replay accepted")
	}
}

func TestDelayedOldAccountStatusDoesNotSwitchCurrentAccount(t *testing.T) {
	m, _, now := fixture(t)
	apply(t, m, account(now.Add(-time.Minute), "a"))
	apply(t, m, account(now, "b"))
	apply(t, m, account(now.Add(-30*time.Second), "a"))
	if m.Snapshot().Providers[0].AccountKey != "b" {
		t.Fatal("delayed status switched back to old account")
	}
}
func TestDailyAggregatesStayWithin35CalendarDays(t *testing.T) {
	m, _, now := fixture(t)
	m.dir = ""
	apply(t, m, account(now, "a"))
	for i := 0; i <= 35; i++ {
		at := now.AddDate(0, 0, -i)
		apply(t, m, usage(at, "a", "s", fmt.Sprint(i), 1))
	}
	if n := len(m.state.Daily[key(data.OpenAI, "a")]); n > 35 {
		t.Fatalf("kept %d daily aggregates", n)
	}
}
func TestAnimationPreferenceMakesProviderFrameStable(t *testing.T) {
	m, _, now := fixture(t)
	apply(t, m, account(now, "a"))
	s := m.Snapshot()
	a := DrawWidget(s, data.OpenAI, 640, 180, now, false).(*image.RGBA)
	b := DrawWidget(s, data.OpenAI, 640, 180, now.Add(300*time.Millisecond), false).(*image.RGBA)
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("animation off still changed frame")
	}
	animatedA := DrawWidget(s, data.OpenAI, 640, 180, now, true).(*image.RGBA)
	animatedB := DrawWidget(s, data.OpenAI, 640, 180, now.Add(300*time.Millisecond), true).(*image.RGBA)
	if bytes.Equal(animatedA.Pix, animatedB.Pix) {
		t.Fatal("connected animation never moves")
	}
}
func TestOfficialMarkScalingPreservesAspectAndColors(t *testing.T) {
	logo := image.NewRGBA(image.Rect(0, 0, 40, 20))
	fill(logo, logo.Bounds(), color.RGBA{217, 151, 117, 255})
	out := image.NewRGBA(image.Rect(0, 0, 40, 40))
	paintLogoFit(out, logo, image.Rect(10, 10, 30, 30))
	if out.RGBAAt(10, 15) != (color.RGBA{217, 151, 117, 255}) || out.RGBAAt(29, 24) != (color.RGBA{217, 151, 117, 255}) || out.RGBAAt(10, 14).A != 0 || out.RGBAAt(10, 25).A != 0 {
		t.Fatal("mark stretched or recolored")
	}
}
func TestAuthenticationFailurePreservesLastKnownAccountData(t *testing.T) {
	m, _, now := fixture(t)
	a := account(now, "a")
	a.Plan = "plus"
	apply(t, m, a)
	apply(t, m, usage(now, "a", "s", "r", 50))
	failure := data.Observation{Kind: "account", Provider: data.OpenAI, Source: "openai-app-server", Connection: "reauthentication", ObservedAt: now.Add(time.Second)}
	apply(t, m, failure)
	p := m.Snapshot().Providers[0]
	if p.AccountKey != "a" || p.Plan != "plus" || p.WeeklyTokens == nil || *p.WeeklyTokens != 50 || p.Connection != "reauthentication" || !p.Stale {
		t.Fatalf("failed auth discarded last-known state: %+v", p)
	}
}
func browserSession(now time.Time, id, activity string) data.Observation {
	return data.Observation{Kind: "session", Provider: data.OpenAI, Source: "browser", SessionID: id, Activity: activity, ObservedAt: now}
}
func TestClosedBrowserSessionsReleaseCapacity(t *testing.T) {
	m, _, now := fixture(t)
	m.dir = ""
	for i := 0; i < 128; i++ {
		apply(t, m, browserSession(now, fmt.Sprint(i), "running"))
	}
	apply(t, m, browserSession(now, "0", "completed"))
	apply(t, m, browserSession(now, "next", "running"))
	s := m.Snapshot()
	if len(s.Sessions) != 128 || s.Partial {
		t.Fatalf("closed tab did not release capacity: %d %v", len(s.Sessions), s.Partial)
	}
	for _, v := range s.Sessions {
		if v.ID == "0" {
			t.Fatal("closed browser record retained")
		}
	}
}
func TestStaleSessionsCanBeReclaimedWithoutLosingUsageDedup(t *testing.T) {
	m, _, now := fixture(t)
	m.dir = ""
	apply(t, m, account(now, "a"))
	apply(t, m, usage(now, "a", "native", "r", 50))
	for i := 0; i < 127; i++ {
		apply(t, m, browserSession(now, fmt.Sprint(i), "running"))
	}
	now = now.Add(181 * time.Second)
	m.now = func() time.Time { return now }
	for i := 0; i < 128; i++ {
		apply(t, m, browserSession(now, "new"+fmt.Sprint(i), "running"))
	}
	if len(m.Snapshot().Sessions) != 128 || m.Snapshot().Partial {
		t.Fatal("stale sessions blocked new activity")
	}
	// A session-less retransmission must remain deduplicated after its display
	// record was reclaimed; weekly aggregates also remain independent.
	apply(t, m, usage(now, "a", "", "r", 50))
	if n := m.Snapshot().Providers[0].WeeklyTokens; n == nil || *n != 50 {
		t.Fatal("session eviction dropped durable replay protection")
	}
}
func TestActivityHeartbeatsDoNotRefreshOldContext(t *testing.T) {
	m, _, now := fixture(t)
	n := int64(123)
	apply(t, m, data.Observation{Kind: "context", Provider: data.Claude, SessionID: "s", Source: "claude-statusline", ObservedAt: now, Context: &data.ContextUsage{Used: &n}})
	current := now.Add(181 * time.Second)
	m.now = func() time.Time { return current }
	apply(t, m, data.Observation{Kind: "session", Provider: data.Claude, SessionID: "s", Source: "claude-hook", Activity: "running", ObservedAt: current})
	s := m.Snapshot().Sessions[0]
	if s.Context != nil || s.Stale || s.Activity != "running" {
		t.Fatalf("activity refreshed obsolete context: %+v", s)
	}
}
func TestContextTimestampSurvivesRestartAndIndependentHookOrdering(t *testing.T) {
	m, dir, now := fixture(t)
	n := int64(123)
	apply(t, m, data.Observation{Kind: "session", Provider: data.Claude, SessionID: "s", Source: "claude-hook", Activity: "running", ObservedAt: now.Add(10 * time.Second)})
	apply(t, m, data.Observation{Kind: "context", Provider: data.Claude, SessionID: "s", Source: "claude-statusline", ObservedAt: now.Add(5 * time.Second), Context: &data.ContextUsage{Used: &n}})
	if m.Snapshot().Sessions[0].Context == nil {
		t.Fatal("late context was discarded behind activity timestamp")
	}
	reopened, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	current := now.Add(190 * time.Second)
	reopened.now = func() time.Time { return current }
	apply(t, reopened, data.Observation{Kind: "session", Provider: data.Claude, SessionID: "s", Source: "claude-hook", Activity: "running", ObservedAt: current})
	s := reopened.Snapshot().Sessions[0]
	if s.Context != nil || s.Activity != "running" || s.Stale {
		t.Fatal("restart lost independent context freshness")
	}
}
