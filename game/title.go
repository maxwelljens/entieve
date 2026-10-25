package game

// The title phase: a flag-gated boot state (Options.StartInTitle, set by
// cmd run only when --load is absent). The scene behind it is the embedded
// artwork plate, cover-fit under a light veil (the already-built space
// scene's static render is the fallback), while the sim stays frozen and
// gameplay chrome stays unmounted. The title owns a menu on the kit's
// root overlay, a save manager and credits window, and a second Screen
// host for the title-scoped rows (Settings, Keybindings) — mounted
// lazily, exclusively and freshly on every reopen, never persisted.

import (
	"bytes"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ebitenui/ebitenui/widget"
	"github.com/entieve/entieve/assets/artwork"
	"github.com/entieve/entieve/render"
	"github.com/entieve/entieve/uikit/theme"
	"github.com/entieve/entieve/uikit/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

// Title chrome metrics, in design units through the Px funnel.
const (
	titleDim     = 150 // black veil over the static world (the fallback)
	titleArtVeil = 96  // black veil over the artwork plate: 3:1 under the captions
	titleMenuW   = 240 // menu button width
	titleMenuH   = 36  // menu button height
	titleWinW    = 560 // save manager window
	titleWinH    = 400
	creditWinW   = 400 // credits window
	creditWinH   = 520
	titleTitleH  = winTitleH
	titleScrollH = 3 * 20 // wheel step (design px) for the title windows
)

// titleArtwork is the title phase's backdrop plate, decoded once on the
// first title frame. nil = no plate, and the phase keeps the static world
// under titleDim — the contract render's runtime artwork has. A var (not a
// plain func) so tests can force the fallback branch.
var titleArtwork = sync.OnceValue(func() *ebiten.Image {
	img, err := jpeg.Decode(bytes.NewReader(artwork.Title))
	if err != nil || img.Bounds().Empty() {
		return nil
	}
	return ebiten.NewImageFromImage(img)
})

// drawTitleBackdrop paints the layer under the title menu: the embedded
// artwork cover-fit (aspect kept, centred, overflow cropped) under the light
// veil that keeps the wordmark's captions legible, or — with no plate — the
// static world under today's heavier dim, exactly as before. It returns the
// plate it painted (nil = the world fallback), which is how a test sees the
// branch without reading pixels.
func (g *Game) drawTitleBackdrop(dst *ebiten.Image) *ebiten.Image {
	art := titleArtwork()
	if art == nil {
		g.space.Draw(dst)
		dimScreen(dst, g.w, g.h, titleDim)
		return nil
	}
	b := art.Bounds()
	s := render.CoverScale(b.Dx(), b.Dy(), g.w, g.h)
	if s <= 0 {
		return art
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(-float64(b.Dx())/2, -float64(b.Dy())/2)
	op.GeoM.Scale(s, s)
	op.GeoM.Translate(float64(g.w)/2, float64(g.h)/2)
	dst.DrawImage(art, op)
	dimScreen(dst, g.w, g.h, titleArtVeil)
	return art
}

// saveEntry is one file in the saves directory.
type saveEntry struct {
	Path   string
	Name   string // prettified display name
	Mod    time.Time
	Active bool
}

// titleItem is one menu row: the label and its mounted button.
type titleItem struct {
	Label string
	Btn   *widget.Button
}

// titleScreen is the title phase's own UI. It is rebuilt from scratch
// when the kit is rebuilt (a scale change), keeping only the open flags.
type titleScreen struct {
	g *Game

	menu       *widget.Container
	detachMenu func()
	wordmark   *theme.ChromeText
	version    *theme.ChromeText
	errLine    *theme.ChromeText
	items      []titleItem
	errMsg     string

	creditsOpen bool
	managerOpen bool

	// The title's Screen host: the title-scoped rows (Settings,
	// Keybindings), mounted lazily and exclusively. Its rows are the same
	// Screen spec rows the game host mounts.
	rows []Screen
	host *ScreenHost

	credits      *widget.Window
	creditsRem   widget.RemoveWindowFunc
	creditScroll *widget.ScrollContainer // the credits page's viewport (pins)
	manager      *widget.Window
	managerPage  *widget.Container
	managerRem   widget.RemoveWindowFunc
	saves        []saveEntry
	pendingDel   string
}

func newTitleScreen(g *Game) *titleScreen {
	t := &titleScreen{g: g, rows: titleRows()}
	t.host = newScreenHost(t.rows, titleScope, g.kit, g.titleServices())
	t.buildMenu()
	return t
}

// titleServices is the game bundle without the owner's close hook: the
// title host owns its rows' open flags, so a row's × is a plain toggle on
// THIS host (with the game's hook it toggled the game host's flag and the
// title window never closed).
func (g *Game) titleServices() screenServices {
	svc := g.gameServices()
	svc.Close = nil
	return svc
}

// titleRows are the Screen rows that also live on the title screen.
func titleRows() []Screen {
	var out []Screen
	for _, s := range screens {
		if s.InTitle {
			out = append(out, s)
		}
	}
	return out
}

// ensureTitleUI returns the title's UI, building it on first use.
func (g *Game) ensureTitleUI() *titleScreen {
	if g.titleUI == nil {
		g.titleUI = newTitleScreen(g)
	}
	return g.titleUI
}

// updateTitle is the title phase's Update: input goes to the title only,
// no key binding and no sim step runs, and the kit pumps the title's own
// windows. Enter/load actions are applied after the pump (they rebuild
// the interface).
func (g *Game) updateTitle(in Input) error {
	t := g.ensureTitleUI()
	ctrl := held(in.KeysDown, ebiten.KeyControl)
	shift := held(in.KeysDown, ebiten.KeyShift)
	for _, k := range in.JustPressed {
		if g.captureKey(k, ctrl, shift) {
			continue
		}
		switch k {
		case ebiten.KeyEscape:
			t.closeTop()
		case ebiten.KeyP:
			// The settings window's hint ("P closes") stays true at the
			// title: P runs the same toggle it does in-game (its row is
			// the title's settings window here).
			t.toggleSettings()
		}
	}
	if err := g.kit.Update(); err != nil {
		return err
	}
	t.sync()

	if p := g.pendingTitleLoad; p != "" {
		g.pendingTitleLoad = ""
		if err := g.titleLoad(p); err != nil {
			t.flash("Load failed: " + err.Error())
		}
	} else if g.pendingTitleEnter {
		g.pendingTitleEnter = false
		g.leaveTitle()
	}
	if g.quit {
		g.quitPersist()
		return ebiten.Termination
	}
	return nil
}

// titleLoad makes path the active save, loads it and enters the game.
func (g *Game) titleLoad(path string) error {
	prev := g.activeSave
	g.activeSave = path
	if err := g.LoadGame(); err != nil {
		g.activeSave = prev
		return err
	}
	g.leaveTitle()
	return nil
}

// leaveTitle ends the title phase: the gameplay chrome returns from the
// player's own open flags and the normal frame path resumes next frame.
func (g *Game) leaveTitle() {
	if g.titleUI != nil {
		g.titleUI.close()
	}
	g.titleUI = nil
	g.title = false
	g.syncScenes()
	g.syncPanels()
	g.syncMounts()
}

// ---------------------------------------------------------------------------
// Menu
// ---------------------------------------------------------------------------

// buildMenu (re)builds the wordmark and the menu buttons on the current
// kit root. The Continue row exists only while at least one save does.
func (t *titleScreen) buildMenu() {
	g := t.g
	p := g.env.Pal()
	if t.detachMenu != nil {
		t.detachMenu()
		t.detachMenu = nil
	}
	menu := widget.NewContainer(
		widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.AnchorLayoutData{
			HorizontalPosition: widget.AnchorLayoutPositionCenter,
			VerticalPosition:   widget.AnchorLayoutPositionCenter,
		})),
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(8)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(12))),
		)),
	)
	t.wordmark = theme.NewChromeText(g.env, theme.StepLarge, "ENTIEVE", p.Accent)
	t.wordmark.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}
	menu.AddChild(t.wordmark)
	t.version = nil
	if v := g.opts.Version; v != "" {
		t.version = theme.NewChromeText(g.env, theme.StepTiny, versionLine(v), p.Dim)
		t.version.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}
		menu.AddChild(t.version)
	}
	if t.errMsg != "" {
		t.errLine = theme.NewChromeText(g.env, theme.StepTiny, t.errMsg, p.Danger)
		t.errLine.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}
		menu.AddChild(t.errLine)
	}
	menu.AddChild(widget.NewContainer(widget.ContainerOpts.WidgetOpts(
		widget.WidgetOpts.MinSize(0, theme.Px(14)),
	)))

	t.items = nil
	for _, label := range t.labels() {
		label := label
		btn := g.newBtn(label, titleMenuW, titleMenuH, t.action(label))
		btn.GetWidget().LayoutData = widget.RowLayoutData{Stretch: true}
		menu.AddChild(btn)
		t.items = append(t.items, titleItem{Label: label, Btn: btn})
	}

	t.menu = menu
	root, ok := g.kit.Root().(*widget.Container)
	if !ok {
		return
	}
	t.detachMenu = root.AddChild(menu)
	theme.ShowWidget(menu.GetWidget(), !t.windowOpen())
}

