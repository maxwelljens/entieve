package game

import (
	"os"
	"slices"
	"testing"
)

// The frozen identity guards: the Screen ids are sidecar keys on disk and
// test-facing names. They cannot be renamed without a migration, so the
// literal list and its order are pinned here.
func TestWindowNamesFrozen(t *testing.T) {
	want := []string{
		"inventory", "fitting", "market", "manufacturing", "scanner",
		"map", "help", "log", "settings", "services", "refinery",
		"finances", "keybindings", "menu",
	}
	var got []string
	for _, s := range screens {
		got = append(got, screenLabel(s))
	}
	if !slices.Equal(got, want) {
		t.Fatalf("window names = %v, want the frozen %v", got, want)
	}
}

// TestSidecarKeyShape pins the persisted keys (id shape and order) before
// the identity collapse: the sidecar's window keys are user data.
func TestSidecarKeyShape(t *testing.T) {
	want := []string{
		"inventory", "fitting", "market", "manufacturing", "scanner",
		"map", "help", "log", "settings", "services", "refinery",
		"finances", "keybindings",
	}
	if got := sidecarNames(screens); !slices.Equal(got, want) {
		t.Fatalf("sidecar names = %v, want %v", got, want)
	}
	for _, k := range want {
		if k == "" || k[0] < 'a' || k[0] > 'z' {
			t.Errorf("sidecar key %q is not a lowercase word", k)
		}
		for _, r := range k {
			if (r < 'a' || r > 'z') && r != '-' {
				t.Errorf("sidecar key %q contains %q", k, r)
			}
		}
	}
}

// TestLayoutSidecarBytesStable is the v3 format's byte golden: one
// representative arrangement (two windows open, a split dragged, measured
// column widths) must serialise exactly as before the window-host
// refactor. Any drift here is user-data drift.
func TestLayoutSidecarBytesStable(t *testing.T) {
	g := newShowcaseGame(t)
	g.applySize(1280, 720)
	g.host.SetOpen(idInventory, true)
	g.host.SetOpen(idMarket, true)
	g.syncMounts()
	pump(t, g, 1280, 720)
	for _, sm := range g.host.Slot(string(idInventory)).Splits() {
		if sm.Name == "inventory-ore" {
			sm.Split.SetRatio(sm.Index, 0.42)
		}
	}
	if err := g.writeLayout(); err != nil {
		t.Fatalf("writeLayout: %v", err)
	}
	data, err := os.ReadFile(g.layoutPath())
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	if string(data) != layoutBytesGolden {
		t.Errorf("sidecar bytes changed:\n got: %s\nwant: %s", data, layoutBytesGolden)
	}
}

const layoutBytesGolden = `{
  "version": 3,
  "windows": {
    "finances": {
      "x": 440,
      "y": 180,
      "w": 400,
      "h": 360,
      "open": false
    },
    "fitting": {
      "x": 370,
      "y": 8,
      "w": 644,
      "h": 682,
      "open": false,
      "splits": {
        "fitting-hangar": 0.6
      }
    },
    "help": {
      "x": 190,
      "y": 24,
      "w": 900,
      "h": 613,
      "open": false
    },
    "infowin": {
      "x": 430,
      "y": 130,
      "w": 420,
      "h": 460,
      "open": false
    },
    "inventory": {
      "x": 396,
      "y": 256,
      "w": 320,
      "h": 434,
      "open": true,
      "splits": {
        "inventory-hangar": 0.6666666666666666,
        "inventory-ore": 0.42
      },
      "tables": {
        "inventory": {
          "Item": {
            "width": 180.96875
          },
          "Qty": {
            "width": 41.4375
          },
          "Vol": {
            "width": 35.59375
          }
        }
      }
    },
    "keybindings": {
      "x": 400,
      "y": 40,
      "w": 480,
      "h": 640,
      "open": false
    },
    "log": {
      "x": 97,
      "y": 470,
      "w": 264,
      "h": 240,
      "open": true
    },
    "manufacturing": {
      "x": 400,
      "y": 120,
      "w": 480,
      "h": 480,
      "open": false,
      "splits": {
        "manufacturing-jobs": 0.5
      }
    },
    "map": {
      "x": 282,
      "y": 80,
      "w": 716,
      "h": 560,
      "open": false
    },
    "market": {
      "x": 97,
      "y": 8,
      "w": 760,
      "h": 560,
      "open": true,
      "splits": {
        "market-orders": 0.6739130434782609,
        "market-tree": 0.2391304347826087
      },
      "tables": {
        "book": {
          "Item": {
            "width": 131.25
          },
          "Price": {
            "width": 53.15625
          },
          "Qty": {
            "width": 41.4375
          },
          "Side": {
            "width": 48.15625
          }
        },
        "tree": {
          "Tree": {
            "width": 130
          }
        }
      }
    },
    "overview": {
      "x": 890,
      "y": 8,
      "w": 380,
      "h": 409,
      "open": true,
      "tables": {
        "overview": {
          "Distance": {
            "width": 97.203125
          },
          "Kind": {
            "width": 49.546875
          },
          "Name": {
            "width": 96.859375
          },
          "Velocity": {
            "width": 74.390625
          }
        }
      }
    },
    "refinery": {
      "x": 430,
      "y": 160,
      "w": 420,
      "h": 400,
      "open": false
    },
    "scanner": {
      "x": 483,
      "y": 276,
      "w": 300,
      "h": 248,
      "open": false
    },
    "selected": {
      "x": 95,
      "y": 8,
      "w": 464,
      "h": 260,
      "open": true
    },
    "services": {
      "x": 97,
      "y": 8,
      "w": 307,
      "h": 200,
      "open": false
    },
    "settings": {
      "x": 450,
      "y": 106,
      "w": 380,
      "h": 508,
      "open": false
    }
  }
}`
