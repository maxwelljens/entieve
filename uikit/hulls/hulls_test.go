package hulls

import "testing"

// The stub keeps the real package's size contract: smallest raster >= size,
// else the largest, never nil.
func TestGetSizeContract(t *testing.T) {
	for _, size := range []int{1, 40, 120, 121, 240} {
		img := Get("ship-sloop", size)
		if img == nil {
			t.Fatalf("Get(size=%d) = nil", size)
		}
		if got := img.Bounds().Dx(); got != 120 {
			t.Errorf("Get(size=%d) = %d px, want the single 120 px placeholder", size, got)
		}
	}
	if !Has("ship-sloop") {
		t.Error("Has(ship-sloop) = false")
	}
}
