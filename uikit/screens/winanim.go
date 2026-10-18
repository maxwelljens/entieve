package screens

import (
	"image"

	"github.com/ebitenui/ebitenui/input"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// Window close affordance and open/close animation (CONTEXT.md: "Close
// button", "Window animation", "Settle").
//
// The × is one ebitenui Button per closable window, attached to the
// title bar when the host registers the window's close semantics
// (SetWindowClose) — the same pattern Show Info's bar always carried.
// Because the button is an ordinary widget it rides Kit.Update's menu
// guard for free: while the context menu owns the frame, ui.Update is
// skipped, so a × press can never close through the popup (see
// window_raise_test.go's menu contract). It fires into startClose, which
// runs the registered close — the window's EXISTING close path (a Screen
// row's toggle, the core toggle, CloseInfo) — and commits the close
// fade.
//
// The animation is purely render-time: no rects move, no layout or
// simulation state changes, nothing consumes RNG. An animating window is
// parked out of ebitenui's window pass (Visibility_Hide, exactly the
// TopChrome park) before ui.Draw, then painted by Kit.Draw's own pass —
// drawn into one screen-sized offscreen and composited back with alpha
// and a slide offset. Between park and composite the window sits shown,
// so input stays live during an open fade (a window you just opened
// accepts clicks mid-fade, and its widgets' press events fire because
// Widget.Update only feeds visible widgets) while a closing window is
// hidden through every Update — closing is committed, so the window is
// inert for the whole fade. The park/unpark flip runs twice per frame
// while a window animates — transient by construction: a drag-arming
// press settles that window's fade and a raise settles them all (a new
// mount settles the close fades), so the per-frame relayout a flip
// requests can never land under a drag and a fade never paints above a
// window that stacks above it (CONTEXT.md "Settle").
//
// Unmounting a closing window is deferred to settle: Kit.AddWindow's
// remove func records itself on the animation instead of running, and
// the fade's last frame performs it. A core window closes by hiding
// (no remove is ever called) and simply stays hidden after settle.

// Animation timing, in frames at 60 TPS — render-only, counted in Draw
// (deterministic frame counting, never wall-clock). Pinned by
// TestWindowAnimationEndpoints.
const (
	openFrames  = 12 // ≈0.2 s fade + slide in
	closeFrames = 10 // fade out before the deferred unmount
	openSlidePx = 8  // slide distance in DESIGN units (from below, upward)
)

// winAnim is one window's running animation. frame counts the frames
// drawn so far in the current phase; remove is the host's deferred
// unmount for a close fade (nil when the close does not unmount — a
// core window — or when the host has not called remove yet).
type winAnim struct {
	open   bool
	frame  int
	remove widget.RemoveWindowFunc
}

// openAnim maps an open frame to its render state: alpha 0→255 and the
// slide offset fading from openSlidePx to 0 over openFrames frames.
// frame ≥ openFrames is the settled endpoint (255, 0) — the state a
// window is in for every frame after its fade completes, which is
// exactly the normal paint path.
func openAnim(frame int) (uint8, int) {
	if frame >= openFrames {
		return 255, 0
	}
	if frame <= 0 {
		return 0, theme.Px(openSlidePx)
	}
	return uint8(frame * 255 / openFrames),
		(openFrames - frame) * theme.Px(openSlidePx) / openFrames
}

// closeAnim maps a close frame to its alpha: 255 at frame 0, 0 at the
// closeFrames endpoint (the frame the deferred unmount runs).
func closeAnim(frame int) uint8 {
	if frame <= 0 {
		return 255
	}
	if frame >= closeFrames {
		return 0
	}
	return uint8((closeFrames - frame) * 255 / closeFrames)
}

// SetWindowClose registers win's close semantics — the callback the
// title bar's × runs — and attaches the × itself (once, and only when
// the window has a title bar: the station services view owns no title
// bar and stays mouse-closable only through its own chrome). The
// callback is the window's EXISTING close path; startClose runs it
// inside the close fade. Hosts call this once per built window, before
// or after the first mount.
func (k *Kit) SetWindowClose(w *widget.Window, close func()) {
	if w == nil || close == nil {
		return
	}
	k.closes[w] = close
	if ch, ok := k.chromes[w]; ok {
		ch.closable = w.TitleBar != nil
	}
	if w.TitleBar == nil || k.closeBtns[w] != nil {
		return
	}
	btn := widget.NewButton(
		widget.ButtonOpts.TextLabel("×"),
		widget.ButtonOpts.WidgetOpts(
			// ≥40 design-unit hit box, scaled with everything else; the
			// chrome's closeButtonRect strip is sized to match it.
			widget.WidgetOpts.MinSize(theme.Px(closeBtnW), theme.Px(closeBtnH)),
			widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
				HorizontalPosition: widget.AnchorLayoutPositionEnd,
				VerticalPosition:   widget.AnchorLayoutPositionCenter,
			}),
		),
		widget.ButtonOpts.ClickedHandler(func(*widget.ButtonClickedEventArgs) {
			// A press the chrome turned into a drag is a drag, not a
			// click: a corner grip drag ends at the same RELATIVE point
			// (the button travels with the window), so its release would
			// otherwise close the window the player just resized. The
			// drag owns the gesture — grip precedence — while a
			// stationary press (armed, never active) still closes.
			if ch, ok := k.chromes[w]; ok && (ch.active || ch.moveActive) {
				return
			}
			k.startClose(w)
		}),
	)
	w.TitleBar.AddChild(btn)
	k.closeBtns[w] = btn
}

