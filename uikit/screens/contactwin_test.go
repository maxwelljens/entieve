package screens

import (
	"image"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui"
	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/content"
	"github.com/entieve/entieve/internal/uitest"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/sim"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
)

// The Details dialog's own tests: one offer rendered whole — the contact's
// face (drop-in art or the fallback glyph), their words, the contract and
// the three verbs — over the seams alone, with an empty art registry unless
// a test drops a portrait in.

// detailsPanel mounts the dialog on a bare UI over the stub view and a
// recording sink.
func detailsPanel(t *testing.T, cv *stubView) (*ContactWin, *recordingSink, *ebitenui.UI) {
	t.Helper()
	rec := &recordingSink{}
	env := widgets.NewSeams(theme.NewSeam(theme.DefaultPalette(), theme.DefaultTypeScale()), cv, rec, nil)
	w := NewContactWin(env)
	return w, rec, newWindowUI(t, env, w.Win())
}

// pumpDetails lays the dialog out at a 1024×700 viewport.
func pumpDetails(t *testing.T, ui *ebitenui.UI, w *ContactWin) {
	t.Helper()
	uitest.PumpWindow(t, ui, w.Sync, 1024, 700)
}

// shippedOffer is one live offer of the shipped content, its prose already
// filled (the sim does that in production).
func shippedOffer(t *testing.T, c *content.Content, id content.ContractID, prose string, stock int) sim.Offer {
	t.Helper()
	def, ok := c.Contract(id)
	if !ok {
		t.Fatalf("missing offer %s", id)
	}
	cp := *def
	cp.Description = prose
	return sim.Offer{Def: &cp, ContactName: "Dukšan Yard Office", Patron: content.PatronCorporation,
		MinStanding: cp.MinStanding, Prose: prose, Stock: stock}
}

// TestContactDetailsRendersOffer: the page carries the contact, their
// spoken line, the objective, the payout and the live stock, and it opens
// with the fallback glyph when no portrait is dropped (the empty asset
// registry is the released-binary case).
func TestContactDetailsRendersOffer(t *testing.T) {
	render.LoadAssets("") // no drop-ins: the portrait falls back to the glyph
	defer render.LoadAssets("")

	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	o := shippedOffer(t, c, "offer-duksan-shield",
		"Hauvnok's fitters are short 4 units of Shield Booster.", 7)
	cv := &stubView{c: c, docked: true}
	cv.contacts = widgets.Contacts{Station: "cel-haven-rest", Standing: 0.5, Offers: []sim.Offer{o}}
	w, _, ui := detailsPanel(t, cv)

	if !w.Show("offer-duksan-shield") {
		t.Fatal("Show refused a live offer")
	}
	pumpDetails(t, ui, w)
	if !w.IsOpen() {
		t.Fatal("dialog did not open")
	}
	if got := w.TitleText(); got != "Contact" {
		t.Errorf("title = %q, want the furniture label Contact", got)
	}
	if got, want := w.name.Label, o.ContactName; got != want {
		t.Errorf("contact name = %q, want %q", got, want)
	}
	if got := w.speech.Label; got != o.Prose {
		t.Errorf("speech = %q, want the offer's filled prose %q", got, o.Prose)
	}
	if got, want := w.attr.Label, "By "+o.ContactName; got != want {
		t.Errorf("attribution = %q, want %q", got, want)
	}
	if got := w.objective.Label; !strings.Contains(got, "Shield Booster") || !strings.Contains(got, "4") {
		t.Errorf("objective = %q, want the item and its count", got)
	}
	if got := w.reward.Label; !strings.Contains(got, theme.FormatCredits(o.Def.Reward.Credits)) {
		t.Errorf("reward = %q, want the payout", got)
	}
	if got := w.destText.Label; !strings.Contains(got, "in stock 7") {
		t.Errorf("destination line = %q, want the live stock", got)
	}
	if w.accept.Text().Label != "Accept work" || w.accept.GetWidget().Disabled {
		t.Errorf("accept verb = %q disabled=%v, want an enabled Accept work",
			w.accept.Text().Label, w.accept.GetWidget().Disabled)
	}
	// The fallback portrait: the glyph is up, the drop-in graphic is not.
	if got := w.port.glyph.Name(); got != portraitFallback {
		t.Errorf("fallback glyph = %q, want %q", got, portraitFallback)
	}
	if w.port.gfx.GetWidget().IsVisible() {
		t.Error("the drop-in graphic shows with an empty art registry")
	}
	if b := w.port.Widget().GetWidget().Rect; b.Dx()*3 != b.Dy()*2 {
		t.Errorf("portrait box = %v, want 2:3", b)
	}
}

