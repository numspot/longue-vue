package collector

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// pollCounter counts the polls: ServerVersion is the first call of each one.
type pollCounter struct {
	*fakeSource
	polls atomic.Int32
}

func (p *pollCounter) ServerVersion(ctx context.Context) (string, error) {
	p.polls.Add(1)
	return p.fakeSource.ServerVersion(ctx)
}

func newJitterCollector(src KubeSource, interval, startJitter time.Duration) *Collector {
	c := New(newFakeStore(), src, "prod", interval, time.Second, true, WithStartJitter(startJitter))
	// Deterministic draw: half of the upper bound.
	c.jitter = func(upper time.Duration) time.Duration { return upper / 2 }
	return c
}

func TestStartDelay(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		interval    time.Duration
		startJitter time.Duration
		want        time.Duration
	}{
		{name: "no jitter polls immediately", interval: 15 * time.Minute, startJitter: 0, want: 0},
		{name: "negative jitter polls immediately", interval: 15 * time.Minute, startJitter: -time.Minute, want: 0},
		{name: "drawn below the jitter", interval: 15 * time.Minute, startJitter: 10 * time.Minute, want: 5 * time.Minute},
		{name: "capped at the interval", interval: 2 * time.Minute, startJitter: time.Hour, want: time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newJitterCollector(&fakeSource{version: "v1.29.5"}, tt.interval, tt.startJitter)

			if got := c.startDelay(); got != tt.want {
				t.Errorf("startDelay()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestStartDelayDefaultDrawStaysBelowBound(t *testing.T) {
	t.Parallel()
	c := New(newFakeStore(), &fakeSource{version: "v1.29.5"}, "prod", 15*time.Minute, time.Second, true,
		WithStartJitter(10*time.Minute))

	for range 1000 {
		if got := c.startDelay(); got < 0 || got >= 10*time.Minute {
			t.Fatalf("startDelay()=%v, want within [0, 10m)", got)
		}
	}
}

func TestRunStartJitter(t *testing.T) {
	t.Parallel()

	t.Run("the first poll waits for the delay and the next ones keep its phase", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			src := &pollCounter{fakeSource: &fakeSource{version: "v1.29.5"}}
			c := newJitterCollector(src, 15*time.Minute, 10*time.Minute) // delay 5m
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() { done <- c.Run(ctx) }()

			time.Sleep(5*time.Minute - time.Second)
			synctest.Wait()
			if got := src.polls.Load(); got != 0 {
				t.Fatalf("polls before the delay=%d, want 0", got)
			}

			time.Sleep(2 * time.Second)
			synctest.Wait()
			if got := src.polls.Load(); got != 1 {
				t.Fatalf("polls after the delay=%d, want 1", got)
			}

			// Next poll one interval after the first one, not after the start.
			time.Sleep(15*time.Minute - 2*time.Second)
			synctest.Wait()
			if got := src.polls.Load(); got != 1 {
				t.Fatalf("polls before the second tick=%d, want 1", got)
			}
			time.Sleep(2 * time.Second)
			synctest.Wait()
			if got := src.polls.Load(); got != 2 {
				t.Fatalf("polls after the second tick=%d, want 2", got)
			}

			cancel()
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Errorf("Run()=%v, want context.Canceled", err)
			}
		})
	})

	t.Run("a stop during the delay returns without polling", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			src := &pollCounter{fakeSource: &fakeSource{version: "v1.29.5"}}
			c := newJitterCollector(src, 15*time.Minute, 10*time.Minute)
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() { done <- c.Run(ctx) }()

			time.Sleep(time.Minute)
			cancel()

			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Errorf("Run()=%v, want context.Canceled", err)
			}
			if got := src.polls.Load(); got != 0 {
				t.Errorf("polls=%d, want 0", got)
			}
		})
	})

	t.Run("without jitter the first poll is immediate", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			src := &pollCounter{fakeSource: &fakeSource{version: "v1.29.5"}}
			c := newJitterCollector(src, 15*time.Minute, 0)
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() { done <- c.Run(ctx) }()

			synctest.Wait()
			if got := src.polls.Load(); got != 1 {
				t.Fatalf("polls at start=%d, want 1", got)
			}

			cancel()
			<-done
		})
	})
}
