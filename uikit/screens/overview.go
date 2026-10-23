package screens

import (
	"image"
	"image/color"
	"sort"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// ovRow is the overview's persistent per-entity row payload: the pointer
// identity doubles as the Table's stable selection/activation key while
// info refreshes every sync (distances tick, flags change).
type ovRow struct {
	info sim.Info
}

// InfoRefOf resolves the row payload inside the seam's Show Info
// resolver (widgets/infoRefPayload): the entity behind the row.
func (r *ovRow) InfoRefOf(cv widgets.ContentView) (widgets.InfoRef, bool) {
	return widgets.InfoRefFor(cv, r.info.Entity)
}

// overview owns the Overview window: a WindowPanel whose body is a
// Table of the visible grid.
type overview struct {
	win  *widget.Window
	tbl  *widgets.Table
	rows map[ecs.Entity]*ovRow // persistent row objects keyed by entity
}

// Env returns this kit's session — the three seams (one Theme, the
// read-only view, the installed ActionSink) plus the shared popup
// services, the same values its own windows were built from. Panel
// hosts construct their extra panels from this instead of calling
// NewEnv with the same arguments (that built a second theme/face set
// the kit never used).
func (k *Kit) Env() Session { return k.sess }

// kitMount is one mounted window: its place in the stack (bottom →
// top) and the remove func that detaches it — chrome registry, stack
// entry and ebitenui window in one call. The same closure is what
// AddWindow returns to the host, so a host unmount and a raise both
// leave identical state behind.
type kitMount struct {
	win    *widget.Window
	remove widget.RemoveWindowFunc
}

// AddWindow mounts an extra window on the kit's UI so it shares the
// kit's theme, stacking and input pump. Every mounted window gets the
// four-edge resize chrome here (the registry is the kit's: see
// Kit.chromes), joins the kit's stack order (Kit.stack — the raise's
// hit-test reads it; ebitenui's own relayering is switched off for kit
// windows so remove + re-mount stays the single top-of-stack path), and
// the returned func closes the window again — detaching its chrome, so
// an unmounted window stops being pumped and a drag interrupted by the
// unmount leaves no rect or cursor state behind. Mounting a window that
// is already mounted is a no-op that returns the same detach.
//
// A fresh mount runs the open animation (fade + slide in — the window
// stays input-live throughout); the internal raise paths call
// addWindow(w, false) instead, because a raise is not an open. A window
// whose × close is still fading re-mounts as a reopen: the pending
// close (and its deferred unmount) is dropped here.
func (k *Kit) AddWindow(w *widget.Window) widget.RemoveWindowFunc {
	return k.addWindow(w, true)
}

// addWindow is AddWindow with the open-animation switch (see AddWindow).
func (k *Kit) addWindow(w *widget.Window, animate bool) widget.RemoveWindowFunc {
	// Re-mounting a window whose close is still fading = a reopen: the
	// unmount its remove owed is cancelled right here.
	k.cancelClose(w)
	_, ok := k.chromes[w]
	if !ok {
		k.chromes[w] = &winChrome{win: w, closable: k.closes[w] != nil}
		// Drag-arming settles the open fade: the fade's park flips would
		// relayout the subtree the drag translates (CONTEXT.md "Settle").
		k.chromes[w].settle = func() { k.settleOpen(w) }
	}
	if m, mounted := k.mountOf(w); mounted {
		if animate {
			// The still-mounted reopen path (a × close deferred its
			// unmount): fade back in.
			k.startOpen(w)
		}
		return m.remove
	}
	// A fresh append changes the stack. A raise (animate=false) retops
	// one window over every animating one — nothing may keep fading
	// below it, so everything settles. An open mount lands on top of the
	// stack: the other open fades keep running (the composite walks the
	// stack in order, so their paint order stays correct), but a close
	// fade anywhere must finish now — its window just landed below this
	// one and the composite would paint it over the new mount.
	if animate {
		k.settleCloseAnims()
	} else {
		k.settleAnims()
	}
	// ebitenui would re-top a window when its container sees a press —
	// but a body widget's scroll viewport claims the input layer over
	// the container, so that relayering only ever fires on the title
	// bar, and it would reorder ebitenui's stack behind the kit's back.
	// The kit's own raise (remove + re-mount, exactly what ShowInfo
	// does) is the one deterministic path; the built-in one stays off.
	w.DisableRelayering = true
	clipWindowBody(w)
	remove := k.ui.AddWindow(w)
	// The actual unmount (chrome detach, stack entry, ebitenui window) —
	// what m.remove runs directly, and what a running close fade holds
	// on to until it settles.
	real := func() {
		if ch, ok := k.chromes[w]; ok {
			ch.cancel() // mid-drag unmount: drop the grip state first
			delete(k.chromes, w)
		}
		delete(k.anims, w)
		k.unstack(w)
		remove()
	}
	m := &kitMount{win: w}
	m.remove = func() {
		if a, ok := k.anims[w]; ok && !a.open {
			// The × close is fading: the unmount waits for settle (run
			// once, there — winanim.go). Idempotent while it lasts.
			if a.remove == nil {
				a.remove = real
			}
			return
		}
		real()
	}
	if animate {
		k.startOpen(w)
	}
	k.stack = append(k.stack, m)
	return m.remove
}

// mountOf finds win's stack entry (false = not mounted on this kit).
func (k *Kit) mountOf(w *widget.Window) (*kitMount, bool) {
	for _, m := range k.stack {
		if m.win == w {
			return m, true
		}
	}
	return nil, false
}

// unstack drops win's stack entry (identity match — a stale remove
// closure must never remove a re-mounted window's live entry, so the
// entry is matched by window, not by closure).
func (k *Kit) unstack(w *widget.Window) {
	for i, m := range k.stack {
		if m.win == w {
			k.stack = append(k.stack[:i], k.stack[i+1:]...)
			return
		}
	}
}

// buildOverview constructs the Overview window around its Table.
func (k *Kit) buildOverview() {
	o := &overview{rows: map[ecs.Entity]*ovRow{}}
	k.ov = o

	tbl := widgets.NewTable(k.Env(), []widgets.Column{
		{Key: "Name", Title: "Name", Priority: 0},
		{Key: "Distance", Title: "Distance", Priority: 1, AlignRight: true},
		{Key: "Kind", Title: "Kind", Priority: 2},
		{Key: "Velocity", Title: "Velocity", Priority: 3, AlignRight: true},
	}, widgets.PrefsID("overview"))
	// Typed sort: string cells would order "1.2 AU" before "42 m".
	tbl.SetSortFunc(sortOverview)
	tbl.SetSort("Distance", true)
	tbl.OnSelect(func(r widgets.TableRow) {
		if row, ok := r.Data.(*ovRow); ok {
			k.selected = row.info.Entity
		}
	})
	// Double-click shows the row's definition (an NPC caste, a hull, an
	// asteroid's ore — the old behaviour, approach-on-activate, moved to
	// the world scene and the menu); right-click opens the entity menu.
	// Both derive from the row's declared RowSpec (overviewSpec), built
	// per row in sync.
	o.tbl = tbl

	// Body: the table fills its grid cell exactly, so the scroll
	// viewport is bounded by the window no matter how tall the rows are.
	contents := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.Bordered(theme.WindowBody(k.th.Pal().BG), theme.NRGBA(k.th.Pal().Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	contents.AddChild(tbl.Container())

	title := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.Bordered(theme.Solid(k.th.Pal().BGHard), theme.NRGBA(k.th.Pal().Border, 200), 1)),
		widget.ContainerOpts.Layout(widget.NewAnchorLayout()),
	)
	title.AddChild(widget.NewText(
		widget.TextOpts.Text("Overview", k.th.FaceBold(theme.StepBase), theme.Solid(k.th.Pal().Accent)),
		widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionStart,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
			Padding:            widget.NewInsetsSimple(theme.Px(theme.ContentsPad)),
		})),
	))

	o.win = widget.NewWindow(
		widget.WindowOpts.Contents(contents),
		widget.WindowOpts.TitleBar(title, theme.Px(theme.TitleBarH)),
		// Resize comes from the winchrome edge grips (all four edges),
		// not widget.WindowOpts.Resizeable() — that one only stretches
		// the bottom/right corner. Kit.AddWindow attaches the chrome.
		widget.WindowOpts.MinSize(theme.Px(320), theme.Px(240)),
		// The default clears the rail's left gutter (RailInset) at the
		// construction scale; the game re-anchors it the same way
		// (placeDefaults reads the same constant) and moves it to the
		// viewport's upper-right corner. Its height stops 8 px above the
		// module rack's top edge at the 1024×700 minimum (425 − 8 − 8 =
		// 409), so the corner placement never overlaps the rack.
		widget.WindowOpts.Location(image.Rect(theme.Px(8+RailInset), theme.Px(8), theme.Px(388+RailInset), theme.Px(8+409))),
	)
}

