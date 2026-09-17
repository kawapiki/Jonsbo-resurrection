//go:build windows

package hardware

import (
	"context"
	"errors"
	"github.com/kawapiki/Jonsbo-resurrection/internal/telemetry"
	"github.com/kawapiki/Jonsbo-resurrection/pkg/module"
	"testing"
	"time"
)

type fakeCollector struct{ samples, closes int }

func (f *fakeCollector) Sample() telemetry.Snapshot {
	f.samples++
	return telemetry.Snapshot{Time: time.Unix(int64(f.samples), 0), GPUs: []telemetry.GPU{}}
}
func (f *fakeCollector) Close() { f.closes++ }

func TestFreshSampleAndClose(t *testing.T) {
	m, err := New(500 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeCollector{}
	opened := 0
	m.open = func() collector { opened++; return f }
	if opened != 0 {
		t.Fatal("collector opened before Run")
	}
	if _, err = m.Render(context.Background(), "summary"); !errors.Is(err, module.ErrUnavailable) {
		t.Fatalf("unprimed frame: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	count := 0
	err = m.Run(ctx, func(v any) error {
		count++
		s := v.(telemetry.Snapshot)
		if s.Time != time.Unix(2, 0) || s.CPU.UsagePercent != nil {
			t.Errorf("published stale or fabricated metrics: %+v", s)
		}
		if time.Since(start) < 500*time.Millisecond {
			t.Error("did not wait for counter interval")
		}
		for _, view := range m.Descriptor().Views {
			frame, e := m.Render(ctx, view.ID)
			if e != nil {
				t.Error(e)
				continue
			}
			if frame.Bounds().Dx() != view.Width || frame.Bounds().Dy() != view.Height {
				t.Error("wrong frame dimensions")
			}
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || count != 1 || opened != 1 || f.closes != 1 {
		t.Fatalf("lifecycle: err=%v count=%d opened=%d closes=%d", err, count, opened, f.closes)
	}
	if _, err = m.Render(context.Background(), "summary"); !errors.Is(err, module.ErrUnavailable) {
		t.Fatalf("stopped module still rendered: %v", err)
	}
}

type blockingCollector struct {
	fakeCollector
	entered, release chan struct{}
}

func (f *blockingCollector) Sample() telemetry.Snapshot {
	s := f.fakeCollector.Sample()
	if f.samples == 3 {
		close(f.entered)
		<-f.release
	}
	return s
}

func TestBlockedSampleMakesOldFramesUnavailable(t *testing.T) {
	m, _ := New(500 * time.Millisecond)
	f := &blockingCollector{entered: make(chan struct{}), release: make(chan struct{})}
	m.open = func() collector { return f }
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.Run(ctx, func(any) error { return nil }) }()
	defer func() {
		cancel()
		close(f.release)
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) || f.closes != 1 {
				t.Errorf("blocked collector cleanup: err=%v closes=%d", err, f.closes)
			}
		case <-time.After(time.Second):
			t.Error("Run did not exit after native sample returned")
		}
	}()
	select {
	case <-f.entered:
	case <-ctx.Done():
		t.Fatal("collector did not enter the blocking sample")
	}
	if _, err := m.Render(ctx, "cpu"); err != nil {
		t.Fatalf("recent completed sample should render: %v", err)
	}
	m.mu.Lock()
	m.lastSample = time.Now().Add(-4 * time.Second)
	m.mu.Unlock()
	if _, err := m.Render(ctx, "cpu"); !errors.Is(err, module.ErrUnavailable) {
		t.Fatalf("blocked collector kept old metrics live: %v", err)
	}
}

func TestFreshnessLimitScalesWithCadence(t *testing.T) {
	for _, tc := range []struct {
		interval, age time.Duration
		unavailable   bool
	}{
		{time.Second, 2 * time.Second, false},
		{time.Second, 4 * time.Second, true},
		{2 * time.Second, 5 * time.Second, false},
		{2 * time.Second, 7 * time.Second, true},
	} {
		m, _ := New(tc.interval)
		m.ready = true
		m.lastSample = time.Now().Add(-tc.age)
		_, err := m.Render(context.Background(), "cpu")
		if errors.Is(err, module.ErrUnavailable) != tc.unavailable {
			t.Errorf("interval=%v age=%v unavailable=%v: %v", tc.interval, tc.age, tc.unavailable, err)
		}
	}
}

func TestCancellationAndPublishFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "canceled", true: "publish failure"}[fail], func(t *testing.T) {
			m, _ := New(500 * time.Millisecond)
			f := &fakeCollector{}
			m.open = func() collector { return f }
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sentinel := errors.New("publish failed")
			if !fail {
				cancel()
			}
			err := m.Run(ctx, func(any) error { return sentinel })
			if fail {
				if !errors.Is(err, sentinel) || f.closes != 1 {
					t.Fatalf("%v close=%d", err, f.closes)
				}
			} else if !errors.Is(err, context.Canceled) || f.samples != 0 {
				t.Fatalf("%v samples=%d", err, f.samples)
			}
		})
	}
	if _, err := New(499 * time.Millisecond); !errors.Is(err, module.ErrInvalid) {
		t.Fatal("accepted aggressive cadence")
	}
}
