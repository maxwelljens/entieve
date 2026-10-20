package screens

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/input"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// cursorTrace records the OS cursor writes of one region's frames. The
// recorder wraps the repo's only cursor seam, so every write this code
// makes is a call; a write made behind it (ebitenui's input.AfterDraw
// resets the shape from its own name cache) shows up as a prev value the
// recorder did not write. differing counts those foreign writes too: it
// is the number of times the OS cursor CHANGED shape in the frame, which
// is what a player sees as flicker.
type cursorTrace struct {
	last      ebiten.CursorShapeType // shape the recorder last left on the OS
	calls     int
	differing int
	log       []string
}

func (tr *cursorTrace) reset() {
	tr.calls, tr.differing, tr.log = 0, 0, nil
	tr.last = ebiten.CursorShape()
}

// record is the cursor seam: it samples the OS value before and after the
// write, so a writer that ran between two of our calls is visible.
func (tr *cursorTrace) record(s ebiten.CursorShapeType) {
	prev := ebiten.CursorShape()
	if prev != tr.last {
		tr.differing++ // someone other than the recorder wrote since our last call
		tr.log = append(tr.log, "  foreign write left "+shapeName(prev))
	}
	ebiten.SetCursorShape(s)
	tr.calls++
	if s != prev {
		tr.differing++
	}
	tr.log = append(tr.log, "  seam write "+shapeName(prev)+" -> "+shapeName(s))
	tr.last = s
}

func shapeName(s ebiten.CursorShapeType) string {
	switch s {
	case ebiten.CursorShapeDefault:
		return "Default"
	case ebiten.CursorShapeEWResize:
		return "EWResize"
	case ebiten.CursorShapeNSResize:
		return "NSResize"
	case ebiten.CursorShapeNESWResize:
		return "NESWResize"
	case ebiten.CursorShapeNWSEResize:
		return "NWSEResize"
	}
	return "other"
}

// TestCursorSingleWriterPerFrame pins the cursor invariant: with a
// stationary pointer over any region — a vertical seam, a horizontal
// seam, a window edge grip or plain content — a frame leaves the OS
// cursor in exactly one shape, changes it at most once, and presents the
// same shape on every frame. The kit computes the shape after all
// ebitenui input processing and writes it once (widgets.ApplyCursor);
// ebitenui's own cursor management is off, so nothing can reset the
// shape behind the kit's back (the regression this pins: two writers
// alternating arrow/resize every frame).
func TestCursorSingleWriterPerFrame(t *testing.T) {
	cases := []struct {
		name string
		// build mounts the case's panel (if any) on the kit and returns
		// its per-frame Sync, and the cursor point plus the shape the
		// region must present.
		build func(t *testing.T, k *Kit) (sync func(), at image.Point, want ebiten.CursorShapeType)
	}{
		{"vertical seam", func(t *testing.T, k *Kit) (func(), image.Point, ebiten.CursorShapeType) {
			env, _ := newTestEnv(t)
			inv := NewInventory(env)
			k.AddWindow(inv.Win())
			pumpKit(t, k, 1280, 720)
			seam := splitSeam(inv.split, 0)
			return inv.Sync, image.Pt(seam.Min.X+2, seam.Min.Y), ebiten.CursorShapeNSResize
		}},
		{"horizontal seam", func(t *testing.T, k *Kit) (func(), image.Point, ebiten.CursorShapeType) {
			env, _ := newTestEnv(t)
			m := NewMarket(env)
			k.AddWindow(m.Win())
			pumpKit(t, k, 1280, 720)
			seam := splitSeam(m.split, 0)
			return m.Sync, image.Pt(seam.Min.X, seam.Min.Y+seam.Dy()/2), ebiten.CursorShapeEWResize
		}},
		{"window edge grip", func(t *testing.T, k *Kit) (func(), image.Point, ebiten.CursorShapeType) {
			pumpKit(t, k, 1280, 720)
			r := k.OverviewWindow().GetContainer().GetWidget().Rect
			return nil, image.Pt(r.Min.X+2, (r.Min.Y+r.Max.Y)/2), ebiten.CursorShapeEWResize
		}},
		{"window corner grip", func(t *testing.T, k *Kit) (func(), image.Point, ebiten.CursorShapeType) {
			pumpKit(t, k, 1280, 720)
			r := k.OverviewWindow().GetContainer().GetWidget().Rect
			return nil, image.Pt(r.Max.X-2, r.Max.Y-2), ebiten.CursorShapeNWSEResize
		}},
		{"plain content", func(t *testing.T, k *Kit) (func(), image.Point, ebiten.CursorShapeType) {
			pumpKit(t, k, 1280, 720)
			r := k.OverviewWindow().GetContainer().GetWidget().Rect
			// Interior, clear of the 8 px grip band on every edge.
			return nil, image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2), ebiten.CursorShapeDefault
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prevMgmt := input.CursorManagementEnabled
			t.Cleanup(func() { input.CursorManagementEnabled = prevMgmt })

			k, _ := newTestKit(t)
			pumpKit(t, k, 1280, 720)
			sync, at, want := c.build(t, k)

			mouse := &fakeMouse{}
			input.SetCursorUpdater(mouse)
			t.Cleanup(func() { input.SetCursorUpdater(nil) })
			mouse.at(at.X, at.Y)
			t.Cleanup(widgets.SetPollCursorForTest(func() (int, int) { return at.X, at.Y }))

			var tr cursorTrace
			tr.last = ebiten.CursorShape()
			defer widgets.SetCursorShapeForTest(tr.record)()

			img := ebiten.NewImage(1280, 720)
			const frames = 6
			shapes := make([]ebiten.CursorShapeType, frames)
			for frame := 0; frame < frames; frame++ {
				tr.reset()
				before := ebiten.CursorShape()
				if sync != nil {
					sync()
				}
				k.Sync()
				if err := k.Update(); err != nil {
					t.Fatalf("frame %d: Update: %v", frame, err)
				}
				afterUpdate := ebiten.CursorShape()
				k.Draw(img)
				afterDraw := ebiten.CursorShape()
				shapes[frame] = afterDraw
				t.Logf("frame %d: before=%s afterUpdate=%s afterDraw=%s calls=%d differing=%d\n%s",
					frame, shapeName(before), shapeName(afterUpdate), shapeName(afterDraw), tr.calls, tr.differing,
					traceLines(tr.log))
				if tr.differing > 1 {
					t.Errorf("frame %d: %d differing cursor writes, want at most one (two writers alternate the shape)", frame, tr.differing)
				}
				if afterDraw != want {
					t.Errorf("frame %d: presented %s, want %s", frame, shapeName(afterDraw), shapeName(want))
				}
				if frame > 0 && afterDraw != shapes[frame-1] {
					t.Errorf("frame %d: presented %s after %s the frame before (stationary pointer must not change shape)",
						frame, shapeName(afterDraw), shapeName(shapes[frame-1]))
				}
			}
			if input.CursorManagementEnabled {
				t.Error("ebitenui cursor management is still enabled: its AfterDraw can reset the shape after the kit's write")
			}
		})
	}
}

func traceLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n"
	}
	return out
}