// labels are the menu rows, in draw order.
func (t *titleScreen) labels() []string {
	out := []string{"New game"}
	if len(t.g.listSaves()) > 0 {
		out = append(out, "Continue")
	}
	out = append(out, "Save manager")
	for _, s := range t.rows {
		out = append(out, s.Title)
	}
	return append(out, "Credits", "Quit")
}

// action resolves one menu label to its click.
func (t *titleScreen) action(label string) func() {
	g := t.g
	switch label {
	case "New game":
		return func() { g.pendingTitleEnter = true }
	case "Continue":
		return func() { t.continueMostRecent() }
	case "Save manager":
		return func() { t.open("manager") }
	case "Credits":
		return func() { t.open("credits") }
	case "Quit":
		return func() { g.quit = true }
	}
	for _, s := range t.rows {
		if s.Title == label {
			id := s.ID
			return func() { t.open(string(id)) }
		}
	}
	return func() {}
}

// continueMostRecent stages a load of the newest save (mtime order).
func (t *titleScreen) continueMostRecent() {
	if saves := t.g.listSaves(); len(saves) > 0 {
		t.g.pendingTitleLoad = saves[0].Path
	}
}

// versionLine is the wordmark's version caption.
func versionLine(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

// ---------------------------------------------------------------------------
// Title windows
// ---------------------------------------------------------------------------

func (t *titleScreen) windowOpen() bool {
	return t.host.AnyOpen() || t.creditsOpen || t.managerOpen
}

// open shows one title window (they are modal: at most one at a time).
func (t *titleScreen) open(which string) {
	t.creditsOpen = which == "credits"
	t.managerOpen = which == "manager"
	t.pendingDel = ""
	t.g.cancelCapture()
	t.host.CloseAll()
	t.host.SetOpen(ScreenID(which), true)
	if t.managerOpen {
		t.saves = t.g.listSaves()
	}
}

// closeTop closes the open title window (Esc / the title-bar ×).
func (t *titleScreen) closeTop() {
	t.host.CloseAll()
	t.creditsOpen, t.managerOpen = false, false
	t.pendingDel = ""
	t.g.cancelCapture()
}

// toggleSettings mirrors the in-game P row: the title owns its settings
// window, so P closes it when open and opens it (over any other title
// window) otherwise — the same toggle the Screen spec's settings row has.
func (t *titleScreen) toggleSettings() {
	if t.host.IsOpen(idSettings) {
		t.closeTop()
		return
	}
	t.open(string(idSettings))
}

// flash reports a failed action on the menu.
func (t *titleScreen) flash(msg string) {
	t.errMsg = msg
	t.closeTop()
	t.buildMenu()
}

// sync mounts/unmounts the open title windows and refreshes the settings
// readout. It runs after the kit pump, so the handlers can only flip the
// flags.
func (t *titleScreen) sync() {
	if t.menu == nil {
		t.buildMenu()
	}
	t.host.Sync()
	t.host.Reconcile(Room{Title: true})
	if t.creditsOpen {
		if t.creditsRem == nil {
			t.mountCredits()
		}
	} else {
		t.unmountCredits()
	}
	if t.managerOpen {
		if t.managerRem == nil {
			t.mountManager()
		}
	} else {
		t.unmountManager()
	}
	if t.menu != nil {
		theme.ShowWidget(t.menu.GetWidget(), !t.windowOpen())
	}
}

// detach drops the widget references of the previous kit (a rebuild); the
// open flags and the save list stay, so the next sync remounts the same
// windows on the fresh kit.
func (t *titleScreen) detach() {
	t.menu = nil
	t.detachMenu = nil
	t.wordmark, t.version, t.errLine = nil, nil, nil
	t.items = nil
	t.credits, t.creditsRem, t.creditScroll = nil, nil, nil
	t.manager, t.managerPage, t.managerRem = nil, nil, nil
}

// close removes every title widget and drops the state.
func (t *titleScreen) close() {
	if t.detachMenu != nil {
		t.detachMenu()
	}
	t.detachMenu = nil
	t.host.CloseAll()
	t.host.Reconcile(Room{Title: true})
	t.unmountCredits()
	t.unmountManager()
	t.detach()
}

func (t *titleScreen) mountCredits() {
	g := t.g
	p := g.env.Pal()
	roleW := theme.Px(84)
	textW := theme.Px(creditWinW-2*theme.ContentsPad-theme.ScrollGutter-2*theme.ContentsPad) - roleW - theme.Px(theme.SpMd)

	page := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(theme.SpSm)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	// The header block: the tracked wordmark (the title screen's own
	// treatment, text only) over the "Credits" subtitle, ruled off from
	// the sections.
	wordmark := theme.NewChromeText(g.env, theme.StepLarge, "ENTIEVE", p.Accent)
	wordmark.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}
	subtitle := theme.NewChromeText(g.env, theme.StepSmall, "CREDITS", p.Dim)
	subtitle.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionCenter}
	page.AddChild(wordmark, subtitle, creditRule(p))

	for i, sec := range creditSections {
		if i > 0 { // the sections' air: a real gap, not another line
			page.AddChild(widget.NewContainer(widget.ContainerOpts.WidgetOpts(
				widget.WidgetOpts.MinSize(0, theme.Px(theme.SpLg)))))
		}
		page.AddChild(theme.NewChromeText(g.env, theme.StepSmall, strings.ToUpper(sec.Title), p.Head))
		page.AddChild(creditRule(p))
		for _, row := range sec.Rows {
			role := widget.NewText(
				widget.TextOpts.Text(row.Role, g.env.Face(theme.StepTiny), theme.Solid(p.Dim)),
				widget.TextOpts.MaxWidth(float64(roleW)),
				widget.TextOpts.WidgetOpts(widget.WidgetOpts.MinSize(roleW, 0)),
			)
			text := widget.NewText(
				widget.TextOpts.Text(row.Text, g.env.Face(theme.StepTiny), theme.Solid(p.Text)),
				widget.TextOpts.MaxWidth(float64(textW)),
			)
			line := widget.NewContainer(
				widget.ContainerOpts.Layout(widget.NewGridLayout(
					widget.GridLayoutOpts.Columns(2),
					widget.GridLayoutOpts.Stretch([]bool{false, true}, []bool{false}),
					widget.GridLayoutOpts.Spacing(theme.Px(theme.SpMd), 0),
				)),
				widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true})),
			)
			line.AddChild(role, text)
			page.AddChild(line)
		}
	}

	// The footer: the licence note that closes the screen.
	page.AddChild(widget.NewContainer(widget.ContainerOpts.WidgetOpts(
		widget.WidgetOpts.MinSize(0, theme.Px(theme.SpMd)))))
	page.AddChild(creditRule(p))
	page.AddChild(widget.NewText(
		widget.TextOpts.Text("MIT License — © 2026 The Entieve Authors. Distributed without warranty; see LICENSE.",
			g.env.Face(theme.StepTiny), theme.Solid(p.Dim)),
		widget.TextOpts.MaxWidth(float64(roleW+theme.Px(theme.SpMd)+textW)),
	))

	// The house title bar (dots, tracked title) — the panels' chrome, so
	// the credits screen opens like every other window.
	bar, _ := theme.NewTitleBar(g.env, "Credits", "")
	win, scroll := t.window(bar, page, creditWinW, creditWinH, image.Rect(380, 100, 380+creditWinW, 100+creditWinH))
	t.creditScroll = scroll
	t.credits = win
	g.kit.SetWindowClose(win, t.closeTop)
	t.creditsRem = g.kit.AddWindow(win)
}

