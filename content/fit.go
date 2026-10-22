package content

import "slices"

// Fitting limits (CONTEXT.md: mount, grid class / cpu class, class quota).
//
// High slots are mounts: a mounted module fits only a mount of its own kind
// whose size is at or above the module's, so a large autocannon never sits on
// a frigate. Every module also demands one unit of its grid class and/or cpu
// class on the hull; a hull's class quotas bound how many modules of each
// class a fit may hold. There is no grid/cpu arithmetic anywhere — only
// classes and counts.

// High-slot mount kinds.
const (
	MountTurret   = "turret"
	MountLauncher = "launcher"
)

// Mount sizes, smallest first.
const (
	SizeS = "s"
	SizeM = "m"
	SizeL = "l"
)

// Module classes, one per tier on each axis (ModuleDef.GridClass/CPUClass).
const (
	ClassI   = "i"
	ClassII  = "ii"
	ClassIII = "iii"
)

// Fitting rule names (FitIssue.Rule).
const (
	RuleMount = "mount"
	RuleGrid  = "grid"
	RuleCPU   = "cpu"
)

var (
	mountKinds    = map[string]bool{MountTurret: true, MountLauncher: true}
	mountSizes    = map[string]int{SizeS: 0, SizeM: 1, SizeL: 2}
	moduleClasses = map[string]bool{ClassI: true, ClassII: true, ClassIII: true}
)

// ClassLabel is the display form of a module class: "Class I".."Class III".
func ClassLabel(class string) string {
	switch class {
	case ClassI:
		return "Class I"
	case ClassII:
		return "Class II"
	case ClassIII:
		return "Class III"
	}
	return ""
}

// Mount is one high slot: a turret or launcher of a size class
// (ShipDef.Mounts, JSON "high_mounts").
type Mount struct {
	Kind string `json:"kind"` // turret | launcher
	Size string `json:"size"` // s | m | l
}

// ClassQuota is a hull's allowance per module class on one axis
// (ShipDef.GridClasses / ShipDef.CPUClasses).
type ClassQuota struct {
	I   int `json:"i,omitempty"`
	II  int `json:"ii,omitempty"`
	III int `json:"iii,omitempty"`
}

// Count returns the allowance for class (0 for an unknown or empty class).
func (q ClassQuota) Count(class string) int {
	switch class {
	case ClassI:
		return q.I
	case ClassII:
		return q.II
	case ClassIII:
		return q.III
	}
	return 0
}

// FitIssue names the fitting rule a module breaks on a hull: the mount rule
// (RuleMount, Kind = the module's mount kind) or a class quota (RuleGrid or
// RuleCPU, Kind = the class). Oversize marks a mount failure where the hull
// does carry mounts of that kind but all are smaller than the module.
type FitIssue struct {
	Rule     string
	Kind     string
	Oversize bool
}

// axisQuota returns the hull's class allowance on a fitting axis.
func axisQuota(s *ShipDef, axis string) ClassQuota {
	if axis == RuleCPU {
		return s.CPUClasses
	}
	return s.GridClasses
}

// FitProblem reports the first fitting rule def would break alongside the
// already-fitted modules on the hull; nil means the module fits. Slot counts
// are the caller's concern.
func (s *ShipDef) FitProblem(def *ModuleDef, fitted []*ModuleDef) *FitIssue {
	if def.Slot == SlotHigh && def.Mount != "" {
		all := make([]*ModuleDef, 0, len(fitted)+1)
		all = append(all, fitted...)
		all = append(all, def)
		if !mountsFit(s.Mounts, all) {
			return &FitIssue{Rule: RuleMount, Kind: def.Mount, Oversize: oversizeFor(s.Mounts, def)}
		}
	}
	for _, axis := range []struct{ rule, class string }{{RuleGrid, def.GridClass}, {RuleCPU, def.CPUClass}} {
		if axis.class != "" && classDemand(fitted, axis.rule, axis.class)+1 > axisQuota(s, axis.rule).Count(axis.class) {
			return &FitIssue{Rule: axis.rule, Kind: axis.class}
		}
	}
	return nil
}

// mountsFit reports whether every mounted module can take its own mount of the
// same kind at or above its size. Modules without a mount kind (mid/low and
// legacy high modules) claim no mount. Greedy smallest-mount-first matching is
// exact for the single size dimension.
//
// shortcut: the rack is matched as a multiset — a high module's tile index is
// not its mount index, so inserting at an index may shift which tile sits on
// which mount; upgrade when the fitting UI pins modules to mount indices.
func mountsFit(mounts []Mount, mods []*ModuleDef) bool {
	for _, kind := range []string{MountTurret, MountLauncher} {
		var have, need []int
		for _, m := range mounts {
			if m.Kind == kind {
				have = append(have, mountSizes[m.Size])
			}
		}
		for _, m := range mods {
			if m.Mount == kind {
				need = append(need, mountSizes[m.Size])
			}
		}
		slices.Sort(have)
		slices.Sort(need)
		i := 0
		for _, n := range need {
			for i < len(have) && have[i] < n {
				i++
			}
			if i == len(have) {
				return false
			}
			i++
		}
	}
	return true
}

// oversizeFor reports whether the hull has mounts of the module's kind and
// every one of them is smaller than the module (the oversize case; an empty
// mount set is a missing-kind case instead).
func oversizeFor(mounts []Mount, def *ModuleDef) bool {
	seen := false
	for _, m := range mounts {
		if m.Kind != def.Mount {
			continue
		}
		seen = true
		if mountSizes[m.Size] >= mountSizes[def.Size] {
			return false
		}
	}
	return seen
}

// classDemand counts the fitted modules demanding class on one axis.
func classDemand(mods []*ModuleDef, axis, class string) int {
	n := 0
	for _, m := range mods {
		if axis == RuleGrid && m.GridClass == class || axis == RuleCPU && m.CPUClass == class {
			n++
		}
	}
	return n
}