// overviewSpec is one overview row's declared interactions: Show Info on
// what the entity IS (declared even when it resolves to nothing — the
// entry shows grayed, first and bold), then the Selected Item window's
// own verbs as row actions — every entry queues the sim command its
// button queues (CmdWarpTo/CmdOrbit/CmdApproach/CmdLock), at the row's
// entity and at the range selectors' current value (read at press).
// Entries the sim would refuse (self-targeting, unwarpageable,
// unlockable kinds) show grayed. The installer turns this data into the
// menu; a right-click never moves the selection.
func (k *Kit) overviewSpec(row *ovRow) widgets.RowSpec {
	e := row.info.Entity
	spec := widgets.RowSpec{Ref: widgets.RowRef(k.cv, row)}
	info, hasInfo := k.cv.Info(e)
	if hasInfo {
		self := e == k.cv.Player()
		tooClose := info.Distance < sim.WarpMinRange
		// The verbs are intents: the executor reads the warp/orbit range
		// from the Selected Item selectors when they fire.
		spec.Actions = []widgets.RowAction{
			{Label: "Warp to", Disabled: self || !info.CanWarp || tooClose,
				Intent: widgets.WarpTo{Entity: e}},
			{Label: "Orbit", Disabled: self, Intent: widgets.Orbit{Entity: e}},
			{Label: "Approach", Disabled: self, Intent: widgets.Approach{Entity: e}},
			{Label: "Lock target", Disabled: self || !canLock(info.Kind),
				Intent: widgets.Lock{Entity: e}},
		}
	}
	return spec
}

