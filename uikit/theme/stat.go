package theme

import "image/color"

// The stat language: every stat row the interface prints carries one glyph
// and one category colour, both looked up by the row's label. One table, so
// a new stat either lights up here or fails TestStatTaxonomyCoversCensus.

// StatCategory is a stat row's family; it picks the colour.
type StatCategory uint8

const (
	StatNone StatCategory = iota // identity rows: no glyph, no tint
	StatOffence
	StatDefence
	StatCapacitor
	StatMobility
	StatUtility
	StatMining
	StatEconomy
	StatFitting

	// The damage sub-system: one family per damage type, tinted by its own
	// palette hue and shared by damage, EHP and resist rows.
	StatEMDamage
	StatThermalDamage
	StatKineticDamage
	StatExplosiveDamage
)

// StatIconSize and StatIconGap are the row gutter: the glyph box and the air
// between it and the label (14 design units of gutter, the tooltip's cap).
const (
	StatIconSize = 10
	StatIconGap  = 4
)

// statStyle is one label's entry: the manifest glyph key and the family.
type statStyle struct {
	icon string
	cat  StatCategory
}

// statStyles is the census: every stat label the UI renders, keyed by the
// label exactly as the row prints it.
var statStyles = map[string]statStyle{
	// Offence
	"Per shot":  {icon: "ammo", cat: StatOffence},
	"Total DPS": {icon: "stat_burst", cat: StatOffence},
	"Optimal":   {icon: "stat_range", cat: StatOffence},
	"Falloff":   {icon: "stat_falloff", cat: StatOffence},
	"Tracking":  {icon: "stat_tracking", cat: StatOffence},

	// Defence
	"Shield":    {icon: "mod_defense", cat: StatDefence},
	"Armor":     {icon: "stat_armor", cat: StatDefence},
	"Structure": {icon: "stat_structure", cat: StatDefence},

	// Capacitor
	"Capacitor":    {icon: "stat_capacitor", cat: StatCapacitor},
	"Capacity":     {icon: "stat_capacitor", cat: StatCapacitor},
	"Restored":     {icon: "stat_capacitor", cat: StatCapacitor},
	"Cap use":      {icon: "stat_capacitor", cat: StatCapacitor},
	"Cap drain":    {icon: "stat_capacitor", cat: StatCapacitor},
	"Cap recharge": {icon: "stat_capacitor", cat: StatCapacitor},
	"Recharge":     {icon: "hud_time", cat: StatCapacitor},

	// Mobility
	"Max speed":    {icon: "stat_speed", cat: StatMobility},
	"Speed":        {icon: "stat_speed", cat: StatMobility},
	"Target speed": {icon: "stat_speed", cat: StatMobility},
	"Align time":   {icon: "stat_align", cat: StatMobility},
	"Align":        {icon: "stat_align", cat: StatMobility},
	"Mass":         {icon: "stat_mass", cat: StatMobility},
	"Warp speed":   {icon: "stat_warp", cat: StatMobility},
	"Warp":         {icon: "stat_warp", cat: StatMobility},

	// Utility
	"Sig radius":       {icon: "stat_signature", cat: StatUtility},
	"Signature":        {icon: "stat_signature", cat: StatUtility},
	"Scan resolution":  {icon: "svc_scanner", cat: StatUtility},
	"Scan":             {icon: "svc_scanner", cat: StatUtility},
	"Max target range": {icon: "stat_range", cat: StatUtility},
	"Max targets":      {icon: "act_lock", cat: StatUtility},
	"Targets":          {icon: "act_lock", cat: StatUtility},
	"Warp scramble":    {icon: "mod_scram", cat: StatUtility},
	"Aggro range":      {icon: "stat_range", cat: StatUtility},
	"Orbit range":      {icon: "act_orbit", cat: StatUtility},
	"Flee threshold":   {icon: "stat_flee", cat: StatUtility},

	// Mining
	"Yield":     {icon: "mod_mining", cat: StatMining},
	"Per cycle": {icon: "mod_mining", cat: StatMining},
	"Ore hold":  {icon: "stat_cargo", cat: StatMining},

	// Economy
	"Price":              {icon: "svc_market", cat: StatEconomy},
	"Base price":         {icon: "svc_market", cat: StatEconomy},
	"Bounty":             {icon: "stat_cash", cat: StatEconomy},
	"Cash":               {icon: "stat_cash", cat: StatEconomy},
	"Receivables":        {icon: "stat_cash", cat: StatEconomy},
	"Deployed in orders": {icon: "stat_cash", cat: StatEconomy},
	"Total":              {icon: "stat_cash", cat: StatEconomy},
	"Cargo":              {icon: "stat_cargo", cat: StatEconomy},

	// Fitting
	"High":        {icon: "row_module", cat: StatFitting},
	"Mid":         {icon: "row_module", cat: StatFitting},
	"Low":         {icon: "row_module", cat: StatFitting},
	"Slot":        {icon: "row_module", cat: StatFitting},
	"Grid class":  {icon: "stat_grid", cat: StatFitting},
	"CPU class":   {icon: "stat_cpu", cat: StatFitting},
	"Mount":       {icon: "mod_weapon", cat: StatFitting},
	"Mounts":      {icon: "mod_weapon", cat: StatFitting},
	"Size":        {icon: "stat_size", cat: StatFitting},
	"Volume":      {icon: "stat_cargo", cat: StatFitting},
	"Cycle":       {icon: "hud_time", cat: StatFitting},
	"Base time":   {icon: "hud_time", cat: StatFitting},
	"Clip":        {icon: "ammo", cat: StatFitting},
	"Loaded":      {icon: "ammo", cat: StatFitting},
	"In cargo":    {icon: "stat_cargo", cat: StatFitting},
	"Grid quotas": {icon: "stat_grid", cat: StatFitting},
	"CPU quotas":  {icon: "stat_cpu", cat: StatFitting},

	// Damage sub-system: one glyph per type, shared by damage, EHP and
	// resist rows.
	"EM":               {icon: "stat_em", cat: StatEMDamage},
	"Electro-magnetic": {icon: "stat_em", cat: StatEMDamage},
	"EM resist":        {icon: "stat_em", cat: StatEMDamage},
	"Thermal":          {icon: "stat_thermal", cat: StatThermalDamage},
	"Thermal resist":   {icon: "stat_thermal", cat: StatThermalDamage},
	"Kinetic":          {icon: "stat_kinetic", cat: StatKineticDamage},
	"Kinetic resist":   {icon: "stat_kinetic", cat: StatKineticDamage},
	"Explosive":        {icon: "stat_explosive", cat: StatExplosiveDamage},
	"Explosive resist": {icon: "stat_explosive", cat: StatExplosiveDamage},
}