// CloseButtonOf returns win's × button (nil when the window is not
// closable or has no title bar) — the handle tests press.
func (k *Kit) CloseButtonOf(w *widget.Window) *widget.Button { return k.closeBtns[w] }

// startClose commits win's close: it starts the close fade (the window
// goes inert this very frame) and runs the registered close semantics —
// for a Screen row that flips its open flag so the host's own mount
// reconciliation performs the unmount, for a core window it hides, for
// Show Info it runs CloseInfo. A close already committed is never run
// twice (the button cannot fire twice — the window is hidden from this
// frame on — but the guard makes the seam safe anyway).
func (k *Kit) startClose(w *widget.Window) {
	close := k.closes[w]
	if close == nil {
		return
	}
	cur, running := k.anims[w]
	if running && !cur.open {
		return // close already committed
	}
	a := &winAnim{open: false}
	if running && cur.open {
		// Closed mid-open: continue from the alpha on screen — no flash
		// back to full. Integer math keeps the frame count deterministic.
		alpha, _ := openAnim(cur.frame)
		a.frame = (255 - int(alpha)) * closeFrames / 255
	}
	k.anims[w] = a
	// Inert from now: Widget.Update feeds no events to a hidden tree,
	// and the next raisePressed/step step skips the hidden rect.
	theme.ShowWidget(w.GetContainer().GetWidget(), false)
	close()
}

// cancelClose drops a pending close fade without unmounting: the player
// reopened the window (the core rail toggle, the docked veil lifting
// with a close still fading) and no unmount is owed — core windows
// never register one.
func (k *Kit) cancelClose(w *widget.Window) {
	if a, ok := k.anims[w]; ok && !a.open {
		delete(k.anims, w)
	}
}

// settleOpen drops a running open animation: the window snaps to the
// normal paint path (full alpha, no slide) from the next paint on. The
// chrome calls it when a drag arms.
func (k *Kit) settleOpen(w *widget.Window) {
	if a, ok := k.anims[w]; ok && a.open {
		delete(k.anims, w)
	}
}

// settleAnims finishes every running animation at once (CONTEXT.md
// "Settle"): open windows snap to full, closing windows perform their
// deferred unmount now. A raise calls it — a retop puts one window above
// every animating one, and the composite pass paints only above the
// window pass, so nothing may keep fading underneath it.
func (k *Kit) settleAnims() {
	k.settleWhere(func(*winAnim) bool { return true })
}

// settleCloseAnims finishes just the close fades: a fresh open mount
// lands on top of the stack (the other open fades below it keep running,
// the composite's stack order is their paint order), but a closing
// window that now sits below the new mount would composite over it.
func (k *Kit) settleCloseAnims() {
	k.settleWhere(func(a *winAnim) bool { return !a.open })
}

// settleWhere runs settle's end-of-animation work for every animation
// the predicate accepts.
func (k *Kit) settleWhere(accept func(*winAnim) bool) {
	for w, a := range k.anims {
		if !accept(a) {
			continue
		}
		delete(k.anims, w)
		if a.open {
			continue // snapped to full: the normal paint path from here
		}
		k.finishClose(w, a)
	}
}

// finishClose ends a close animation: the deferred unmount runs if the
// host owed one. When none is owed the close's hide must still be
// lifted unless a core window owns the visibility itself (applyCore —
// its rail/dock state already says hidden, or the player reopened it
// through cancelClose). The no-unmount non-core case is a host whose
// open flag flip-flopped back to open before its reconciliation ever
// ran the remove — the window stays mounted and wants to be shown.
func (k *Kit) finishClose(w *widget.Window, a *winAnim) {
	if a.remove != nil {
		remove := a.remove
		a.remove = nil
		remove()
		return
	}
	if w == nil {
		return
	}
	if (k.ov != nil && w == k.ov.win) || (k.sp != nil && w == k.sp.win) {
		return // a core window: applyCore owns its visibility
	}
	theme.ShowWidget(w.GetContainer().GetWidget(), true)
}

