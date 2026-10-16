package widgets

import (
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/ecs"
	"github.com/entieve/entieve/sim"
)

// Show Info reference identity and resolution (the row-data half of the
// seam): InfoRef names one definition, the resolvers below turn row
// payloads and grid entities into one. Screens render from these; the
// info window consumes them.

// infoRefPayload is the resolver's extension point for row payloads
// whose ref needs the surface's own knowledge (the factory's job rows,
// the overview's entity rows, the fitting's slot picks): each screen
// implements InfoRefOf on its own payload type, so this package — and
// the seam it serves — never imports a screen.
type infoRefPayload interface {
	InfoRefOf(cv ContentView) (InfoRef, bool)
}

// InfoKind is the definition family an info window shows.
type InfoKind uint8

// The four Show Info kinds (the content defs behind them).
const (
	InfoShip InfoKind = iota
	InfoModule
	InfoItem
	InfoNpc
)

// InfoRef names one definition for Show Info. The zero ref is invalid;
// ShipRef/ModuleRef/ItemRef/NpcRef are the constructors, InfoRefOf and
// InfoRefFor resolve row payloads and grid entities to one.
type InfoRef struct {
	Kind InfoKind
	ID   string
}

// ShipRef/ModuleRef/ItemRef/NpcRef build the ref for one content def.
func ShipRef(id content.ShipID) InfoRef     { return InfoRef{Kind: InfoShip, ID: string(id)} }
func ModuleRef(id content.ModuleID) InfoRef { return InfoRef{Kind: InfoModule, ID: string(id)} }
func ItemRef(id content.ItemID) InfoRef     { return InfoRef{Kind: InfoItem, ID: string(id)} }
func NpcRef(id content.NpcID) InfoRef       { return InfoRef{Kind: InfoNpc, ID: string(id)} }

// Valid reports whether the ref carries an id at all (kind resolution is
// the Content's job).
func (r InfoRef) Valid() bool { return r.ID != "" }

// String renders the ref for logs and tests ("module/mod-emp-turret").
func (r InfoRef) String() string {
	var kind string
	switch r.Kind {
	case InfoShip:
		kind = "ship"
	case InfoModule:
		kind = "module"
	case InfoItem:
		kind = "item"
	case InfoNpc:
		kind = "npc"
	}
	return kind + "/" + r.ID
}

// ---------------------------------------------------------------------------
// Reference resolution
// ---------------------------------------------------------------------------

// InfoRefFor resolves a grid entity to the definition its window shows:
// an NPC caste first (the dossier belongs to the caste, not its hull),
// then the hull, then the asteroid's ore. False for celestials, wrecks
// and anything else without a def.
func InfoRefFor(cv ContentView, entity ecs.Entity) (InfoRef, bool) {
	if cv == nil || cv.Defs() == nil {
		return InfoRef{}, false
	}
	if ai, ok := cv.NpcAI(entity); ok && ai.Npc != "" {
		if ref := NpcRef(ai.Npc); InfoRefExists(cv, ref) {
			return ref, true
		}
	}
	if sh, ok := cv.ShipState(entity); ok && sh.Hull != "" {
		if ref := ShipRef(sh.Hull); InfoRefExists(cv, ref) {
			return ref, true
		}
	}
	if a, ok := cv.Asteroid(entity); ok && a.Ore != "" {
		if ref := ItemRef(a.Ore); InfoRefExists(cv, ref) {
			return ref, true
		}
	}
	return InfoRef{}, false
}

// InfoRefOf resolves a row payload to a Show Info ref. The table surfaces
// hand it whatever their TableRow.Data carries (see market.itemLabel for
// the same multi-kind resolution); false means the row has nothing to
// show and its Show Info entry stays disabled.
func InfoRefOf(cv ContentView, data any) (InfoRef, bool) {
	switch v := data.(type) {
	case InfoRef:
		return v, InfoRefExists(cv, v)
	case infoRefPayload: // screen row payloads answer for themselves
		return v.InfoRefOf(cv)
	case content.ItemID:
		return RefOfID(cv, string(v))
	case content.ModuleID:
		return RefOfID(cv, string(v))
	case content.ShipID:
		return RefOfID(cv, string(v))
	case ecs.Entity:
		return InfoRefFor(cv, v)
	}
	return InfoRef{}, false
}

// refOfID resolves a raw id the way itemLabel does: item, then module,
// then ship (holds can store any of them under one id type).
func RefOfID(cv ContentView, id string) (InfoRef, bool) {
	if cv.Defs() == nil || id == "" {
		return InfoRef{}, false
	}
	if _, ok := cv.Defs().Item(content.ItemID(id)); ok {
		return ItemRef(content.ItemID(id)), true
	}
	if _, ok := cv.Defs().Module(content.ModuleID(id)); ok {
		return ModuleRef(content.ModuleID(id)), true
	}
	if _, ok := cv.Defs().Ship(content.ShipID(id)); ok {
		return ShipRef(content.ShipID(id)), true
	}
	return InfoRef{}, false
}

// infoRefExists reports whether ref's definition is loaded.
func InfoRefExists(cv ContentView, ref InfoRef) bool {
	if cv.Defs() == nil || !ref.Valid() {
		return false
	}
	switch ref.Kind {
	case InfoShip:
		_, ok := cv.Defs().Ship(content.ShipID(ref.ID))
		return ok
	case InfoModule:
		_, ok := cv.Defs().Module(content.ModuleID(ref.ID))
		return ok
	case InfoItem:
		_, ok := cv.Defs().Item(content.ItemID(ref.ID))
		return ok
	case InfoNpc:
		_, ok := cv.Defs().Npc(content.NpcID(ref.ID))
		return ok
	}
	return false
}

// chainName resolves a raw def id (item, module or ship) for display.
func ChainName(cv ContentView, id content.ItemID) string {
	if d, ok := cv.Defs().Item(id); ok {
		return d.Name
	}
	if d, ok := cv.Defs().Module(content.ModuleID(id)); ok {
		return d.Name
	}
	if d, ok := cv.Defs().Ship(content.ShipID(id)); ok {
		return d.Name
	}
	return string(id)
}

// SlotPick is the fitting slot table's row payload: the slot reference
// plus the fitted module ID ("" = the slot is empty).
type SlotPick struct {
	Ref sim.SlotRef
	Mod content.ModuleID
}

// InfoRefOf resolves the slot pick inside the seam's Show Info resolver
// (infoRefPayload): an empty slot shows nothing.
func (s SlotPick) InfoRefOf(cv ContentView) (InfoRef, bool) {
	if s.Mod == "" {
		return InfoRef{}, false
	}
	return RefOfID(cv, string(s.Mod))
}