// TestContactDetailsDroppedPortrait: a JPEG dropped at
// assets/portraits/<contact-id>.jpg replaces the glyph in the same 2:3 box —
// kind, directory and shipped extension pinned to the owner's drop-in set.
func TestContactDetailsDroppedPortrait(t *testing.T) {
	dir := t.TempDir()
	const id = content.ContactID("contact-duksan-yard")
	if err := os.MkdirAll(filepath.Join(dir, "portraits"), 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 60, 90)) // 2:3
	for i := range img.Pix {
		img.Pix[i] = 0xC0
	}
	f, err := os.Create(filepath.Join(dir, "portraits", string(id)+".jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
	f.Close()
	render.LoadAssets(dir)
	defer render.LoadAssets("")

	if render.ContactPortrait(id) == nil {
		t.Fatalf("no portrait registered for %s", id)
	}
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	o := shippedOffer(t, c, "offer-duksan-shield", "Prose.", 0)
	cv := &stubView{c: c, docked: true}
	cv.contacts = widgets.Contacts{Station: "cel-haven-rest", Offers: []sim.Offer{o}}
	w, _, ui := detailsPanel(t, cv)
	w.Show("offer-duksan-shield")
	pumpDetails(t, ui, w)

	if !w.port.dropped || !w.port.gfx.GetWidget().IsVisible() {
		t.Error("the dropped portrait never became visible")
	}
	if got := w.port.glyph.Name(); got != "" {
		t.Errorf("fallback glyph %q still set beside the dropped art", got)
	}
}

// TestContactDetailsAcceptEmitsCommand: the accept verb fires the very
// widgets.Accept intent the board's old button did — one path to
// CmdAcceptMission.
func TestContactDetailsAcceptEmitsCommand(t *testing.T) {
	c, defs := contactContent()
	cv := &stubView{c: c, docked: true}
	cv.contacts = widgets.Contacts{Station: "cel-stub", Offers: offersFor(defs, "ct-a-open")}
	w, rec, ui := detailsPanel(t, cv)
	w.Show("ct-a-open")
	pumpDetails(t, ui, w)

	pressButton(t, ui, w.accept)
	if len(rec.got) != 1 {
		t.Fatalf("accept emitted %#v, want one intent", rec.got)
	}
	if a, ok := rec.got[0].(widgets.Accept); !ok || a.Contract != "ct-a-open" {
		t.Fatalf("intent = %#v, want Accept{ct-a-open}", rec.got[0])
	}
}

// TestContactDetailsGate: a standing-gated offer states its requirement and
// disables the accept; the row's Details stays the way in. Once accepted the
// verb reads Accepted and stays disabled, and the standing line says where
// the work is claimed.
func TestContactDetailsGate(t *testing.T) {
	c, defs := contactContent()
	cv := &stubView{c: c, docked: true}
	cv.contacts = widgets.Contacts{Station: "cel-stub", Standing: 0.1, Offers: offersFor(defs, "ct-c-gated")}
	w, rec, ui := detailsPanel(t, cv)
	if !w.Show("ct-c-gated") {
		t.Fatal("Show refused the gated offer")
	}
	pumpDetails(t, ui, w)

	if !w.accept.GetWidget().Disabled {
		t.Fatal("gated offer's accept is enabled")
	}
	if got := w.gateText.Label; got != "" {
		t.Errorf("gate line = %q, want the meter (no sentence)", got)
	}
	if frac, shown := w.GateFrac(); !shown || math.Abs(frac-0.2) > 1e-9 {
		t.Errorf("gate meter = %v shown=%v, want standing 0.1 / 0.5", frac, shown)
	}
	before := len(rec.got)
	w.accept.ClickedEvent.Fire(&widget.ButtonClickedEventArgs{Button: w.accept})
	ui.Update()
	if len(rec.got) != before {
		t.Fatalf("gated accept emitted %#v, want nothing", rec.got[before:])
	}

	// Accepted: the sim now lists it as active work.
	cv.contacts = widgets.Contacts{
		Station: "cel-stub",
		Active:  []sim.MissionState{{ID: "ct-c-gated", Def: defs["ct-c-gated"]}},
	}
	pumpDetails(t, ui, w)
	if got := w.accept.Text().Label; got != "Accepted" {
		t.Errorf("accepted verb = %q, want Accepted", got)
	}
	if !w.accept.GetWidget().Disabled {
		t.Error("accepted verb is enabled")
	}
	if got := w.gateText.Label; !strings.HasPrefix(got, "Accepted:") {
		t.Errorf("accepted standing line = %q, want the claim note", got)
	}
}

// TestContactDetailsInspectEmitsShowInfo: the Inspect verb opens the
// objective's Show Info page through the shared seam — the item for a
// delivery, the caste for a kill.
func TestContactDetailsInspectEmitsShowInfo(t *testing.T) {
	c, defs := contactContent()
	cv := &stubView{c: c, docked: true}
	cv.contacts = widgets.Contacts{Station: "cel-stub", Offers: offersFor(defs, "ct-a-open")}
	w, rec, ui := detailsPanel(t, cv)
	w.Show("ct-a-open")
	pumpDetails(t, ui, w)

	if !w.inspect.GetWidget().IsVisible() {
		t.Fatal("Inspect hidden on an offer with an inspectable objective")
	}
	if got, want := w.inspect.Text().Label, "Inspect item"; got != want {
		t.Errorf("inspect label = %q, want %q", got, want)
	}
	pressButton(t, ui, w.inspect)
	if len(rec.got) != 1 {
		t.Fatalf("inspect emitted %#v, want one intent", rec.got)
	}
	if v, ok := rec.got[0].(widgets.ShowInfo); !ok || v.Ref != widgets.ItemRef("item-stub-ore") {
		t.Fatalf("intent = %#v, want ShowInfo{item-stub-ore}", rec.got[0])
	}
}

// TestContactDetailsDeclineCloses: Decline hides the dialog and emits
// nothing — declining has no engine verb.
func TestContactDetailsDeclineCloses(t *testing.T) {
	c, defs := contactContent()
	cv := &stubView{c: c, docked: true}
	cv.contacts = widgets.Contacts{Station: "cel-stub", Offers: offersFor(defs, "ct-a-open")}
	w, rec, ui := detailsPanel(t, cv)
	w.Show("ct-a-open")
	pumpDetails(t, ui, w)

	pressButton(t, ui, w.decline)
	if w.IsOpen() {
		t.Fatal("Decline left the dialog open")
	}
	if len(rec.got) != 0 {
		t.Fatalf("Decline emitted %#v, want nothing", rec.got)
	}
}

// TestContactDetailsRetargets: one instance re-targets on a second offer; an
// offer that is no longer live (its gap closed) opens nothing and leaves the
// page it had.
func TestContactDetailsRetargets(t *testing.T) {
	c, defs := contactContent()
	cv := &stubView{c: c, docked: true}
	cv.contacts = widgets.Contacts{Station: "cel-stub", Offers: offersFor(defs, "ct-a-open", "ct-b-open")}
	w, _, ui := detailsPanel(t, cv)
	w.Show("ct-a-open")
	pumpDetails(t, ui, w)
	if got, want := w.name.Label, "Stub Office"; got != want {
		t.Errorf("first page name = %q, want %q", got, want)
	}

	if !w.Show("ct-b-open") {
		t.Fatal("re-target refused a live offer")
	}
	pumpDetails(t, ui, w)
	if got := w.speech.Label; !strings.Contains(got, defs["ct-b-open"].Title) && !strings.Contains(got, "Prose for") {
		t.Errorf("re-targeted speech = %q", got)
	}
	if got := w.Offer(); got != "ct-b-open" {
		t.Errorf("offer = %q, want the re-target", got)
	}

	before := w.name.Label
	if w.Show("ct-missing") {
		t.Fatal("Show accepted an offer that is not live")
	}
	if got := w.name.Label; got != before {
		t.Errorf("a refused show changed the page to %q", got)
	}
}

// TestContactDetailsFitsContent: the window's declared floor holds every
// shipped offer's page — the portrait row, the briefing, the contract and
// the verbs — so no authored prose is ever clipped inside the dialog.
func TestContactDetailsFitsContent(t *testing.T) {
	render.LoadAssets("")
	defer render.LoadAssets("")
	c, err := content.Load(content.FS())
	if err != nil {
		t.Fatalf("content.Load: %v", err)
	}
	ids := make([]content.ContractID, 0, len(c.Contracts))
	for id := range c.Contracts {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	cv := &stubView{c: c, docked: true}
	w, _, ui := detailsPanel(t, cv)
	for _, id := range ids {
		o := shippedOffer(t, c, id, c.Contracts[id].Description, 12)
		cv.contacts = widgets.Contacts{Station: "cel-haven-rest", Standing: 0.5, Offers: []sim.Offer{o}}
		if !w.Show(id) {
			t.Fatalf("%s: Show refused a shipped offer", id)
		}
		pumpDetails(t, ui, w)
		bw, bh := w.body.PreferredSize()
		if bw > theme.Px(contactW) {
			t.Errorf("%s: body prefers %d px wide, floor %d", id, bw, theme.Px(contactW))
		}
		if bh > theme.Px(contactH)-theme.Px(theme.TitleBarH) {
			t.Errorf("%s: body prefers %d px tall, floor %d (window %d minus title %d)",
				id, bh, theme.Px(contactH)-theme.Px(theme.TitleBarH), theme.Px(contactH), theme.Px(theme.TitleBarH))
		}
	}
}

// TestContactDetailsDropsPatronAndPostedAt: the page's left column is the
// face and the name. The patron ("Corporation"/"Charter") and the posted-at
// line are gone from every text the dialog renders, and no other contact
// surface states them either (the board row never did).
func TestContactDetailsDropsPatronAndPostedAt(t *testing.T) {
	c, defs := contactContent()
	cv := &stubView{c: c, docked: true, station: widgets.StationInfo{Name: "Haven Rest", System: "Solace"}}
	cv.contacts = widgets.Contacts{Station: "cel-stub", Standing: 0.5, Offers: offersFor(defs, "ct-a-open")}
	w, _, ui := detailsPanel(t, cv)
	if !w.Show("ct-a-open") {
		t.Fatal("Show refused the offer")
	}
	pumpDetails(t, ui, w)
	if got := w.name.Label; got != "Stub Office" {
		t.Errorf("contact name = %q, want it kept", got)
	}
	var texts []string
	collectTexts(w.win.GetContainer(), &texts)
	for _, s := range texts {
		for _, gone := range []string{"Posted at", "Charter", "Corporation"} {
			if strings.Contains(s, gone) {
				t.Errorf("dialog still states %q: %q", gone, s)
			}
		}
	}
}

// TestContactDetailsItemisedRewards: the CONTRACT block itemises the reward —
// one row and one glyph per reward: coins on the payout, the standing mark on
// the gain, the material's own icon on the item. Rows vanish with their
// reward, so a money-only offer shows one.
func TestContactDetailsItemisedRewards(t *testing.T) {
	c, defs := contactContent()
	at := func(reward content.ContractReward) (*ContactWin, *stubView) {
		t.Helper()
		def := *defs["ct-a-open"]
		def.Reward = reward
		cv := &stubView{c: c, docked: true}
		cv.contacts = widgets.Contacts{Station: "cel-stub", Standing: 0.5, Offers: []sim.Offer{{
			Def: &def, ContactName: "Dukšan Yard Office", Patron: content.PatronCharter,
		}}}
		w, _, ui := detailsPanel(t, cv)
		if !w.Show("ct-a-open") {
			t.Fatal("Show refused the offer")
		}
		pumpDetails(t, ui, w)
		return w, cv
	}

	// All three rewards: three rows, each with its icon.
	w, _ := at(content.ContractReward{Credits: 4200, StandingGain: 0.25, Item: "item-stub-ore"})
	visible := func(c *widget.Container) bool { return c.GetWidget().IsVisible() }
	if !visible(w.rewardRow) || !visible(w.gainRow) || !visible(w.itemRow) {
		t.Fatal("an offer paying all three rewards hides one of its rows")
	}
	if got, want := w.reward.Label, theme.FormatCredits(4200); got != want {
		t.Errorf("payout = %q, want %q", got, want)
	}
	if got := w.rewardMark.Name(); got != rewardMoneyIcon {
		t.Errorf("payout icon = %q, want the coins %q", got, rewardMoneyIcon)
	}
	if got, want := w.gain.Label, "+0.25 standing"; got != want {
		t.Errorf("gain = %q, want %q", got, want)
	}
	if got := w.gainMark.Name(); got != rewardStandingIcon {
		t.Errorf("gain icon = %q, want the standing mark %q", got, rewardStandingIcon)
	}
	if got, want := w.item.Label, "Stub Ore"; got != want {
		t.Errorf("material = %q, want %q", got, want)
	}
	if got, want := w.itemMark.Name(), widgets.TradeableIcon(c, "item-stub-ore"); got != want || got == "" {
		t.Errorf("material icon = %q, want the item's own %q", got, want)
	}

	// Money only: one row.
	w, _ = at(content.ContractReward{Credits: 4200})
	if !visible(w.rewardRow) || visible(w.gainRow) || visible(w.itemRow) {
		t.Error("a money-only offer does not itemise to one reward row")
	}

	// Money and standing: two rows.
	w, _ = at(content.ContractReward{Credits: 4200, StandingGain: 0.1})
	if !visible(w.rewardRow) || !visible(w.gainRow) || visible(w.itemRow) {
		t.Error("an offer without a material shows both paid rows and no item row")
	}
}

// TestContactDetailsGateMeterStates: the Standing row speaks in one meter,
// never a sentence — locked shows the partial strip in the warn role, met
// shows it full, an offer with no requirement keeps no row, and an accepted
// offer swaps the meter for its claim line.
func TestContactDetailsGateMeterStates(t *testing.T) {
	c, defs := contactContent()
	show := func(id content.ContractID, standing float64, active bool) (*ContactWin, *stubView) {
		t.Helper()
		cv := &stubView{c: c, docked: true}
		cv.contacts = widgets.Contacts{Station: "cel-stub", Standing: standing, Offers: offersFor(defs, id)}
		if active {
			cv.contacts.Active = []sim.MissionState{{ID: id, Def: defs[id]}}
		}
		w, _, ui := detailsPanel(t, cv)
		if !w.Show(id) {
			t.Fatalf("Show refused %s", id)
		}
		pumpDetails(t, ui, w)
		return w, cv
	}

	w, _ := show("ct-c-gated", 0.1, false) // 0.10 of 0.50
	if frac, shown := w.GateFrac(); !shown || math.Abs(frac-0.2) > 1e-9 {
		t.Errorf("locked gate = %v shown=%v, want 0.2", frac, shown)
	}
	if !w.gateGfx.GetWidget().IsVisible() || w.gateText.GetWidget().IsVisible() {
		t.Error("locked gate shows text instead of the meter")
	}
	if w.accept.GetWidget().Disabled != true {
		t.Error("locked gate leaves Accept enabled")
	}
	var texts []string
	collectTexts(w.win.GetContainer(), &texts)
	for _, line := range texts {
		if strings.Contains(line, "Requires standing") {
			t.Errorf("the page still states the gate as text: %q", line)
		}
	}

	w, _ = show("ct-c-gated", 0.5, false)
	if frac, shown := w.GateFrac(); !shown || frac != 1 {
		t.Errorf("met gate = %v shown=%v, want a full meter", frac, shown)
	}
	if w.accept.GetWidget().Disabled {
		t.Error("met gate leaves Accept disabled")
	}

	w, _ = show("ct-a-open", 0, false) // no requirement
	if _, shown := w.GateFrac(); shown {
		t.Error("an offer with no requirement draws a gate meter")
	}
	if w.gate.GetWidget().IsVisible() {
		t.Error("an offer with no requirement keeps its Standing row")
	}

	w, _ = show("ct-c-gated", 0.5, true)
	if _, shown := w.GateFrac(); shown {
		t.Error("an accepted offer still draws the gate meter")
	}
	if got := w.GateText(); !strings.HasPrefix(got, "Accepted:") {
		t.Errorf("accepted standing row = %q, want the claim line", got)
	}
}
