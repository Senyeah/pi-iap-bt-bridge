package main

import (
	"testing"
	"time"
)

func TestParseFrameNumber(t *testing.T) {
	// From the Pi 5 while the stereo was stuck (10/10/2026).
	s := "DCFG=0x00040000, DCTL=0x00000000, DSTS=0x00066902\nDIEPMSK=0x0000000f, DOEPMASK=0x0000000f\n"
	if fn, ok := parseFrameNumber(s); !ok || fn != 0x669 {
		t.Errorf("frame %#x %v, want 0x669", fn, ok)
	}
	for _, s := range []string{"", "DCFG=0x00040000", "DSTS=0x0006", "DSTS=0xzzzzzzzz"} {
		if _, ok := parseFrameNumber(s); ok {
			t.Errorf("%q gives a frame number", s)
		}
	}
}

// samples feeds the watch every 0.5 s, from start, and returns the times of the replugs.
func samples(w *stuckWatch, start time.Time, n int, state string, frames bool) (replugs []time.Duration, end time.Time) {
	fn := 0
	for i := range n {
		now := start.Add(time.Duration(i) * 500 * time.Millisecond)
		if frames {
			fn = (fn + 500) & 0x3fff
		}
		if w.sample(now, state, fn) {
			replugs = append(replugs, now.Sub(start))
		}
		end = now
	}
	return replugs, end.Add(500 * time.Millisecond)
}

func TestStuckStereoGetsAReplug(t *testing.T) {
	w := newStuckWatch(3 * time.Second)
	got, end := samples(w, time.Unix(0, 0), 40, "not attached", true)
	want := []time.Duration{3500 * time.Millisecond, 7000 * time.Millisecond, 10500 * time.Millisecond}
	if len(got) != len(want) {
		t.Fatalf("replugs at %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("replug %d at %v, want %v", i+1, got[i], want[i])
		}
	}
	// A configuration starts the count again.
	samples(w, end, 2, "configured", true)
	if got, _ := samples(w, end.Add(time.Second), 10, "not attached", true); len(got) != 1 {
		t.Errorf("after a configuration: %d replugs, want 1", len(got))
	}
}

func TestNoReplug(t *testing.T) {
	for _, c := range []struct {
		name   string
		after  time.Duration
		state  string
		frames bool
	}{
		{"no host", 3 * time.Second, "not attached", false},
		{"suspended", 3 * time.Second, "suspended", false},
		{"configured", 3 * time.Second, "configured", true},
		{"off", 0, "not attached", true},
	} {
		if got, _ := samples(newStuckWatch(c.after), time.Unix(0, 0), 40, c.state, c.frames); len(got) != 0 {
			t.Errorf("%s: replugs at %v", c.name, got)
		}
	}
}

func TestNormalEnumerationGetsNoReplug(t *testing.T) {
	w := newStuckWatch(3 * time.Second)
	_, end := samples(w, time.Unix(0, 0), 2, "not attached", true) // reset to configuration: 0.8 s in the car
	samples(w, end, 20, "configured", true)
	if w.tries != 0 {
		t.Errorf("%d replugs", w.tries)
	}
}
