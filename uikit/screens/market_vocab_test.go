package screens

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/ebitenui/ebitenui/widget"
)

// bidAskWord is the retired market vocabulary: whole words only, so
// ordinary prose ("asks for an amount", "abide") never false-positives.
var bidAskWord = regexp.MustCompile(`(?i)\b(bid|bids|ask|asks)\b`)

// assertNoBidAsk fails for every rendered line still naming the old side
// vocabulary.
func assertNoBidAsk(t *testing.T, surface string, lines []string) {
	t.Helper()
	for _, s := range lines {
		if m := bidAskWord.FindString(s); m != "" {
			t.Errorf("%s still renders %q in %q", surface, m, s)
		}
	}
}

// TestMarketSurfacesCarryNoBidAsk: every player-facing market string reads
// buy order / sell order (the book cells' compact Buy/Sell), in the branch
// catalog and in the search results alike. The sweep walks the rendered
// widget tree, not the model.
func TestMarketSurfacesCarryNoBidAsk(t *testing.T) {
	m, _, _, frame, click := mktTreeEnv(t)
	frame()
	frame()
	click(mktParentRow(t, m, "item-stub-ore"))
	frame()

	var texts []string
	for _, w := range []widget.PreferredSizeLocateableWidget{
		m.tbl.Container(), m.bookBar, m.bookTbl.Container(), m.mineBar, m.mineBox, m.headBox,
	} {
		collectTexts(w, &texts)
	}
	joined := strings.Join(texts, "\n")
	if !strings.Contains(joined, "\nBuy\n") || !strings.Contains(joined, "\nSell\n") {
		t.Fatalf("fixture: the book's compact side cells did not render: %q", joined)
	}
	assertNoBidAsk(t, "market catalog", texts)

	m.search.SetText("ore")
	frame()
	texts = texts[:0]
	for _, w := range []widget.PreferredSizeLocateableWidget{m.tbl.Container(), m.bookTbl.Container(), m.mineBox} {
		collectTexts(w, &texts)
	}
	if len(m.results) == 0 {
		t.Fatal("fixture: the search rendered no results")
	}
	assertNoBidAsk(t, "market search", texts)
}

// TestMoneyTablesCarryNoBidAsk: the manufacturing profit table and the
// refinery's refine math speak the same vocabulary, including their dash
// markers (`— · no buy order`).
func TestMoneyTablesCarryNoBidAsk(t *testing.T) {
	f, env, ui, _, frame := boardEnv(t)
	rec := env.Content.Recipes["rec-mod-artillery-l"]
	clickRow(ui, f.recipes, recipeRowFor(f, rec.Product))
	frame()
	var texts []string
	collectTexts(f.Win().GetContainer(), &texts)
	assertNoBidAsk(t, "profit table", texts)

	r, _, _, _, frame2 := refineryEnv(t)
	frame2()
	texts = texts[:0]
	collectTexts(r.Win().GetContainer(), &texts)
	assertNoBidAsk(t, "refine math", texts)
}

// TestDocsCarryTheMarketVocabulary: the player-facing docs name the sides
// buy order / sell order, and the availability suffix section is gone.
// CONTEXT keeps the one mapping row (the sim's bid/ask are code names).
func TestDocsCarryTheMarketVocabulary(t *testing.T) {
	gameplay, err := os.ReadFile("../../docs/GAMEPLAY.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"no bid", "· N more", "cheapest ask", "best ask", "best bid", "Cross-station availability"} {
		if strings.Contains(string(gameplay), gone) {
			t.Errorf("GAMEPLAY.md still carries %q", gone)
		}
	}
	sim, err := os.ReadFile("../../docs/SIM.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"best ask", "best bid", "asks\n  before bids"} {
		if strings.Contains(string(sim), gone) {
			t.Errorf("SIM.md still carries %q", gone)
		}
	}
	ctx, err := os.ReadFile("../../CONTEXT.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"**Buy order**", "**Sell order**", "the sim's *bid*", "the sim's *ask*"} {
		if !strings.Contains(string(ctx), want) {
			t.Errorf("CONTEXT.md lost %q", want)
		}
	}
}
