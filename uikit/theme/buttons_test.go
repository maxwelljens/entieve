package theme

import (
	"image/color"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
)

// hexState prints one button state's body/edge pair as #rrggbbaa (always
// eight digits, so an opaque value reads the same shape as a tinted one).
func hexState(s buttonState) (body, edge string) {
	hex := func(c color.NRGBA) string {
		return "#" + hexByte(uint32(c.R)) + hexByte(uint32(c.G)) + hexByte(uint32(c.B)) + hexByte(uint32(c.A))
	}
	return hex(s.body), hex(s.edge)
}

// statesGolden compares five states against a golden table in the order the
// image is built (idle, hover, pressed, pressed-hover, disabled).
func statesGolden(t *testing.T, name string, got [5]buttonState, want [5][2]string) {
	t.Helper()
	for i, w := range want {
		body, edge := hexState(got[i])
		if body != w[0] || edge != w[1] {
			t.Errorf("%s state %d = %s/%s, want %s/%s", name, i, body, edge, w[0], w[1])
		}
	}
}

// TestHouseButtonStatesGolden pins the one house control recipe. The values
// are the literals the theme, rack, rail, services, fitting and table
// recipes each carried before they were consolidated here: an edit to this
// table is a visible chrome change (hover tint, disabled alpha), so it must
// be deliberate.
func TestHouseButtonStatesGolden(t *testing.T) {
	p := DefaultPalette()
	statesGolden(t, "HouseButton", houseStates(p), [5][2]string{
		{"#141a24ff", "#3a4452dc"}, // idle: cell fill, border
		{"#ffb45412", "#ffb454c8"}, // hover: accent tint
		{"#ffb4542d", "#3a4452dc"}, // pressed: deeper accent
		{"#ffb4542d", "#ffb454c8"}, // pressed-hover
		{"#141a248c", "#3a44526e"}, // disabled: dimmed cell + border
	})
}

// TestCellImageStatesGolden pins the one cell recipe in both variants. The
// highlighted idle (accent glow) is the rack's running module, the rail's
// open window and the fitting grid's fitted slot; the quiet idle is the
// rail's closed entry and the services strip. Every other state is shared.
func TestCellImageStatesGolden(t *testing.T) {
	p := DefaultPalette()
	shared := [5][2]string{
		{"", ""},
		{"#ffb4545f", "#ffb454ff"}, // hover
		{"#ffb4546e", "#ffb454dc"}, // pressed
		{"#ffb4546e", "#ffb454ff"}, // pressed-hover
		{"#141a248c", "#3a44526e"}, // disabled
	}
	on := shared
	on[0] = [2]string{"#ffb4543c", "#ffb454ff"}
	statesGolden(t, "CellImage highlighted", cellStates(p, true), on)
	off := shared
	off[0] = [2]string{"#0a0e14d2", "#3a4452c8"}
	statesGolden(t, "CellImage quiet", cellStates(p, false), off)
}

// TestButtonConstructorsFillEveryState: the constructors populate all five
// ebitenui states (a nil state is a transparent widget).
func TestButtonConstructorsFillEveryState(t *testing.T) {
	p := DefaultPalette()
	for _, c := range []struct {
		name string
		bi   *widget.ButtonImage
	}{
		{"HouseButton", HouseButton(p)},
		{"CellImage highlighted", CellImage(p, true)},
		{"CellImage quiet", CellImage(p, false)},
	} {
		if c.bi.Idle == nil || c.bi.Hover == nil || c.bi.Pressed == nil ||
			c.bi.PressedHover == nil || c.bi.Disabled == nil {
			t.Errorf("%s: a state is nil", c.name)
		}
	}
}