// parkAnims hides every animating window before ui.Draw: the window
// pass paints neither it nor its relayout. ebitenui registers the
// window's own input layer regardless of visibility — but a nested
// ScrollContainer does not, which is what relayAnimWindows repairs
// after the unpark.
func (k *Kit) parkAnims() {
	for w := range k.anims {
		theme.ShowWidget(w.GetContainer().GetWidget(), false)
	}
}

// unparkAnims brings every animating window back after ui.Draw: the
// draw-phase chrome step and the composite below see a shown window
// (the closing window's press edge was spent while it was hidden, so
// this half-frame of visibility can arm nothing), and the next Update
// runs with an open window live — a closing window is closed off again
// by drawAnims, which hides it before the frame ends.
func (k *Kit) unparkAnims() {
	for w := range k.anims {
		theme.ShowWidget(w.GetContainer().GetWidget(), true)
	}
}

// relayAnimWindows re-runs the input-layer setup of every animating
// window, in stack order. ui.Draw built the layer stack while they were
// parked, and ebitenui's ScrollContainer only elevates its viewport
// layer when it is visible — a parked table would lose its body layer
// for the whole frame (row presses and the grip's wheel-layer trick
// read those layers). An animating window is always top-of-stack (a
// mount appends on top, a × close follows its raise, and the settle
// rules hold it there), so re-running setup here re-adds each one's
// window + viewport layers in stack order on top of the settled ones —
// exactly where the paint pass put their pixels. The top chrome and the
// menu elevate after this pass, as before.
func (k *Kit) relayAnimWindows() {
	if len(k.anims) == 0 {
		return
	}
	// The deferred-setup executor ebitenui uses internally, wired to run
	// immediately (the same pattern as drawTops): the layers must land
	// in place, not back on ui's already-drained queue.
	var next input.DeferredSetupInputLayerFunc
	next = func(s input.SetupInputLayerFunc) { s(next) }
	for _, m := range k.stack {
		if _, ok := k.anims[m.win]; ok {
			m.win.SetupInputLayer(next)
		}
	}
}

// animJob is one animating window queued for this frame's composite.
type animJob struct {
	win  *widget.Window
	anim *winAnim
}

// drawAnims is Kit.Draw's animation pass: composite every animating
// window (stack order, bottom → top, so the pass preserves paint order
// among windows), advance the frame counters, and settle the animations
// that have run out — a closing window's deferred unmount executes here.
// The offscreen is one screen-sized image reused across frames and
// windows and only exists while an animation runs, so settled windows
// (the normal paint path) cost exactly what they cost before this
// feature.
func (k *Kit) drawAnims(screen *ebiten.Image) {
	if len(k.anims) == 0 {
		return
	}
	// Snapshot through the stack (settle below unmounts, which rewrites
	// it): every animating window is mounted by construction — remove
	// drops the animation with the chrome.
	jobs := make([]animJob, 0, len(k.anims))
	for _, m := range k.stack {
		if a, ok := k.anims[m.win]; ok {
			jobs = append(jobs, animJob{win: m.win, anim: a})
		}
	}
	for _, j := range jobs {
		if _, still := k.anims[j.win]; !still {
			continue // a previous job's settle already finished it
		}
		alpha, dy := openAnim(j.anim.frame)
		if !j.anim.open {
			alpha, dy = closeAnim(j.anim.frame), 0
		}
		// The window always renders into the offscreen — even at alpha 0
		// (the open fade's first frame): that pass is what lays the
		// subtree out, and dropping it would delay every rect-dependent
		// read (a table's first fit, the bounds pins) by a frame versus
		// the settled path. Only the composite onto the screen is
		// conditional — nothing of an invisible window reaches pixels.
		img := k.animSurface(screen.Bounds())
		j.win.Render(img)
		if alpha > 0 {
			opts := &ebiten.DrawImageOptions{}
			opts.GeoM.Translate(0, float64(dy))
			opts.ColorScale.ScaleAlpha(float32(alpha) / 255)
			screen.DrawImage(img, opts)
		}
		if !j.anim.open {
			// Closing: hidden again before the next Update — the fade is
			// committed, the window stays inert until it unmounts.
			theme.ShowWidget(j.win.GetContainer().GetWidget(), false)
		}
		j.anim.frame++
		limit := closeFrames
		if j.anim.open {
			limit = openFrames
		}
		if j.anim.frame >= limit {
			delete(k.anims, j.win)
			if !j.anim.open {
				k.finishClose(j.win, j.anim) // the deferred unmount, exactly at settle
			}
		}
	}
}

// animSurface returns the animation composite's offscreen (sized to the
// screen, cleared for this window's pass). It exists only while at
// least one window animates.
func (k *Kit) animSurface(b image.Rectangle) *ebiten.Image {
	if k.animImg == nil || k.animImgSize != b.Size() {
		k.animImg = ebiten.NewImage(b.Dx(), b.Dy())
		k.animImgSize = b.Size()
	}
	k.animImg.Clear()
	return k.animImg
}
