package capture

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestCaptureSkipsWithoutEnv pins the gate: no ENTIEVE_CAPTURE, no loop.
func TestCaptureSkipsWithoutEnv(t *testing.T) {
	t.Setenv("ENTIEVE_CAPTURE", "")
	var sub *testing.T
	t.Run("capture", func(tt *testing.T) {
		sub = tt
		_ = (&Capture{Name: "ungated", Frames: 1}).run(tt)
	})
	if sub == nil || !sub.Skipped() {
		t.Fatal("Capture.run drove a loop without ENTIEVE_CAPTURE")
	}
}

// TestCaptureSkipsSecondLoopInProcess pins the one-loop guard: a second
// capture in the process skips instead of panicking inside RunGame.
func TestCaptureSkipsSecondLoopInProcess(t *testing.T) {
	old := ebitenLoopRan
	ebitenLoopRan = true
	t.Cleanup(func() { ebitenLoopRan = old })
	t.Setenv("ENTIEVE_CAPTURE", "1")
	var sub *testing.T
	t.Run("capture", func(tt *testing.T) {
		sub = tt
		_ = (&Capture{Name: "second loop", Frames: 1}).run(tt)
	})
	if sub == nil || !sub.Skipped() {
		t.Fatal("a second Capture.run in one process must skip: one ebiten loop per test process")
	}
}

// TestOutResolvesCaptureDir pins the output dir: $ENTIEVE_CAPTURE_DIR, else
// /tmp/opencode.
func TestOutResolvesCaptureDir(t *testing.T) {
	t.Setenv("ENTIEVE_CAPTURE_DIR", "")
	if got, want := Out("x.png"), filepath.Join("/tmp/opencode", "x.png"); got != want {
		t.Errorf("Out with no dir = %q, want %q", got, want)
	}
	t.Setenv("ENTIEVE_CAPTURE_DIR", filepath.Join(t.TempDir(), "shots"))
	if got, want := Out("x.png"), filepath.Join(os.Getenv("ENTIEVE_CAPTURE_DIR"), "x.png"); got != want {
		t.Errorf("Out = %q, want %q", got, want)
	}
}

// TestCaptureWritesEvidenceOnVerifyFailure is the driver's own loop check
// (gated: it needs a display): a failing Verify still writes the PNG — the
// evidence lands — and the failure reaches the caller.
func TestCaptureWritesEvidenceOnVerifyFailure(t *testing.T) {
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	c := &Capture{
		Name:   "Entieve capture driver self-check",
		Frames: 1,
		Draw:   func(int, *ebiten.Image) error { return nil },
		Verify: func(*ebiten.Image) error { return errors.New("intended verify failure") },
		Out:    filepath.Join(t.TempDir(), "evidence.png"),
	}
	err := c.run(t)
	if err == nil || !strings.Contains(err.Error(), "intended verify failure") {
		t.Fatalf("verify failure = %v, want it reported", err)
	}
	if _, statErr := os.Stat(c.Out); statErr != nil {
		t.Fatalf("no PNG on a verify failure: %v — the evidence must land", statErr)
	}
}