// statIdentity is the explicit iconless allowlist: rows that name the thing
// rather than measure it.
var statIdentity = map[string]bool{
	"Class": true, "Maker": true, "Origin": true, "Family": true,
	"Faction": true, "Hull": true, "Kind": true, "Category": true, "Qty": true,
}

// sectionStyles is the section-bar map: the bar's glyph and tint.
var sectionStyles = map[string]statStyle{
	"FITTING":           {icon: "row_module", cat: StatFitting},
	"SLOTS":             {icon: "row_module", cat: StatFitting},
	"SURVIVAL":          {icon: "mod_defense", cat: StatDefence},
	"CAPACITOR":         {icon: "stat_capacitor", cat: StatCapacitor},
	"PROPULSION":        {icon: "stat_speed", cat: StatMobility},
	"SENSORS":           {icon: "svc_scanner", cat: StatUtility},
	"CARGO":             {icon: "stat_cargo", cat: StatEconomy},
	"ACTIVATION":        {icon: "row_module", cat: StatFitting},
	"DAMAGE":            {icon: "stat_burst", cat: StatOffence},
	"RANGE":             {icon: "stat_range", cat: StatOffence},
	"FIRING":            {icon: "stat_tracking", cat: StatOffence},
	"CHARGE":            {icon: "ammo", cat: StatFitting},
	"CHARGES":           {icon: "ammo", cat: StatFitting},
	"YIELD":             {icon: "mod_mining", cat: StatMining},
	"REPAIR":            {icon: "mod_defense", cat: StatDefence},
	"EFFECT":            {icon: "mod_utility", cat: StatUtility},
	"BONUSES":           {icon: "stat_burst", cat: StatOffence},
	"MARKET":            {icon: "svc_market", cat: StatEconomy},
	"REFINES TO":        {icon: "mod_mining", cat: StatMining},
	"BEHAVIOR":          {icon: "svc_scanner", cat: StatUtility},
	"LOOT":              {icon: "act_loot", cat: StatEconomy},
	"CONSTRUCTION":      {cat: StatNone},
	"MANUFACTURED FROM": {cat: StatNone},
	"USED IN":           {cat: StatNone},
	"PROPERTIES":        {cat: StatNone},
	"DOSSIER":           {cat: StatNone},
}

// StatIcon is label's glyph key ("" for an identity row or an unknown label).
func StatIcon(label string) string { return statStyles[label].icon }

// StatCategoryOf is label's family (StatNone for identity rows).
func StatCategoryOf(label string) StatCategory { return statStyles[label].cat }

// StatTint is label's category colour (zero for an identity row).
func StatTint(p Palette, label string) color.RGBA { return StatColor(p, StatCategoryOf(label)) }

// StatColor resolves a category to its palette role.
func StatColor(p Palette, c StatCategory) color.RGBA {
	switch c {
	case StatOffence:
		return p.StatOffence
	case StatDefence:
		return p.Shield
	case StatCapacitor:
		return p.Cap
	case StatMobility:
		return p.StatMobility
	case StatUtility:
		return p.Info
	case StatMining:
		return p.Warn
	case StatEconomy:
		return p.Good
	case StatFitting:
		return p.Struct
	case StatEMDamage:
		return p.StatEM
	case StatThermalDamage:
		return p.StatThermal
	case StatKineticDamage:
		return p.StatKinetic
	case StatExplosiveDamage:
		return p.StatExplosive
	}
	return color.RGBA{}
}

// SectionBar is a section's bar glyph and tint: the section's own entry when
// it has one, else the grey identity rule (iconless).
func SectionBar(p Palette, label string) (icon string, tint color.RGBA) {
	s := sectionStyles[label]
	if s.icon == "" {
		return "", p.Border
	}
	return s.icon, StatColor(p, s.cat)
}
