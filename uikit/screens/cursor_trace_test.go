package screens

import (
	"image"
	"testing"

	"github.com/ebitenui/ebitenui/input"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// traceSource is a scripted Source that records the shapes the frame
// writes. The frame's Apply is the repo's only cursor writer, so every OS
// write is one call here.
type traceSource struct {
	scriptPointer
	writes []ebiten.CursorShapeType
}

func (s *traceSource) SetShape(sh ebiten.CursorShapeType) {
	s.writes = append(s.writes, sh)
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
// seam, a window edge grip or plain content — a frame leaves exactly ONE
// shape behind (the frame's Apply) and presents the same shape on every
// frame. The kit computes the shape after all ebitenui input processing
// and applies it once (Pointer.Apply); ebitenui's own cursor management is
// off, so nothing can reset the shape behind the kit's back (the
// regression this pins: two writers alternating arrow/resize every frame).
func TestCursorSingleWriterPerFrame(t *testing.T) {
	cases := []struct {
		name string
		// build mounts the case's panel (if any) on the kit and returns
		// its per-frame Sync, and the cursor point plus the shape the
		// region must present.
		build func(t *testing.T, k *Kit, p *widgets.Pointer) (sync func(), at image.Point, want ebiten.CursorShapeType)
	}{
		{"vertical seam", func(t *testing.T, k *Kit, p *widgets.Pointer) (func(), image.Point, ebiten.CursorShapeType) {
			env, _ := newTestEnvSource(t, p)
			inv := NewInventory(env)
			k.AddWindow(inv.Win())
			pumpKit(t, k, 1280, 720)
			seam := splitSeam(inv.split, 0)
			return inv.Sync, image.Pt(seam.Min.X+2, seam.Min.Y), ebiten.CursorShapeNSResize
		}},
		{"horizontal seam", func(t *testing.T, k *Kit, p *widgets.Pointer) (func(), image.Point, ebiten.CursorShapeType) {
			env, _ := newTestEnvSource(t, p)
			m := NewMarket(env)
			k.AddWindow(m.Win())
			pumpKit(t, k, 1280, 720)
			seam := splitSeam(m.split, 0)
			return m.Sync, image.Pt(seam.Min.X, seam.Min.Y+seam.Dy()/2), ebiten.CursorShapeEWResize
		}},
		{"window edge grip", func(t *testing.T, k *Kit, p *widgets.Pointer) (func(), image.Point, ebiten.CursorShapeType) {
			pumpKit(t, k, 1280, 720)
			r := k.OverviewWindow().GetContainer().GetWidget().Rect
			return nil, image.Pt(r.Min.X+2, (r.Min.Y+r.Max.Y)/2), ebiten.CursorShapeEWResize
		}},
		{"window corner grip", func(t *testing.T, k *Kit, p *widgets.Pointer) (func(), image.Point, ebiten.CursorShapeType) {
			pumpKit(t, k, 1280, 720)
			r := k.OverviewWindow().GetContainer().GetWidget().Rect
			return nil, image.Pt(r.Max.X-2, r.Max.Y-2), ebiten.CursorShapeNWSEResize
		}},
		{"plain content", func(t *testing.T, k *Kit, p *widgets.Pointer) (func(), image.Point, ebiten.CursorShapeType) {
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

			src := &traceSource{}
			p := widgets.NewPointer(src)
			k, _ := newTestKitWith(t, p)
			pumpKit(t, k, 1280, 720)
			sync, at, want := c.build(t, k, p)
			src.x, src.y = at.X, at.Y

			img := ebiten.NewImage(1280, 720)
			const frames = 6
			shapes := make([]ebiten.CursorShapeType, frames)
			for frame := 0; frame < frames; frame++ {
				src.writes = nil
				p.Begin() // the host's one sample per Update
				if sync != nil {
					sync()
				}
				k.Sync()
				if err := k.Update(); err != nil {
					t.Fatalf("frame %d: Update: %v", frame, err)
				}
				k.Draw(img)
				if len(src.writes) != 1 {
					t.Errorf("frame %d: %d cursor writes, want exactly one (Apply)", frame, len(src.writes))
					continue
				}
				shapes[frame] = src.writes[0]
				t.Logf("frame %d: presented %s", frame, shapeName(shapes[frame]))
				if shapes[frame] != want {
					t.Errorf("frame %d: presented %s, want %s", frame, shapeName(shapes[frame]), shapeName(want))
				}
				if frame > 0 && shapes[frame] != shapes[frame-1] {
					t.Errorf("frame %d: presented %s after %s the frame before (stationary pointer must not change shape)",
						frame, shapeName(shapes[frame]), shapeName(shapes[frame-1]))
				}
			}
			if input.CursorManagementEnabled {
				t.Error("ebitenui cursor management is still enabled: its AfterDraw can reset the shape after the kit's write")
			}
		})
	}
}
