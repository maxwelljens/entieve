// Package capture drives the gated pixel captures: one hidden ebiten loop
// per process, the ENTIEVE_CAPTURE skip, the output dir, the PNG save and
// the one-loop guard. It is a leaf package (ebiten only) so the render probe
// can use it too.
package capture

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// ebitenLoopRan records whether this process already drove a window
// loop, so running the whole capture set skips the extras instead of
// panicking.
var ebitenLoopRan bool

// SkipIfLoopRan skips the test when this process already ran a loop.
func SkipIfLoopRan(t *testing.T) {
	t.Helper()
	if ebitenLoopRan {
		t.Skip("one ebiten game loop per test process: run this capture on its own")
	}
}

// RunCaptureLoop drives one capture game through ebiten, records that the
// loop ran and skips when the window system is unavailable.
func RunCaptureLoop(t *testing.T, title string, g ebiten.Game) {
	t.Helper()
	ebiten.SetWindowTitle(title)
	ebiten.SetWindowSize(1280, 720)
	// Run the loop without showing the window: captures render offscreen
	// (SetWindowVisible before RunGame — the desktop stays clear).
	ebiten.SetWindowVisible(false)
	err := ebiten.RunGame(g)
	ebitenLoopRan = true
	if err != nil {
		t.Skipf("cannot run capture game loop: %v", err)
	}
}

// maxCaptureFrames bounds a staged capture that never calls Stop, so a bug in
// a scenario fails the test instead of running the loop forever.
const maxCaptureFrames = 1000

// Capture is one gated pixel capture: the scenario builds its game and renders
// the frames; the driver owns the envelope every capture test used to
// re-implement — the ENTIEVE_CAPTURE skip, the output dir, the
// one-loop-per-process guard, the hidden ebiten loop, the PNG save and the
// failure report.
//
// The loop draws Frames frames (Update and Draw keep ebiten's own cadence, so
// their counters are independent; both start at 1), then hands the last frame
// to Verify and saves it to Out. A staged capture — a frame saved mid-loop, or
// a second file — saves from its own hooks, leaves Frames 0 and Out empty, and
// ends the loop with Stop.
type Capture struct {
	Name   string                               // window title, and the label in failure messages
	Frames int                                  // frames to draw before Verify; 0 = until Stop
	Size   image.Point                          // frame size; zero = 1280×720
	Update func(n int) error                    // one game tick (nil = none)
	Draw   func(n int, img *ebiten.Image) error // frame n, drawn into img
	Verify func(img *ebiten.Image) error        // the last frame, inside the loop
	Out    string                               // PNG for the last frame; empty = the scenario saves its own

	stopped bool
}

// Run drives the capture and fails the test with anything that went wrong.
func (c *Capture) Run(t *testing.T) {
	t.Helper()
	if err := c.run(t); err != nil {
		t.Fatal(err)
	}
}

// run is Run without the fatal: the driver's own self-check inspects the error
// instead of failing on it.
func (c *Capture) run(t *testing.T) error {
	t.Helper()
	if os.Getenv("ENTIEVE_CAPTURE") == "" {
		t.Skip("set ENTIEVE_CAPTURE=1 to render a capture frame")
	}
	SkipIfLoopRan(t)
	size := c.Size
	if size == (image.Point{}) {
		size = image.Pt(1280, 720)
	}
	g := &captureGame{c: c, size: size}
	// RunCaptureLoop owns the hidden window, the loop and the loop-ran marker.
	RunCaptureLoop(t, c.Name, g)
	if g.last == nil {
		return fmt.Errorf("capture %s never rendered", c.Name)
	}
	if g.saved {
		t.Logf("capture written to %s", c.Out)
	}
	return g.err
}

// Stop ends the loop after the frame being drawn: a staged capture calls it
// once its own last shot is on disk.
func (c *Capture) Stop() { c.stopped = true }

// Save writes img as a PNG at path.
func Save(path string, img *ebiten.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Out resolves a capture file name inside $ENTIEVE_CAPTURE_DIR (default
// /tmp/opencode).
func Out(name string) string {
	dir := os.Getenv("ENTIEVE_CAPTURE_DIR")
	if dir == "" {
		dir = "/tmp/opencode"
	}
	return filepath.Join(dir, name)
}

// captureGame is the ebiten.Game a Capture drives.
type captureGame struct {
	c       *Capture
	size    image.Point
	updates int
	draws   int
	last    *ebiten.Image
	saved   bool
	done    bool
	err     error
}

func (g *captureGame) Layout(w, h int) (int, int) { return g.size.X, g.size.Y }

func (g *captureGame) Update() error {
	if g.done {
		return ebiten.Termination
	}
	g.updates++
	if g.c.Update != nil {
		if err := g.c.Update(g.updates); err != nil {
			g.err, g.done = err, true
			return ebiten.Termination
		}
	}
	if !g.c.stopped && (g.c.Frames == 0 || g.draws < g.c.Frames) {
		return nil
	}
	if g.last == nil {
		return nil // ebiten can tick several Updates before the first Draw
	}
	return g.finish()
}

func (g *captureGame) Draw(screen *ebiten.Image) {
	if g.done {
		return
	}
	if g.draws >= maxCaptureFrames {
		g.err = fmt.Errorf("capture %s did not finish in %d frames (a staged capture must call Stop)",
			g.c.Name, maxCaptureFrames)
		g.done = true
		return
	}
	img := ebiten.NewImage(g.size.X, g.size.Y)
	if g.c.Draw != nil {
		if err := g.c.Draw(g.draws+1, img); err != nil {
			g.err, g.done = err, true
			return
		}
	}
	screen.DrawImage(img, nil)
	g.last = img
	g.draws++
}

// finish verifies the last frame and writes the evidence: a failing capture
// still leaves its PNG on disk.
func (g *captureGame) finish() error {
	g.done = true
	if g.c.Verify != nil {
		g.err = g.c.Verify(g.last)
	}
	if g.c.Out != "" {
		if err := Save(g.c.Out, g.last); err != nil {
			if g.err == nil {
				g.err = err
			}
		} else {
			g.saved = true
		}
	}
	return ebiten.Termination
}
