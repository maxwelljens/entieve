package svgraster

import (
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// probeSVG exercises every element and path command of the subset, plus two
// things it must ignore: an unknown <g> wrapper and a fill="none" shape.
const probeSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48">
  <rect x="4" y="4" width="12" height="12" fill="#c8102e"/>
  <circle cx="32" cy="10" r="6" fill="#123456"/>
  <ellipse cx="10" cy="34" rx="6" ry="4" fill="#0f0"/>
  <polygon points="26,26 40,26 33,40" fill="#ffcc00"/>
  <path d="M2 44 h8 v4 h-8 z" fill="#3366ff"/>
  <path d="M44 20 q2 4 4 0" fill="#993300"/>
  <path d="M18 40 c0 -6 4 -6 4 0 z" fill="#559944"/>
  <g fill="#ffffff"><rect x="44" y="44" width="2" height="2" fill="none"/></g>
</svg>`

// probePNG pins the bytes tools/emblemraster produced for probeSVG at a 48
// design-unit viewBox before the pipeline was shared. Sharing the code must
// not move one byte of the committed art, and this is the same code path.
var probePNG = map[int]string{
	24: "664b6865dc495303c95cf055293108d35b41626e74ae10655fc23f3635c2276a",
	32: "21fc939298ef309bb9e2f85226f02c0270bab351811e24764731b5c753384dd0",
	48: "40e1d7238965d6fb43733a71a4ce4dfb7960d4d27bdc1145b6127b369f1cf753",
}

func TestBuildIsByteStable(t *testing.T) {
	first, firstOut := build(t)
	_, secondOut := build(t)
	for size, want := range probePNG {
		key := strconv.Itoa(size)
		file := "probe-" + key + ".png"
		a, err := os.ReadFile(filepath.Join(firstOut, file))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(secondOut, file))
		if err != nil {
			t.Fatal(err)
		}
		if sha(a) != want {
			t.Errorf("%s: hash = %s, want the pre-share %s", file, sha(a), want)
		}
		if got := first["probe"].PNG[key]; got != want {
			t.Errorf("%s: manifest records %s, want %s", file, got, want)
		}
		if string(a) != string(b) {
			t.Errorf("%s: two builds of one source disagree", file)
		}
	}
}

func TestBuildRendersFills(t *testing.T) {
	_, out := build(t)
	f, err := os.Open(filepath.Join(out, "probe-24.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 24 || b.Dy() != 24 {
		t.Fatalf("box = %v, want 24×24", b)
	}
	// The rect at design (4,4)–(16,16) is opaque #c8102e at half scale.
	if got := color.NRGBAModel.Convert(img.At(5, 5)).(color.NRGBA); got != (color.NRGBA{0xc8, 0x10, 0x2e, 0xff}) {
		t.Errorf("rect fill = %v, want #c8102e", got)
	}
	if got := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA); got.A != 0 {
		t.Errorf("corner = %v, want transparent", got)
	}
}

func TestBuildRejects(t *testing.T) {
	for _, c := range []struct{ name, svg, want string }{
		{"gradient fill", `<rect x="1" y="1" width="2" height="2" fill="url(#g)"/>`, "unsupported fill"},
		{"unknown command", `<path d="X1 1" fill="#fff"/>`, "unsupported path command"},
		{"odd points", `<polygon points="1,2 3,4 5" fill="#fff"/>`, "want x,y pairs"},
		{"missing attr", `<rect x="1" y="1" width="2" fill="#fff"/>`, "rect needs x/y/width/height"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			writeSource(t, dir, c.svg)
			if _, err := Build(filepath.Join(dir, "svg"), filepath.Join(dir, "out"), 48, []int{8}); err == nil {
				t.Fatal("Build accepted an unsupported source")
			} else if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestBuildNeedsSources(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "svg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(filepath.Join(dir, "svg"), dir, 48, []int{8}); err == nil {
		t.Fatal("Build accepted an empty source directory")
	}
}

func build(t *testing.T) (map[string]Entry, string) {
	t.Helper()
	dir := t.TempDir()
	writeSource(t, dir, probeSVG)
	out := filepath.Join(dir, "out")
	set, err := Build(filepath.Join(dir, "svg"), out, 48, []int{24, 32, 48})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return set, out
}

func writeSource(t *testing.T, dir, svg string) {
	t.Helper()
	for _, d := range []string{filepath.Join(dir, "svg"), filepath.Join(dir, "out")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "svg", "probe.svg"), []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
}
