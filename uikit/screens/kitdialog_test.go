package screens

import (
	"testing"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
)

// TestDialogLifecycle pins the one on-demand-window lifecycle (CONTEXT.md
// "Dialog") on a fake panel, so the rules survive every rewrite of the
// three real dialogs: first open mounts + fades in; a launch on the open
// window is ONE remove + ONE add with animate=false; a launch during the
// × close fade drops the pending unmount and fades back in; close is
// idempotent; a refused re-target mounts nothing; place runs on every
// show, and only the picker has one.
//
// The same six rules were reproducible through the pre-dialog trio (Show
// Info's ShowInfo/CloseInfo) before this file existed — the table's job
// is to keep them, not to find them.
func TestDialogLifecycle(t *testing.T) {
	// fakePanel is the dialog's panel half: a bare window, an open flag
	// its hide/isOpen closures flip (the real panels' contract), and a
	// place counter.
	type fakePanel struct {
		win         *widget.Window
		open        bool
		places      int
		placeTo     func(*widget.Window)
		refuseNext  bool
		targetShows int
	}
	newFake := func(place bool) *fakePanel {
		f := &fakePanel{win: widget.NewWindow(widget.WindowOpts.Contents(widget.NewContainer()))}
		theme.ShowWidget(f.win.GetContainer().GetWidget(), false)
		if place {
			f.placeTo = func(*widget.Window) { f.places++ }
		}
		return f
	}
	// bind builds the kit's dialog over the fake, exactly the panel shape
	// the Kit wires: hide flips visibility + the flag, isOpen reads it.
	bind := func(k *Kit, f *fakePanel) *dialog {
		var d *dialog
		d = k.newDialog(f.win,
			func() { f.open = false; theme.ShowWidget(f.win.GetContainer().GetWidget(), false) },
			func() bool { return f.open },
			f.placeTo,
			func() { k.closeDialog(d) })
		return d
	}
	show := func(k *Kit, d *dialog, f *fakePanel) {
		k.showDialog(d, func() bool {
			if f.refuseNext {
				f.refuseNext = false
				return false
			}
			f.targetShows++
			f.open = true
			theme.ShowWidget(f.win.GetContainer().GetWidget(), true)
			return true
		})
	}
	stackEntries := func(k *Kit, w *widget.Window) int {
		n := 0
		for _, m := range k.stack {
			if m.win == w {
				n++
			}
		}
		return n
	}
	top := func(k *Kit) *widget.Window {
		if len(k.stack) == 0 {
			return nil
		}
		return k.stack[len(k.stack)-1].win
	}

	t.Run("first open mounts and fades in", func(t *testing.T) {
		k, _ := newTestKit(t)
		f := newFake(false)
		d := bind(k, f)
		show(k, d, f)
		if stackEntries(k, f.win) != 1 {
			t.Fatalf("stack entries = %d, want 1", stackEntries(k, f.win))
		}
		if top(k) != f.win {
			t.Fatal("a fresh dialog did not land on top of the stack")
		}
		if a, ok := k.anims[f.win]; !ok || !a.open {
			t.Fatalf("first open did not fade in: ok=%v", ok)
		}
	})

	t.Run("launch on the open window is one remove + one add, no animate", func(t *testing.T) {
		k, _ := newTestKit(t)
		f := newFake(false)
		d := bind(k, f)
		show(k, d, f)
		show(k, d, f) // raise while the open fade still runs
		if n := stackEntries(k, f.win); n != 1 {
			t.Fatalf("stack entries = %d, want exactly one (a second add would append another)", n)
		}
		if _, ok := k.anims[f.win]; ok {
			t.Fatal("a raise restarted a fade — showDialog must mount with animate=false")
		}
		// Re-top proof: mount a rival above the dialog, then re-launch.
		rival := widget.NewWindow(widget.WindowOpts.Contents(widget.NewContainer()))
		theme.ShowWidget(rival.GetContainer().GetWidget(), true)
		k.AddWindow(rival)
		ch := k.chromes[f.win]
		show(k, d, f)
		if top(k) != f.win {
			t.Fatal("re-launch did not re-top the dialog")
		}
		if k.chromes[f.win] == ch {
			t.Fatal("the chrome survived the re-launch — no remove ran (re-mount must be remove + add)")
		}
		if n := stackEntries(k, f.win); n != 1 {
			t.Fatalf("stack entries after the raise = %d, want 1", n)
		}
	})

	t.Run("launch during the close fade drops the pending unmount", func(t *testing.T) {
		k, _ := newTestKit(t)
		f := newFake(false)
		d := bind(k, f)
		show(k, d, f)
		settlePump(t, k, 1280, 720)
		k.startClose(f.win) // the × path: close fade + deferred unmount
		if a, ok := k.anims[f.win]; !ok || a.open {
			t.Fatalf("no close fade committed: ok=%v", ok)
		}
		show(k, d, f)
		if a, ok := k.anims[f.win]; !ok || !a.open {
			t.Fatalf("reopen did not fade back in: ok=%v", ok)
		}
		settlePump(t, k, 1280, 720)
		if stackEntries(k, f.win) != 1 {
			t.Fatal("the pending unmount survived: the reopen lost its mount")
		}
		if !f.open {
			t.Fatal("reopened dialog reads closed")
		}
	})

	t.Run("close twice is idempotent", func(t *testing.T) {
		k, _ := newTestKit(t)
		f := newFake(false)
		d := bind(k, f)
		show(k, d, f)
		settlePump(t, k, 1280, 720)
		k.closeDialog(d)
		k.closeDialog(d)
		if f.open {
			t.Fatal("close left the panel open")
		}
		if stackEntries(k, f.win) != 0 {
			t.Fatalf("stack entries = %d, want 0", stackEntries(k, f.win))
		}
		k.closeDialog(d) // and again on a closed dialog
		if stackEntries(k, f.win) != 0 {
			t.Fatal("a third close remounted the window")
		}
	})

	t.Run("a refused re-target mounts nothing", func(t *testing.T) {
		k, _ := newTestKit(t)
		f := newFake(false)
		d := bind(k, f)
		f.refuseNext = true
		show(k, d, f)
		if stackEntries(k, f.win) != 0 {
			t.Fatalf("a refused show mounted the window")
		}
		if _, ok := k.anims[f.win]; ok {
			t.Fatal("a refused show animated")
		}
		if f.open {
			t.Fatal("a refused show opened the panel")
		}
	})

	t.Run("place runs on every show, only where one is set", func(t *testing.T) {
		k, _ := newTestKit(t)
		f := newFake(true)
		d := bind(k, f)
		show(k, d, f)
		show(k, d, f)
		if f.places != 2 {
			t.Fatalf("place ran %d times over two shows, want 2 (every show)", f.places)
		}
		plain := newFake(false)
		pd := bind(k, plain)
		if pd.place != nil {
			t.Fatal("a dialog without a place hook got one")
		}
		show(k, pd, plain) // must not panic or move anything
	})

	// The Kit's own wiring: all three dialogs register their × through
	// newDialog, and place is the picker's alone.
	t.Run("kit dialogs register the × and only the picker places", func(t *testing.T) {
		k, _ := newTestKit(t)
		for name, d := range map[string]*dialog{"info": k.info, "order": k.order, "qty": k.qty} {
			if d == nil {
				t.Fatalf("%s dialog is nil after NewKit", name)
			}
			if k.closes[d.win] == nil {
				t.Errorf("%s dialog has no registered × close", name)
			}
		}
		if k.info.place != nil || k.order.place != nil {
			t.Error("Show Info / order dialogs must not re-place on show")
		}
		if k.qty.place == nil {
			t.Error("the quantity picker must centre on every show")
		}
	})
}
