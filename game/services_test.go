package game

import (
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
)

// TestServiceTableAgreesWithScreens pins the two ends of the station
// service vocabulary: content.Services (the owner) and the Screen rows the
// rail opens. The contacts strip carries no window button at all — a second
// door to a window the rail already lists is a second thing to keep in
// sync — so the pin is that every service's window IS a Screen row, gated
// on that service, with the table's icon, and that the strip offers none of
// them.
func TestServiceTableAgreesWithScreens(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	svc := stationBoard(t, g).Services()
	for _, s := range content.Services {
		if s.Window == "" {
			if s.Icon != "" {
				t.Errorf("%s: no window, but the table names icon %q", s.ID, s.Icon)
			}
			continue
		}
		if b := svc.Button(s.ID); b != nil {
			t.Errorf("%s: the contacts strip draws a %q window button — the rail owns that window", s.ID, s.ID)
		}
		id, ok := g.host.ByTitle(strings.ToLower(s.Window))
		if !ok {
			t.Fatalf("%s: window %q names no Screen row", s.ID, s.Window)
		}
		row := screenByID(id)
		if row.Title != s.Window {
			t.Errorf("%s: window %q resolves to row %q", s.ID, s.Window, row.Title)
		}
		if row.Service != s.ID {
			t.Errorf("%s: window %q is gated on service %q", s.ID, row.Title, row.Service)
		}
		if row.Icon != s.Icon {
			t.Errorf("%s: row icon %q, table icon %q", s.ID, row.Icon, s.Icon)
		}
	}
	// The other direction: every service-gated row is a table entry naming
	// that very window.
	for _, s := range screens {
		if s.Service == "" {
			continue
		}
		srv, ok := content.Service(s.Service)
		if !ok {
			t.Errorf("row %q is gated on unknown service %q", s.Title, s.Service)
			continue
		}
		if srv.Window != s.Title {
			t.Errorf("row %q needs %q, but that service names window %q", s.Title, s.Service, srv.Window)
		}
	}
}
