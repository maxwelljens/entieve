package content

// The station service vocabulary — the one owner of the ids the content
// files spell (StationDef.Services), their display labels (interface
// strings registered in docs/lore/NAMING.md), and the Screen window each
// service unlocks. The rail and the Screen spec's own gate, the station
// host's dock reads and the sim's command gates all read this table, so a
// renamed window or a typo'd id fails one pin instead of a row that
// silently never opens.
//
// The ids are plain strings: content files carry them verbatim and the
// sim's Station component copies them, so no conversion happens at any
// boundary.
const (
	SvcRepair   = "repair"
	SvcMarket   = "market"
	SvcRefine   = "refine"
	SvcFitting  = "fitting"
	SvcIndustry = "industry"
)

// ServiceDef is one station service's interface description.
type ServiceDef struct {
	ID     string // the id StationDef.Services spells
	Label  string // display name (docs/lore/NAMING.md interface strings)
	Icon   string // icon manifest name ("" = the rail draws no glyph)
	Window string // Screen title the service unlocks ("" = no window)
}

// Services is the vocabulary in the order the rail lists the windows;
// entries without a Window (repair) open no window.
var Services = []ServiceDef{
	{ID: SvcRepair, Label: "Repair"},
	{ID: SvcMarket, Label: "Market", Icon: "svc_market", Window: "Market"},
	{ID: SvcFitting, Label: "Fitting", Icon: "svc_fitting", Window: "Fitting"},
	{ID: SvcIndustry, Label: "Manufacturing", Icon: "svc_manufacturing", Window: "Manufacturing"},
	{ID: SvcRefine, Label: "Refinery", Icon: "svc_refinery", Window: "Refinery"},
}

// Service returns the table entry with the given id (ok=false when id is
// not a station service).
func Service(id string) (ServiceDef, bool) {
	for _, s := range Services {
		if s.ID == id {
			return s, true
		}
	}
	return ServiceDef{}, false
}
