package game

import (
	"strings"
	"testing"

	"github.com/entieve/entieve/content"
)

// TestServiceTableAgreesWithScreens pins the two ends of the station
// service vocabulary: content.Services (the owner) and the Screen rows the
// services row's buttons toggle. A renamed window, a drifted icon or a
// lost button fails here instead of dying silently — Toggle{Screen} falls
// through to the kit on an unknown title, and a hidden button offers
// nothing.
func TestServiceTableAgreesWithScreens(t *testing.T) {
	g := dockedShowcaseGame(t, 1280, 720)
	for _, svc := range content.Services {
		b := stationBoard(t, g).Services().Button(svc.ID)
		if svc.Window == "" {
			if b != nil {
				t.Errorf("%s: no window, but the services row drew a %q button", svc.ID, svc.ID)
			}
			if svc.Icon != "" {
				t.Errorf("%s: no window, but the table names icon %q", svc.ID, svc.Icon)
			}
			continue
		}
		if b == nil {
			t.Fatalf("%s: the services row has no %q button", svc.ID, svc.ID)
		}
		id, ok := g.host.ByTitle(strings.ToLower(svc.Window))
		if !ok {
			t.Fatalf("%s: window %q names no Screen row", svc.ID, svc.Window)
		}
		row := screenByID(id)
		if row.Title != svc.Window {
			t.Errorf("%s: window %q resolves to row %q", svc.ID, svc.Window, row.Title)
		}
		if row.Service != svc.ID {
			t.Errorf("%s: window %q is gated on service %q", svc.ID, row.Title, row.Service)
		}
		if row.Icon != svc.Icon {
			t.Errorf("%s: row icon %q, table icon %q", svc.ID, row.Icon, svc.Icon)
		}
	}
	// The other direction: every service-gated row is a table entry whose
	// button opens that very row.
	for _, s := range screens {
		if s.Service == "" {
			continue
		}
		svc, ok := content.Service(s.Service)
		if !ok {
			t.Errorf("row %q is gated on unknown service %q", s.Title, s.Service)
			continue
		}
		if svc.Window != s.Title {
			t.Errorf("row %q needs %q, but that service's button opens %q", s.Title, s.Service, svc.Window)
		}
	}
}
