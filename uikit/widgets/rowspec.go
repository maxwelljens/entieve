package widgets

import (
	"image"
	"time"

	"github.com/entieve/entieve/content"
)

// Row interactions — the RowSpec seam (CONTEXT.md: Row, RowSpec, Ref,
// Actions).
//
// A Row's interactions are declared data: its RowSpec. One installer
// derives everything a row offers from that data, so no surface builds
// menu entries, tooltips or double-click gestures by hand again:
//
//	RowSpec
//	├─ Ref/SecRef  → right-click: "Show Info" (bold, first), then the
//	│                secondary ref "Show Info — <name>"; grayed when the
//	│                def is not loaded. Double-click opens Ref, strictly.
//	├─ Actions     → right-click: MenuSep then the row's verbs as typed
//	│                intents, grayed when disabled (never hidden).
//	└─ Tip         → hover: the lazy tooltip (rowTip — built on first
//	                   attach, rebuilt only when its change key moves).
//
// Surfaces route their EXISTING click dispatch through the installer
// (Table.rowClicked/rowMenu, the rack's cell handler, each widget
// handler) instead of having handlers attached on their behalf: the
// table must select before the double-click resolves, and the rack's
// single-click toggle has to consult the double-click first (a second
// click inside the window shows info and skips the toggle). The shape,
// gating and timing rules live here, once.

// RowAction is one verb a row offers in its context menu: the label as
// it reads, the gate as it stood when the spec was declared, and the
// payload — a typed intent the row's ActionSink executes. These field
// names and the installer API are the stable surface of the seam.
type RowAction struct {
	Label    string
	Disabled bool
	Intent   Intent
}

// TipData is a row's tooltip payload: the change key plus the lazy model
// builder — the one mechanism behind what rowTip.set, the rack's
// cellTip and the fitting table's tableRowTips used to do separately.
// An empty Build means the row describes nothing hoverable (any
// attached tooltip drops).
type TipData struct {
	Key   string
	Build func() TooltipModel
}

// RowSpec is one Row's declared interaction data (CONTEXT.md): what the
// row IS (Ref, optionally SecRef — drives Show Info), what it OFFERS
// (Actions — its verbs, as intents) and how it describes itself on hover
// (Tip). The zero spec wires nothing: no menu, no tooltip, no
// double-click — an anonymous row (scanner contact, note row, the
// resists strip) simply carries none.
//
// Ref and SecRef are pointers: the tri-state matters. nil = the row
// shows no info at all (no menu entry); a set-but-unresolvable ref
// keeps its entry, grayed — the grayed-not-hidden convention at the ref
// level too (the overview's def-less celestials read exactly like
// before).
type RowSpec struct {
	Ref     *InfoRef
	SecRef  *InfoRef // second info entry ("Show Info — <name>"): the rack's loaded charge
	Actions []RowAction
	Tip     TipData
}

// rowRef resolves a row payload to the spec's Ref. The ref is declared
// whether or not it resolves: an unresolvable one earns a grayed Show
// Info entry instead of vanishing.
func RowRef(cv ContentView, data any) *InfoRef {
	ref, _ := cv.InfoRefOf(data)
	return &ref
}

// PtrRef boxes a directly-known ref for a RowSpec's Ref/SecRef field.
func PtrRef(r InfoRef) *InfoRef { return &r }

// RowMenuEntries derives the context-menu entries from a spec — the one
// place the menu shape exists:
//
//	[Show Info            bold, first, when Ref is set]
//	[Show Info — <name>]  the secondary ref, when SecRef is set
//	[───────────]         MenuSep, only between a non-empty info group
//	                      and a non-empty action group
//	[action ...]          the Actions, grayed when disabled — each one
//	                      emitting its intent through the sink
//
// An empty spec yields no entries at all (an empty rack slot, a note
// row: no popup, not an empty one).
func RowMenuEntries(cv ContentView, as ActionSink, spec RowSpec) []MenuEntry {
	var info, verbs []MenuEntry
	if spec.Ref != nil {
		info = append(info, showInfoEntry(cv, as, *spec.Ref, true))
	}
	if spec.SecRef != nil {
		info = append(info, showInfoEntry(cv, as, *spec.SecRef, false))
	}
	for _, a := range spec.Actions {
		action := a
		verbs = append(verbs, MenuEntry{Label: action.Label, Disabled: action.Disabled, Action: func() {
			// Grayed rows emit nothing — belt and braces with the menu's
			// own disabled gate (a harness may press the closure directly).
			if !action.Disabled && action.Intent != nil {
				as.Do(action.Intent)
			}
		}})
	}
	entries := info
	if len(info) > 0 && len(verbs) > 0 {
		entries = append(entries, MenuSep)
	}
	return append(entries, verbs...)
}