// sync refreshes the rows from the sim: visible grid entities only, with
// hostile/player tints, then lets the table sort and diff the widgets.
func (o *overview) sync(k *Kit) {
	order := make([]widgets.TableRow, 0, 32)
	for _, info := range k.cv.Grid() {
		if !info.Visible {
			continue
		}
		var tint, iconTint color.RGBA
		switch {
		case info.Entity == k.cv.Player():
			tint = widgets.WithA(k.th.Pal().Player, 60)
			iconTint = k.th.Pal().Info // own ship wears ColInfo
		case info.Hostile:
			tint = widgets.WithA(k.th.Pal().Danger, 60)
			iconTint = k.th.Pal().Danger // hostiles wear ColDanger
		}
		row := o.rowFor(info)
		order = append(order, widgets.TableRow{
			Cells: []string{
				info.Name,
				theme.FormatDistance(info.Distance),
				theme.KindLabel(info.Kind),
				theme.FormatSpeed(info.Vel.Len()),
			},
			Icons:    []string{"", "", theme.KindIcon(info.Kind), ""},
			IconTint: iconTint,
			Tint:     tint,
			Data:     row,
			Spec:     k.overviewSpec(row),
		})
	}
	// Prune row objects of entities that left the grid (selection
	// identity survives because it lives on the *ovRow, not the index).
	if len(o.rows) != len(order) {
		fresh := make(map[ecs.Entity]*ovRow, len(order))
		for _, r := range order {
			row := r.Data.(*ovRow)
			fresh[row.info.Entity] = row
		}
		o.rows = fresh
	}
	o.tbl.SetRows(order)
	o.tbl.Sync()
}

// rowFor returns the persistent row object for an entity, refreshing its
// info snapshot.
func (o *overview) rowFor(info sim.Info) *ovRow {
	r := o.rows[info.Entity]
	if r == nil {
		r = &ovRow{}
		o.rows[info.Entity] = r
	}
	r.info = info
	return r
}

// selectEntity syncs the row highlight to an externally chosen selection
// (a world click or keyboard cycle). Unknown entities clear the highlight.
func (o *overview) selectEntity(e ecs.Entity) {
	if r, ok := o.rows[e]; ok {
		o.tbl.SelectData(r)
		return
	}
	o.tbl.SelectData(nil)
}

// drawTints is the kit.go Draw seam: the Table paints row tints itself
// now (per-row backgrounds), so there is no post-draw tint pass left.
func (o *overview) drawTints(screen *ebiten.Image, k *Kit) {}

// sortOverview orders overview rows stably by their typed Info values;
// equal keys keep their previous relative order.
func sortOverview(key string, asc bool, rows []widgets.TableRow) {
	less := func(a, b widgets.TableRow) bool {
		ia, okA := a.Data.(*ovRow)
		ib, okB := b.Data.(*ovRow)
		if !okA || !okB {
			return false
		}
		switch key {
		case "Name":
			return ia.info.Name < ib.info.Name
		case "Distance":
			return ia.info.Distance < ib.info.Distance
		case "Kind":
			return ia.info.Kind < ib.info.Kind
		case "Velocity":
			return ia.info.Vel.Len() < ib.info.Vel.Len()
		default:
			return false
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if asc {
			return less(rows[i], rows[j])
		}
		return less(rows[j], rows[i])
	})
}
