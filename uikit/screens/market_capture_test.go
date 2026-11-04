package screens

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"github.com/entieve/entieve/internal/capture"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/hajimehoshi/ebiten/v2"
)

// marketCaptureFrames is the first fully painted frame (the open fade is
// 12) plus the frames the selection gesture settles over.
const marketCaptureFrames = 16

// TestMarketCaptureFrame renders the market window docked at a station
// with an item selected, through a real ebiten loop, and pins what the
// headless suite cannot read back: the right pane carries the item's
// order book over My orders, both in the one scroll column with the house
// rail, and the book's rows really draw (glyph pixels inside the book
// section). The PNG lands in $ENTIEVE_CAPTURE_DIR (default /tmp/opencode).
//
// Skipped unless ENTIEVE_CAPTURE=1; one ebiten loop per process — run it
// on its own:
//
//	ENTIEVE_CAPTURE=1 go test ./uikit/screens/ -run TestMarketCaptureFrame
func TestMarketCaptureFrame(t *testing.T) {
	const station = "cel-haven-rest" // Jalvrek, sys-solace's market station
	env, _ := newTestEnv(t)
	env.Sim.Docked.Set(env.Sim.Player(), sim.Docked{At: station})
	m := NewMarket(env)
	ui := newWindowUI(t, env, m.Win())

	// The fixture's book at this station: the first catalog item resting
	// on BOTH sides, so the capture shows asks and bids.
	pick := func() int {
		for i, r := range m.tbl.Rows {
			it, ok := r.Data.(mktItemRow)
			if !ok {
				continue
			}
			ask, bid := false, false
			for _, o := range env.Sim.BookOrders(station) {
				if o.Item != it.Item {
					continue
				}
				if o.Side == sim.OrderSell {
					ask = true
				} else {
					bid = true
				}
			}
			if ask && bid {
				return i
			}
		}
		return -1
	}

	var picked bool
	out := capture.Out("market_capture.png")
	(&capture.Capture{
		Name:   "Entieve market capture",
		Frames: marketCaptureFrames,
		Update: func(n int) error {
			m.Sync()
			ui.Update()
			if !picked && n >= 3 {
				row := pick()
				if row < 0 {
					return fmt.Errorf("fixture: no %s item rests on both sides of %s", "catalog", station)
				}
				clickRow(ui, m.tbl, row)
				picked = true
			}
			return nil
		},
		Draw: func(_ int, img *ebiten.Image) error {
			img.Fill(color.White)
			ui.Draw(img)
			return nil
		},
		Verify: func(img *ebiten.Image) error {
			var errs []string
			fail := func(format string, args ...any) {
				errs = append(errs, fmt.Sprintf(format, args...))
			}

			item := m.cv.TradeableName(m.sel)
			if !m.hasSel || item == "" {
				fail("no item selected (hasSel=%v)", m.hasSel)
			}
			if got, want := m.bookLbl.Label, "ORDER BOOK"; got != want {
				fail("book bar = %q, want %q", got, want)
			}
			// The head leads the pane: the item's hero icon and its
			// description above the book.
			if b := m.headIcon.Widget().Image.Bounds(); b.Dx() != theme.Px(theme.IconHero) {
				fail("head icon = %v, want the %d px item hero", b, theme.Px(theme.IconHero))
			}
			if m.headProse.Label == "" || m.headProse.GetWidget().Rect.Empty() {
				fail("head description missing at %v", m.headProse.GetWidget().Rect)
			}
			asks, bids := 0, 0
			for _, r := range m.book {
				switch r.Cells[0] {
				case "Sell":
					asks++
				case "Buy":
					bids++
				default:
					fail("book row side = %q, want Sell or Buy", r.Cells[0])
				}
			}
			if asks == 0 || bids == 0 {
				fail("%s's book shows %d sell orders / %d buy orders, want both sides", item, asks, bids)
			}
			if got, want := len(m.bookTbl.RowsForTest()), len(m.book); got != want {
				fail("book rows rendered = %d, want %d", got, want)
			}

			// Both sections in the one scroll column, book first, and the
			// book's glyphs really on screen.
			book := m.bookTbl.Container().GetWidget().Rect
			mine := m.mineBox.GetWidget().Rect
			if book.Max.Y > mine.Min.Y {
				fail("book section %v overlaps My orders %v", book, mine)
			}
			pane := m.sideRail.Container().GetWidget().Rect
			if !pane.In(m.Win().GetContainer().GetWidget().Rect) {
				fail("right pane %v outside the window", pane)
			}
			if len(m.book) > 0 {
				if n := uitest.InkIn(img, m.bookTbl.RowWidgetForTest(0).GetWidget().Rect); n == 0 {
					fail("book row 1 drew no glyph pixels at %v", m.bookTbl.RowWidgetForTest(0).GetWidget().Rect)
				}
			}
			if n := uitest.InkIn(img, m.bookLbl.GetWidget().Rect); n == 0 {
				fail("book section bar drew no glyph pixels at %v", m.bookLbl.GetWidget().Rect)
			}

			if len(errs) > 0 {
				return fmt.Errorf("capture %s: %s", out, strings.Join(errs, "; "))
			}
			return nil
		},
		Out: out,
	}).Run(t)
}
