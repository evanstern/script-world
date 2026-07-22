package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/evanstern/promptworld/internal/llm"
)

// fakeSubmitter answers every call after a fixed simulated duration.
type fakeSubmitter struct {
	millisPerPoint int64
	fail           bool
	calls          int
}

func (f *fakeSubmitter) Submit(_ context.Context, req llm.Request) (llm.Response, error) {
	f.calls++
	if f.fail {
		return llm.Response{}, errors.New("model down")
	}
	points := int64(1)
	if req.Kind == llm.KindPlanner {
		points = 3
	}
	return llm.Response{Text: "ok", Millis: f.millisPerPoint * points}, nil
}

func TestCalibrateTierMedian(t *testing.T) {
	f := &fakeSubmitter{millisPerPoint: 17000}
	tp, err := calibrateTier(f, llm.TierLocal, 3)
	if err != nil {
		t.Fatal(err)
	}
	if f.calls != 3 { // 1 local shape (planner-3pt) × 3 samples (musing retired, spec 017)
		t.Errorf("calls = %d, want 3", f.calls)
	}
	if tp.SecondsPerPoint < 16.9 || tp.SecondsPerPoint > 17.1 {
		t.Errorf("seconds_per_point = %g, want ~17", tp.SecondsPerPoint)
	}
	if len(tp.Samples) != 1 || len(tp.Samples[0].WallMs) != 3 {
		t.Errorf("audit samples incomplete: %+v", tp.Samples)
	}
}

func TestCalibrateTierAllFailed(t *testing.T) {
	f := &fakeSubmitter{fail: true}
	if _, err := calibrateTier(f, llm.TierLocal, 2); err == nil {
		t.Fatal("unusable tier must error (profile not written)")
	}
}

func TestHorizonSummary(t *testing.T) {
	// At 17 s/pt the contract's sanity table: planner suppressed above 16x,
	// conversation/meeting OK at 32x.
	s := horizonSummary(17.0)
	if !strings.Contains(s, "planner suppressed above 16x") {
		t.Errorf("summary = %q", s)
	}
	if !strings.Contains(s, "conversation OK at 32x") {
		t.Errorf("summary = %q", s)
	}
}

func TestFakeSubmitterTiming(t *testing.T) {
	// Guard the fake itself: calibrate never sleeps for real in tests.
	start := time.Now()
	f := &fakeSubmitter{millisPerPoint: 17000}
	if _, err := calibrateTier(f, llm.TierLocal, 5); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("calibrateTier did real waiting in a unit test")
	}
}