// creditRule is the hairline under the credits header block and each
// section header (the InfoWin section-rule idiom).
func creditRule(p theme.Palette) *widget.Container {
	return widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.NineSlice(theme.NRGBA(p.Accent, widgets.TipRuleA))),
		widget.ContainerOpts.WidgetOpts(
			widget.WidgetOpts.MinSize(0, theme.Px(widgets.TipRuleH)),
			widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}),
		),
	)
}

func (t *titleScreen) unmountCredits() {
	if t.creditsRem != nil {
		t.creditsRem()
	}
	t.credits, t.creditsRem = nil, nil
}

func (t *titleScreen) mountManager() {
	g := t.g
	t.saves = g.listSaves()
	page := widget.NewContainer(
		widget.ContainerOpts.Layout(widget.NewRowLayout(
			widget.RowLayoutOpts.Direction(widget.DirectionVertical),
			widget.RowLayoutOpts.Spacing(theme.Px(4)),
			widget.RowLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	t.managerPage = page
	t.refreshManager()
	win, _ := t.window(g.titleBar("Save manager"), page, titleWinW, titleWinH, image.Rect(360, 140, 360+titleWinW, 140+titleWinH))
	t.manager = win
	g.kit.SetWindowClose(win, t.closeTop)
	t.managerRem = g.kit.AddWindow(win)
}

func (t *titleScreen) unmountManager() {
	if t.managerRem != nil {
		t.managerRem()
	}
	t.manager, t.managerPage, t.managerRem = nil, nil, nil
	t.pendingDel = ""
}

// window wraps a scrollable page in the house window chrome (InfoWin's
// pattern: fixed rect, grid-stretched viewport beside the house rail).
func (t *titleScreen) window(bar *widget.Container, page *widget.Container, w, h int, at image.Rectangle) (*widget.Window, *widget.ScrollContainer) {
	g := t.g
	p := g.env.Pal()
	scroll := widget.NewScrollContainer(
		widget.ScrollContainerOpts.Content(page),
		widget.ScrollContainerOpts.StretchContentWidth(),
		widget.ScrollContainerOpts.Image(&widget.ScrollContainerImage{
			Idle: bordered(theme.WindowBody(p.BG), nrgba(p.Border, 160), 1),
			Mask: theme.NineSlice(theme.Solid(p.BG)),
		}),
	)
	scroll.GetWidget().ScrolledEvent.AddHandler(func(args any) {
		ev, ok := args.(*widget.WidgetScrolledEventArgs)
		if !ok {
			return
		}
		_, ch := page.PreferredSize()
		over := ch - scroll.ViewRect().Dy()
		if over <= 0 {
			scroll.ScrollTop = 0
			return
		}
		scroll.ScrollTop = max(0, min(1, scroll.ScrollTop-ev.Y*float64(theme.Px(titleScrollH))/float64(over)))
	})
	widgets.AttachScrollContent(scroll, page)
	rail := widgets.NewScrollRail(g.env, scroll, scroll, page)
	body := widget.NewContainer(
		widget.ContainerOpts.BackgroundImage(theme.WindowBodyBox(p, theme.WindowBody(p.BG), nrgba(p.Border, 200))),
		widget.ContainerOpts.Layout(widget.NewGridLayout(
			widget.GridLayoutOpts.Columns(1),
			widget.GridLayoutOpts.Stretch([]bool{true}, []bool{true}),
			widget.GridLayoutOpts.Padding(widget.NewInsetsSimple(theme.Px(theme.ContentsPad))),
		)),
	)
	body.AddChild(rail.Container())
	win := widget.NewWindow(
		widget.WindowOpts.Contents(body),
		widget.WindowOpts.TitleBar(bar, theme.Px(titleTitleH)),
		widget.WindowOpts.Modal(),
		widget.WindowOpts.MinSize(theme.Px(w), theme.Px(h)),
		widget.WindowOpts.Location(image.Rect(
			theme.Px(at.Min.X), theme.Px(at.Min.Y), theme.Px(at.Max.X), theme.Px(at.Max.Y))),
	)
	return win, scroll
}

// refreshManager rebuilds the save rows from t.saves (and shows the empty
// state when the directory holds none).
func (t *titleScreen) refreshManager() {
	if t.managerPage == nil {
		return
	}
	g := t.g
	p := g.env.Pal()
	page := t.managerPage
	page.RemoveChildren()
	if len(t.saves) == 0 {
		page.AddChild(widget.NewText(widget.TextOpts.Text("No saves yet.", g.env.Face(theme.StepSmall), theme.Solid(p.Dim))))
		return
	}
	for _, e := range t.saves {
		e := e
		row := widget.NewContainer(
			widget.ContainerOpts.WidgetOpts(widget.WidgetOpts.MinSize(0, theme.Px(30))),
			widget.ContainerOpts.Layout(widget.NewRowLayout(
				widget.RowLayoutOpts.Direction(widget.DirectionHorizontal),
				widget.RowLayoutOpts.Spacing(theme.Px(8)),
			)),
		)
		mark, nameCol := "", p.Text
		if e.Active {
			mark, nameCol = "●", p.Accent
		}
		row.AddChild(
			widget.NewText(widget.TextOpts.Text(mark, g.env.Face(theme.StepSmall), theme.Solid(p.Accent)),
				widget.TextOpts.WidgetOpts(widget.WidgetOpts.MinSize(theme.Px(14), 0))),
			widget.NewText(widget.TextOpts.Text(e.Name, g.env.Face(theme.StepSmall), theme.Solid(nameCol))),
			widget.NewText(widget.TextOpts.Text(e.Mod.Format("2006-01-02 15:04"), g.env.Face(theme.StepTiny), theme.Solid(p.Dim)),
				widget.TextOpts.WidgetOpts(widget.WidgetOpts.LayoutData(widget.RowLayoutData{Stretch: true}))),
		)
		del := "Delete"
		if t.pendingDel == e.Path {
			del = "Confirm?"
		}
		loadBtn := g.newBtn("Load", 70, 30, func() { g.pendingTitleLoad = e.Path })
		delBtn := g.newBtn(del, 90, 30, func() { t.deleteSave(e.Path) })
		loadBtn.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionEnd}
		delBtn.GetWidget().LayoutData = widget.RowLayoutData{Position: widget.RowLayoutPositionEnd}
		row.AddChild(loadBtn, delBtn)
		page.AddChild(row)
	}
}

// deleteSave is the two-click delete: the first click arms the row, the
// second removes exactly that file.
func (t *titleScreen) deleteSave(path string) {
	if t.pendingDel != path {
		t.pendingDel = path
		t.refreshManager()
		return
	}
	t.pendingDel = ""
	if err := os.Remove(path); err != nil {
		t.flash("Delete failed: " + err.Error())
		return
	}
	t.saves = t.g.listSaves()
	t.refreshManager()
	t.buildMenu() // Continue follows the save count
}

// ---------------------------------------------------------------------------
// Saves directory
// ---------------------------------------------------------------------------

// listSaves reads dir's save files — *.json minus the layout/settings/keys
// sidecars — newest first (name breaks mtime ties).
func listSaves(dir string) []saveEntry {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []saveEntry
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		if strings.HasSuffix(name, ".layout.json") || strings.HasSuffix(name, ".settings.json") ||
			strings.HasSuffix(name, ".keys.json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, saveEntry{
			Path: filepath.Join(dir, name),
			Name: saveDisplayName(name),
			Mod:  info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Mod.Equal(out[j].Mod) {
			return out[i].Mod.After(out[j].Mod)
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// g.listSaves is the manager's list with the active save marked.
func (g *Game) listSaves() []saveEntry {
	saves := listSaves(filepath.Dir(g.SavePath()))
	active := filepath.Clean(g.activeSavePath())
	for i := range saves {
		saves[i].Active = filepath.Clean(saves[i].Path) == active
	}
	return saves
}

// saveDisplayName prettifies a save filename: "save" → "Save",
// "save-2" → "Save 2".
func saveDisplayName(name string) string {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	parts := strings.FieldsFunc(stem, func(r rune) bool { return r == '-' || r == '_' })
	for i, p := range parts {
		r := []rune(p)
		r[0] = unicode.ToUpper(r[0])
		parts[i] = string(r)
	}
	return strings.Join(parts, " ")
}
