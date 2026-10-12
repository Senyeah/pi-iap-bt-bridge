package main

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// The car stereo fails when the Pi is on the bus before its USB host starts: it resets the bus once, then sends
// frames (SOF) but no request. A new connection after that works (CLAUDE.md, "Quirks").

const (
	replugGap  = 500 * time.Millisecond
	maxReplugs = 3
)

// frameNumber is the frame number of the last SOF from the host, from the dwc2 debug file.
func frameNumber(udc string) (int, bool) {
	b, err := os.ReadFile("/sys/kernel/debug/usb/" + udc + "/state")
	if err != nil {
		return 0, false
	}
	return parseFrameNumber(string(b))
}

func parseFrameNumber(s string) (int, bool) {
	i := strings.Index(s, "DSTS=0x")
	if i < 0 || len(s) < i+15 {
		return 0, false
	}
	v, err := strconv.ParseUint(s[i+7:i+15], 16, 32)
	if err != nil {
		return 0, false
	}
	return int(v>>8) & 0x3fff, true
}

// stuckWatch finds a host that sends frames but does not configure the device.
type stuckWatch struct {
	after time.Duration // 0: off
	frame int           // -1: no sample yet
	since time.Time     // start of the frames without a configuration
	tries int
}

func newStuckWatch(after time.Duration) *stuckWatch { return &stuckWatch{after: after, frame: -1} }

// sample takes the USB state and the frame number every 0.5 s. It says when to connect again.
func (w *stuckWatch) sample(now time.Time, state string, frame int) bool {
	sof := w.frame >= 0 && frame != w.frame
	w.frame = frame
	switch {
	case w.after <= 0:
		return false
	case state == "configured":
		w.tries, w.since = 0, time.Time{}
		return false
	case state == "suspended" || !sof: // the stereo may suspend the bus on another source
		w.since = time.Time{}
		return false
	case w.since.IsZero():
		w.since = now
		return false
	case now.Sub(w.since) < w.after || w.tries >= maxReplugs:
		return false
	}
	w.tries++
	w.since = time.Time{}
	return true
}

// replug disconnects the device from the bus and connects it again, as a cable unplug does.
func replug(udc string) {
	p := "/sys/class/udc/" + udc + "/soft_connect"
	if err := os.WriteFile(p, []byte("disconnect"), 0); err != nil {
		slog.Warn("cannot disconnect from the bus", "err", err)
		return
	}
	time.Sleep(replugGap)
	if err := os.WriteFile(p, []byte("connect"), 0); err != nil {
		slog.Warn("cannot connect to the bus", "err", err)
	}
}
