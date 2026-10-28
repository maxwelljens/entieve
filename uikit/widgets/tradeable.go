package widgets

import (
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/uikit/theme"
)

// Tradeable is one resolved marketable id: what it is (Kind is valid only
// when Found), how it reads and draws, and whether a content def backs it.
// ResolveTradeable is the one ladder every surface names and draws an id
// through; the icon delegates to the theme chain, so the player's drop-in art
// wins in every slot.
type Tradeable struct {
	ID    content.ItemID
	Kind  InfoKind
	Name  string
	Icon  string
	Found bool
}

// Ref is the Show Info reference for t; false when no def backs the id.
func (t Tradeable) Ref() (InfoRef, bool) {
	if !t.Found {
		return InfoRef{}, false
	}
	return InfoRef{Kind: t.Kind, ID: string(t.ID)}, true
}

// ResolveTradeable resolves any marketable id against c in the sim's itemName
// order: item, then module, then hull. Ids are unique per kind, so the order
// only decides colliding or stale data (the market used to check module
// before item — drift, now one ladder). An unknown id reads as itself, with
// no icon and no ref.
func ResolveTradeable(c *content.Content, id content.ItemID) Tradeable {
	t := Tradeable{ID: id, Name: string(id)}
	if c == nil || id == "" {
		return t
	}
	if def, ok := c.Item(id); ok {
		t.Kind, t.Name, t.Icon, t.Found = InfoItem, def.Name, theme.ItemIconFor(def), true
		return t
	}
	if def, ok := c.Module(content.ModuleID(id)); ok {
		t.Kind, t.Name, t.Icon, t.Found = InfoModule, def.Name, theme.ModuleIconFor(def), true
		return t
	}
	if def, ok := c.Ship(content.ShipID(id)); ok {
		t.Kind, t.Name, t.Icon, t.Found = InfoShip, def.Name, theme.HullIconFor(def), true
		return t
	}
	return t
}

// TradeableName is the display name of any marketable id — items, modules and
// hulls resolve exactly as the sim's own itemName does.
func TradeableName(c *content.Content, id content.ItemID) string {
	return ResolveTradeable(c, id).Name
}

// TradeableIcon is the semantic icon name of any marketable id ("" when no
// def backs it).
func TradeableIcon(c *content.Content, id content.ItemID) string {
	return ResolveTradeable(c, id).Icon
}