// showInfoEntry is one Show Info menu entry for ref: bold when it leads
// the menu, labelled "Show Info — <name>" as a secondary ref, grayed
// (never hidden) when the def is not loaded. Pressing it emits
// ShowInfo{Ref} through the sink.
func showInfoEntry(cv ContentView, as ActionSink, ref InfoRef, primary bool) MenuEntry {
	label := "Show Info"
	if !primary {
		label += " — " + ChainName(cv, content.ItemID(ref.ID))
	}
	ok := InfoRefExists(cv, ref)
	return MenuEntry{Label: label, Bold: primary, Disabled: !ok, Action: func() {
		if ok {
			as.Do(ShowInfo{Ref: ref})
		}
	}}
}

// openRowMenu is the installer's right-click half: it derives the spec's
// menu and mounts it through the sink (OpenMenu) at at — the click
// position. A spec with nothing to offer opens no menu at all — the
// world-input gate the menu exposes (MenuRect) never sees an empty popup.
func OpenRowMenu(cv ContentView, as ActionSink, spec RowSpec, at image.Point) {
	entries := RowMenuEntries(cv, as, spec)
	if len(entries) == 0 {
		return
	}
	as.Do(OpenMenu{Entries: entries, At: at})
}

// RowDouble is one row's (or one surface's) double-click bookkeeping —
// the installer's double-click half. A repeat click on the same
// RESOLVABLE Ref inside doubleClickWindow completes the gesture: it
// opens Show Info and nothing else (strictly), reports true so a
// surface with its own single-click activation (the rack's toggle) can
// skip this click, and spends itself. Any other click re-arms, and an
// unresolvable Ref never fires. The zero value is ready; keep one per
// surface (tables, rack, Show Info links key it by Ref, which is what
// made two rows of the same item/caste activate together).
type RowDouble struct {
	ref   InfoRef
	at    time.Time
	Armed bool
}

// Gesture runs one left click through the same timer for a surface whose
// second click means something other than Show Info (the fitting hangar's
// Fit verb): fire runs when a repeat click on the same resolvable Ref
// lands inside the window. Show Info never fires from here.
func (d *RowDouble) Gesture(cv ContentView, spec RowSpec, fire func()) bool {
	now := time.Now()
	ref, ok := InfoRef{}, false
	if spec.Ref != nil && InfoRefExists(cv, *spec.Ref) {
		ref, ok = *spec.Ref, true
	}
	hit := ok && d.Armed && d.ref == ref && now.Sub(d.at) < doubleClickWindow
	d.ref, d.at, d.Armed = ref, now, true
	if hit {
		d.Armed = false
		fire()
	}
	return hit
}

// Click runs one left click through the double-click rule (see
// RowDouble): true = the gesture completed and Show Info opened.
func (d *RowDouble) Click(cv ContentView, as ActionSink, spec RowSpec) bool {
	now := time.Now()
	ref, fire := InfoRef{}, false
	if spec.Ref != nil && InfoRefExists(cv, *spec.Ref) {
		ref = *spec.Ref
		fire = d.Armed && d.ref == ref && now.Sub(d.at) < doubleClickWindow
	}
	d.ref, d.at, d.Armed = ref, now, true
	if fire {
		as.Do(ShowInfo{Ref: ref})
		d.Armed = false // the gesture is spent
	}
	return fire
}
