// Copyright (C) 2026  Dinko Korunic
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, version 3.
//
// This program is distributed in the hope that it will be useful, but
// WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
// General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.

//go:build darwin

package platform

type Product struct {
	Name   string
	Family string
	CPU    string
	Year   int
}

// Pair signatures describe which two core types coexist on an Apple Silicon SKU.
// Driven by the validate-temp-mappings family roster: M1–M4 and A18 use P+E; M5
// base uses S+E; M5 Pro/Max use S+P. The same Tp* SMC prefix maps to different
// physical core types depending on this signature, so any consumer that names
// per-core sensors must consult it (cmd/guess.go does so when labelling phases).
const (
	PairSignaturePE = "P+E" // Performance + Efficiency (M1–M4, A18, all variants)
	PairSignatureSE = "S+E" // Super + Efficiency (M5 base only)
	PairSignatureSP = "S+P" // Super + Performance (M5 Pro / M5 Max)
)

// SKULayout describes the per-SKU physical core composition, GPU core counts, die
// count, and pair signature for an Apple Silicon variant. Sourced from the
// validate-temp-mappings family roster (.claude/skills/validate-temp-mappings).
//
// Multiple field configurations exist for many SKUs (e.g. M1 Pro: 6P+2E or 8P+2E,
// M3 Max: 10P+4E or 12P+4E). The {Min,Max} pairs span every observed variant, so
// a runtime-detected count is consistent with the SKU iff it falls inside the
// inclusive range. A zero value for a core type means that type is absent on the
// SKU (e.g. M5 Pro has no E-cores → ECoresMin = ECoresMax = 0).
//
// PairSignature uses one of the PairSignature* constants. Dies is 1 for monolithic
// chips and 2 for UltraFusion SKUs (M1/M2/M3 Ultra) — die count drives the
// "look for parallel die-2 sensor sets" expectation in validators.
type SKULayout struct {
	PairSignature            string
	PCoresMin, PCoresMax     int
	ECoresMin, ECoresMax     int
	SCoresMin, SCoresMax     int
	GPUCoresMin, GPUCoresMax int
	Dies                     int
}

// skuLayouts maps an Apple Silicon SKU (the Product.CPU string) to its known core
// composition. This is the on-device counterpart of the validate-temp-mappings
// family roster and is the single source of truth used by cmd/guess.go for phase
// labelling and detected-count validation.
//
// When a new chip is announced: add an entry here, add the corresponding model
// IDs to the products map above, and (only if the family tag introduces a new
// pair signature) add a temp.txt sub-family split per the skill's procedure.
var skuLayouts = map[string]SKULayout{
	// M1 family — P+E
	"M1":       {PCoresMin: 4, PCoresMax: 4, ECoresMin: 4, ECoresMax: 4, GPUCoresMin: 7, GPUCoresMax: 8, PairSignature: PairSignaturePE, Dies: 1},
	"M1 Pro":   {PCoresMin: 6, PCoresMax: 8, ECoresMin: 2, ECoresMax: 2, GPUCoresMin: 14, GPUCoresMax: 16, PairSignature: PairSignaturePE, Dies: 1},
	"M1 Max":   {PCoresMin: 8, PCoresMax: 8, ECoresMin: 2, ECoresMax: 2, GPUCoresMin: 24, GPUCoresMax: 32, PairSignature: PairSignaturePE, Dies: 1},
	"M1 Ultra": {PCoresMin: 16, PCoresMax: 16, ECoresMin: 4, ECoresMax: 4, GPUCoresMin: 48, GPUCoresMax: 64, PairSignature: PairSignaturePE, Dies: 2},

	// M2 family — P+E
	"M2":       {PCoresMin: 4, PCoresMax: 4, ECoresMin: 4, ECoresMax: 4, GPUCoresMin: 8, GPUCoresMax: 10, PairSignature: PairSignaturePE, Dies: 1},
	"M2 Pro":   {PCoresMin: 6, PCoresMax: 8, ECoresMin: 4, ECoresMax: 4, GPUCoresMin: 16, GPUCoresMax: 19, PairSignature: PairSignaturePE, Dies: 1},
	"M2 Max":   {PCoresMin: 8, PCoresMax: 8, ECoresMin: 4, ECoresMax: 4, GPUCoresMin: 30, GPUCoresMax: 38, PairSignature: PairSignaturePE, Dies: 1},
	"M2 Ultra": {PCoresMin: 16, PCoresMax: 16, ECoresMin: 8, ECoresMax: 8, GPUCoresMin: 60, GPUCoresMax: 76, PairSignature: PairSignaturePE, Dies: 2},

	// M3 family — P+E
	"M3":       {PCoresMin: 4, PCoresMax: 4, ECoresMin: 4, ECoresMax: 4, GPUCoresMin: 8, GPUCoresMax: 10, PairSignature: PairSignaturePE, Dies: 1},
	"M3 Pro":   {PCoresMin: 5, PCoresMax: 6, ECoresMin: 6, ECoresMax: 6, GPUCoresMin: 14, GPUCoresMax: 18, PairSignature: PairSignaturePE, Dies: 1},
	"M3 Max":   {PCoresMin: 10, PCoresMax: 12, ECoresMin: 4, ECoresMax: 4, GPUCoresMin: 30, GPUCoresMax: 40, PairSignature: PairSignaturePE, Dies: 1},
	"M3 Ultra": {PCoresMin: 20, PCoresMax: 24, ECoresMin: 8, ECoresMax: 8, GPUCoresMin: 60, GPUCoresMax: 80, PairSignature: PairSignaturePE, Dies: 2},

	// M4 family — P+E
	"M4":     {PCoresMin: 3, PCoresMax: 4, ECoresMin: 4, ECoresMax: 6, GPUCoresMin: 8, GPUCoresMax: 10, PairSignature: PairSignaturePE, Dies: 1},
	"M4 Pro": {PCoresMin: 8, PCoresMax: 10, ECoresMin: 4, ECoresMax: 4, GPUCoresMin: 16, GPUCoresMax: 20, PairSignature: PairSignaturePE, Dies: 1},
	"M4 Max": {PCoresMin: 10, PCoresMax: 12, ECoresMin: 4, ECoresMax: 4, GPUCoresMin: 32, GPUCoresMax: 40, PairSignature: PairSignaturePE, Dies: 1},

	// M5 family — base = S+E, Pro/Max = S+P. Two distinct pair signatures share
	// the "M5" family tag; do not collapse without also splitting temp.txt.
	"M5":     {SCoresMin: 3, SCoresMax: 4, ECoresMin: 6, ECoresMax: 6, GPUCoresMin: 8, GPUCoresMax: 10, PairSignature: PairSignatureSE, Dies: 1},
	"M5 Pro": {SCoresMin: 5, SCoresMax: 6, PCoresMin: 10, PCoresMax: 12, GPUCoresMin: 16, GPUCoresMax: 20, PairSignature: PairSignatureSP, Dies: 1},
	"M5 Max": {SCoresMin: 6, SCoresMax: 6, PCoresMin: 12, PCoresMax: 12, GPUCoresMin: 32, GPUCoresMax: 40, PairSignature: PairSignatureSP, Dies: 1},

	// A-series
	"A18 Pro": {PCoresMin: 2, PCoresMax: 2, ECoresMin: 4, ECoresMax: 4, GPUCoresMin: 6, GPUCoresMax: 6, PairSignature: PairSignaturePE, Dies: 1},
}

// https://en.wikipedia.org/wiki/List_of_Mac_models
// https://github.com/exelban/stats/blob/master/Kit/plugins/SystemKit.swift
var products = map[string]Product{
	// Mac Mini
	"Macmini1,1": {Name: "Mac mini", Year: 2006, Family: "Intel", CPU: "Core Duo T2300"},
	"Macmini2,1": {Name: "Mac mini", Year: 2007, Family: "Intel", CPU: "Core 2 Duo T5600"},
	"Macmini3,1": {Name: "Mac mini", Year: 2009, Family: "Intel", CPU: "Core 2 Duo P7350"},
	"Macmini4,1": {Name: "Mac mini", Year: 2010, Family: "Intel", CPU: "Core 2 Duo P8600"},
	"Macmini5,1": {Name: "Mac mini", Year: 2011, Family: "Intel", CPU: "Core i5-2415M"},
	"Macmini5,2": {Name: "Mac mini", Year: 2011, Family: "Intel", CPU: "Core i5-2520M"},
	"Macmini5,3": {Name: "Mac mini", Year: 2011, Family: "Intel", CPU: "Core i7-2635QM"},
	"Macmini6,1": {Name: "Mac mini", Year: 2012, Family: "Intel", CPU: "Core i5-3210M"},
	"Macmini6,2": {Name: "Mac mini", Year: 2012, Family: "Intel", CPU: "Core i7-3615QM"},
	"Macmini7,1": {Name: "Mac mini", Year: 2014, Family: "Intel", CPU: "Core i5-4260U"},
	"Macmini8,1": {Name: "Mac mini", Year: 2018, Family: "Intel", CPU: "Core i3-8100B"},
	"Macmini9,1": {Name: "Mac mini (M1)", Year: 2020, Family: "M1", CPU: "M1"},
	"Mac14,3":    {Name: "Mac mini (M2)", Year: 2023, Family: "M2", CPU: "M2"},
	"Mac14,12":   {Name: "Mac mini (M2 Pro)", Year: 2023, Family: "M2", CPU: "M2 Pro"},
	"Mac16,10":   {Name: "Mac mini (M4)", Year: 2024, Family: "M4", CPU: "M4"},
	"Mac16,11":   {Name: "Mac mini (M4 Pro)", Year: 2024, Family: "M4", CPU: "M4 Pro"},

	// Mac Studio
	"Mac13,1":  {Name: "Mac Studio (M1 Max)", Year: 2022, Family: "M1", CPU: "M1 Max"},
	"Mac13,2":  {Name: "Mac Studio (M1 Ultra)", Year: 2022, Family: "M1", CPU: "M1 Ultra"},
	"Mac14,13": {Name: "Mac Studio (M2 Max)", Year: 2023, Family: "M2", CPU: "M2 Max"},
	"Mac14,14": {Name: "Mac Studio (M2 Ultra)", Year: 2023, Family: "M2", CPU: "M2 Ultra"},
	"Mac15,14": {Name: "Mac Studio (M3 Ultra)", Year: 2025, Family: "M3", CPU: "M3 Ultra"},
	"Mac16,9":  {Name: "Mac Studio (M4 Max)", Year: 2025, Family: "M4", CPU: "M4 Max"},

	// Mac Pro
	"MacPro1,1": {Name: "Mac Pro", Year: 2006, Family: "Intel", CPU: "Xeon 5130"},
	"MacPro2,1": {Name: "Mac Pro", Year: 2007, Family: "Intel", CPU: "Xeon X5365"},
	"MacPro3,1": {Name: "Mac Pro", Year: 2008, Family: "Intel", CPU: "Xeon E5462"},
	"MacPro4,1": {Name: "Mac Pro", Year: 2009, Family: "Intel", CPU: "Xeon W3520"},
	"MacPro5,1": {Name: "Mac Pro", Year: 2010, Family: "Intel", CPU: "Xeon W3530"},
	"MacPro6,1": {Name: "Mac Pro", Year: 2016, Family: "Intel", CPU: "Xeon E5-1620v2"},
	"MacPro7,1": {Name: "Mac Pro", Year: 2019, Family: "Intel", CPU: "Xeon W-3223"},
	"Mac14,8":   {Name: "Mac Pro (M2 Ultra)", Year: 2023, Family: "M2", CPU: "M2 Ultra"},

	// iMac
	"iMac10,1": {Name: "iMac 21.5-Inch", Year: 2009, Family: "Intel", CPU: "Core 2 Duo E7600"},
	"iMac11,2": {Name: "iMac 21.5-Inch", Year: 2010, Family: "Intel", CPU: "Core i3-540"},
	"iMac11,3": {Name: "iMac 27-Inch", Year: 2010, Family: "Intel", CPU: "Core i3-550"},
	"iMac12,1": {Name: "iMac 21.5-Inch", Year: 2011, Family: "Intel", CPU: "Core i5-2400S"},
	"iMac12,2": {Name: "iMac 27-Inch", Year: 2011, Family: "Intel", CPU: "Core i5-2500S"},
	"iMac13,1": {Name: "iMac 21.5-Inch", Year: 2012, Family: "Intel", CPU: "Core i5-3330S"},
	"iMac13,2": {Name: "iMac 27-Inch", Year: 2012, Family: "Intel", CPU: "Core i5-3470S"},
	"iMac14,1": {Name: "iMac 21.5-Inch", Year: 2013, Family: "Intel", CPU: "Core i5-4570R"},
	"iMac14,2": {Name: "iMac 27-Inch", Year: 2013, Family: "Intel", CPU: "Core i5-4570"},
	"iMac14,3": {Name: "iMac 21.5-Inch", Year: 2013, Family: "Intel", CPU: "Core i5-4570S"},
	"iMac14,4": {Name: "iMac 21.5-Inch", Year: 2014, Family: "Intel", CPU: "Core i5-4260U"},
	"iMac15,1": {Name: "iMac 27-Inch", Year: 2014, Family: "Intel", CPU: "Core i5-4590"},
	"iMac16,1": {Name: "iMac 21.5-Inch", Year: 2015, Family: "Intel", CPU: "Core i5-5250U"},
	"iMac16,2": {Name: "iMac 21.5-Inch", Year: 2015, Family: "Intel", CPU: "Core i5-5575R"},
	"iMac17,1": {Name: "iMac 27-Inch", Year: 2015, Family: "Intel", CPU: "Core i5-6500"},
	"iMac18,1": {Name: "iMac 21.5-Inch", Year: 2017, Family: "Intel", CPU: "Core i5-7360U"},
	"iMac18,2": {Name: "iMac 21.5-Inch", Year: 2017, Family: "Intel", CPU: "Core i5-7400"},
	"iMac18,3": {Name: "iMac 27-Inch", Year: 2017, Family: "Intel", CPU: "Core i5-7500"},
	"iMac19,1": {Name: "iMac 27-Inch", Year: 2019, Family: "Intel", CPU: "Core i5-8500"},
	"iMac19,2": {Name: "iMac 21.5-Inch", Year: 2019, Family: "Intel", CPU: "Core i3-8100"},
	"iMac20,1": {Name: "iMac 27-Inch", Year: 2020, Family: "Intel", CPU: "Core i5-10500"},
	"iMac20,2": {Name: "iMac 27-Inch", Year: 2020, Family: "Intel", CPU: "Core i7-10700K"},
	"iMac21,1": {Name: "iMac 24-Inch (M1)", Year: 2021, Family: "M1", CPU: "M1"},
	"iMac21,2": {Name: "iMac 24-Inch (M1)", Year: 2021, Family: "M1", CPU: "M1"},
	"Mac15,4":  {Name: "iMac 24-Inch (M3, 8 CPU/8 GPU)", Year: 2023, Family: "M3", CPU: "M3"},
	"Mac15,5":  {Name: "iMac 24-Inch (M3, 8 CPU/10 GPU)", Year: 2023, Family: "M3", CPU: "M3"},
	"Mac16,2":  {Name: "iMac 24-Inch (M4, 8 CPU/8 GPU)", Year: 2024, Family: "M4", CPU: "M4"},
	"Mac16,3":  {Name: "iMac 24-Inch (M4, 10 CPU/10 GPU)", Year: 2024, Family: "M4", CPU: "M4"},

	// iMac Pro
	"iMacPro1,1": {Name: "iMac Pro", Year: 2017, Family: "Intel", CPU: "Xeon W-2140B"},

	// MacBook
	"MacBook8,1":  {Name: "MacBook", Year: 2015, Family: "Intel", CPU: "Core M-5Y31"},
	"MacBook9,1":  {Name: "MacBook", Year: 2016, Family: "Intel", CPU: "Core m3-6Y30"},
	"MacBook10,1": {Name: "MacBook", Year: 2017, Family: "Intel", CPU: "Core m3-7Y32"},

	// MacBook Neo
	"Mac17,5": {Name: "MacBook Neo", Year: 2026, Family: "A18", CPU: "A18 Pro"},

	// MacBook Air
	"MacBookAir1,1":  {Name: "MacBook Air 13", Year: 2008, Family: "Intel", CPU: "Core 2 Duo P7500"},
	"MacBookAir2,1":  {Name: "MacBook Air 13", Year: 2009, Family: "Intel", CPU: "Core 2 Duo SL9300"},
	"MacBookAir3,1":  {Name: "MacBook Air 11", Year: 2010, Family: "Intel", CPU: "Core 2 Duo SU9400"},
	"MacBookAir3,2":  {Name: "MacBook Air 13", Year: 2010, Family: "Intel", CPU: "Core 2 Duo SL9400"},
	"MacBookAir4,1":  {Name: "MacBook Air 11", Year: 2011, Family: "Intel", CPU: "Core i5-2467M"},
	"MacBookAir4,2":  {Name: "MacBook Air 13", Year: 2011, Family: "Intel", CPU: "Core i5-2467M"},
	"MacBookAir5,1":  {Name: "MacBook Air 11", Year: 2012, Family: "Intel", CPU: "Core i5-3317U"},
	"MacBookAir5,2":  {Name: "MacBook Air 13", Year: 2012, Family: "Intel", CPU: "Core i5-3317U"},
	"MacBookAir6,1":  {Name: "MacBook Air 11", Year: 2014, Family: "Intel", CPU: "Core i5-4250U"},
	"MacBookAir6,2":  {Name: "MacBook Air 13", Year: 2014, Family: "Intel", CPU: "Core i5-4250U"},
	"MacBookAir7,1":  {Name: "MacBook Air 11", Year: 2015, Family: "Intel", CPU: "Core i5-5250U"},
	"MacBookAir7,2":  {Name: "MacBook Air 13", Year: 2015, Family: "Intel", CPU: "Core i5-5250U"},
	"MacBookAir8,1":  {Name: "MacBook Air 13", Year: 2018, Family: "Intel", CPU: "Core i5-8210Y"},
	"MacBookAir8,2":  {Name: "MacBook Air 13", Year: 2019, Family: "Intel", CPU: "Core i5-8210Y"},
	"MacBookAir9,1":  {Name: "MacBook Air 13", Year: 2020, Family: "Intel", CPU: "Core i3-1000NG4"},
	"MacBookAir10,1": {Name: "MacBook Air 13 (M1)", Year: 2020, Family: "M1", CPU: "M1"},
	"Mac14,2":        {Name: "MacBook Air 13 (M2)", Year: 2022, Family: "M2", CPU: "M2"},
	"Mac14,15":       {Name: "MacBook Air 15 (M2)", Year: 2023, Family: "M2", CPU: "M2"},
	"Mac15,12":       {Name: "MacBook Air 13 (M3)", Year: 2024, Family: "M3", CPU: "M3"},
	"Mac15,13":       {Name: "MacBook Air 15 (M3)", Year: 2024, Family: "M3", CPU: "M3"},
	"Mac16,12":       {Name: "MacBook Air 13 (M4)", Year: 2025, Family: "M4", CPU: "M4"},
	"Mac16,13":       {Name: "MacBook Air 15 (M4)", Year: 2025, Family: "M4", CPU: "M4"},
	"Mac17,3":        {Name: "MacBook Air 13 (M5)", Year: 2026, Family: "M5", CPU: "M5"},
	"Mac17,4":        {Name: "MacBook Air 15 (M5)", Year: 2026, Family: "M5", CPU: "M5"},

	// MacBook Pro
	"MacBookPro1,1":  {Name: "MacBook Pro 15", Year: 2006, Family: "Intel", CPU: "Core Duo L2400"},
	"MacBookPro1,2":  {Name: "MacBook Pro 17", Year: 2006, Family: "Intel", CPU: "Core Duo T2600"},
	"MacBookPro2,1":  {Name: "MacBook Pro 17", Year: 2006, Family: "Intel", CPU: "Core 2 Duo T7600"},
	"MacBookPro2,2":  {Name: "MacBook Pro 15", Year: 2006, Family: "Intel", CPU: "Core 2 Duo T7400"},
	"MacBookPro3,1":  {Name: "MacBook Pro", Year: 2007, Family: "Intel", CPU: "Core 2 Duo T7500"},
	"MacBookPro4,1":  {Name: "MacBook Pro", Year: 2008, Family: "Intel", CPU: "Core 2 Duo T8300"},
	"MacBookPro5,1":  {Name: "MacBook Pro 15", Year: 2008, Family: "Intel", CPU: "Core 2 Duo P8600"},
	"MacBookPro5,2":  {Name: "MacBook Pro 17", Year: 2009, Family: "Intel", CPU: "Core 2 Duo T9550"},
	"MacBookPro5,3":  {Name: "MacBook Pro 15", Year: 2009, Family: "Intel", CPU: "Core 2 Duo P8800"},
	"MacBookPro5,4":  {Name: "MacBook Pro 15", Year: 2009, Family: "Intel", CPU: "Core 2 Duo P8700"},
	"MacBookPro5,5":  {Name: "MacBook Pro 13", Year: 2009, Family: "Intel", CPU: "Core 2 Duo P7550"},
	"MacBookPro6,1":  {Name: "MacBook Pro 17", Year: 2010, Family: "Intel", CPU: "Core i5-540M"},
	"MacBookPro6,2":  {Name: "MacBook Pro 15", Year: 2010, Family: "Intel", CPU: "Core i5-520M"},
	"MacBookPro7,1":  {Name: "MacBook Pro 13", Year: 2010, Family: "Intel", CPU: "Core 2 Duo P8600"},
	"MacBookPro8,1":  {Name: "MacBook Pro 13", Year: 2011, Family: "Intel", CPU: "Core i5-2415M"},
	"MacBookPro8,2":  {Name: "MacBook Pro 15", Year: 2011, Family: "Intel", CPU: "Core i7-2635QM"},
	"MacBookPro8,3":  {Name: "MacBook Pro 17", Year: 2011, Family: "Intel", CPU: "Core i7-2720QM"},
	"MacBookPro9,1":  {Name: "MacBook Pro 15", Year: 2012, Family: "Intel", CPU: "Core i7-3615QM"},
	"MacBookPro9,2":  {Name: "MacBook Pro 13", Year: 2012, Family: "Intel", CPU: "Core i5-3210M"},
	"MacBookPro10,1": {Name: "MacBook Pro 15", Year: 2012, Family: "Intel", CPU: "Core i7-3615QM"},
	"MacBookPro10,2": {Name: "MacBook Pro 13", Year: 2012, Family: "Intel", CPU: "Core i5-3210M"},
	"MacBookPro11,1": {Name: "MacBook Pro 13", Year: 2014, Family: "Intel", CPU: "Core i5-4258U"},
	"MacBookPro11,2": {Name: "MacBook Pro 15", Year: 2014, Family: "Intel", CPU: "Core i7-4750HQ"},
	"MacBookPro11,3": {Name: "MacBook Pro 15", Year: 2014, Family: "Intel", CPU: "Core i7-4850HQ"},
	"MacBookPro11,4": {Name: "MacBook Pro 15", Year: 2015, Family: "Intel", CPU: "Core i7-4770HQ"},
	"MacBookPro11,5": {Name: "MacBook Pro 15", Year: 2015, Family: "Intel", CPU: "Core i7-4870HQ"},
	"MacBookPro12,1": {Name: "MacBook Pro 13", Year: 2015, Family: "Intel", CPU: "Core i5-5257U"},
	"MacBookPro13,1": {Name: "MacBook Pro 13", Year: 2016, Family: "Intel", CPU: "Core i5-6360U"},
	"MacBookPro13,2": {Name: "MacBook Pro 13", Year: 2016, Family: "Intel", CPU: "Core i5-6267U"},
	"MacBookPro13,3": {Name: "MacBook Pro 15", Year: 2016, Family: "Intel", CPU: "Core i7-6700HQ"},
	"MacBookPro14,1": {Name: "MacBook Pro 13", Year: 2017, Family: "Intel", CPU: "Core i5-7360U"},
	"MacBookPro14,2": {Name: "MacBook Pro 13", Year: 2017, Family: "Intel", CPU: "Core i5-7267U"},
	"MacBookPro14,3": {Name: "MacBook Pro 15", Year: 2017, Family: "Intel", CPU: "Core i7-7700HQ"},
	"MacBookPro15,1": {Name: "MacBook Pro 15", Year: 2018, Family: "Intel", CPU: "Core i7-8750H"},
	"MacBookPro15,2": {Name: "MacBook Pro 13", Year: 2019, Family: "Intel", CPU: "Core i5-8259U"},
	"MacBookPro15,3": {Name: "MacBook Pro 15", Year: 2019, Family: "Intel", CPU: "Core i7-8850H"},
	"MacBookPro15,4": {Name: "MacBook Pro 13", Year: 2019, Family: "Intel", CPU: "Core i5-8257U"},
	"MacBookPro16,1": {Name: "MacBook Pro 16", Year: 2019, Family: "Intel", CPU: "Core i7-9750H"},
	"MacBookPro16,2": {Name: "MacBook Pro 13", Year: 2019, Family: "Intel", CPU: "Core i5-1038NG7"},
	"MacBookPro16,3": {Name: "MacBook Pro 13", Year: 2020, Family: "Intel", CPU: "Core i5-8257U"},
	"MacBookPro16,4": {Name: "MacBook Pro 16", Year: 2019, Family: "Intel", CPU: "Core i7-9750H"},
	"MacBookPro17,1": {Name: "MacBook Pro 13 (M1)", Year: 2020, Family: "M1", CPU: "M1"},
	"MacBookPro18,1": {Name: "MacBook Pro 16 (M1 Pro)", Year: 2021, Family: "M1", CPU: "M1 Pro"},
	"MacBookPro18,2": {Name: "MacBook Pro 16 (M1 Max)", Year: 2021, Family: "M1", CPU: "M1 Max"},
	"MacBookPro18,3": {Name: "MacBook Pro 14 (M1 Pro)", Year: 2021, Family: "M1", CPU: "M1 Pro"},
	"MacBookPro18,4": {Name: "MacBook Pro 14 (M1 Max)", Year: 2021, Family: "M1", CPU: "M1 Max"},
	"Mac14,7":        {Name: "MacBook Pro 13 (M2)", Year: 2022, Family: "M2", CPU: "M2"},
	"Mac14,5":        {Name: "MacBook Pro 14 (M2 Max)", Year: 2023, Family: "M2", CPU: "M2 Max"},
	"Mac14,6":        {Name: "MacBook Pro 16 (M2 Max)", Year: 2023, Family: "M2", CPU: "M2 Max"},
	"Mac14,9":        {Name: "MacBook Pro 14 (M2 Pro)", Year: 2023, Family: "M2", CPU: "M2 Pro"},
	"Mac14,10":       {Name: "MacBook Pro 16 (M2 Pro)", Year: 2023, Family: "M2", CPU: "M2 Pro"},
	"Mac15,3":        {Name: "MacBook Pro 14 (M3)", Year: 2023, Family: "M3", CPU: "M3"},
	"Mac15,6":        {Name: "MacBook Pro 14 (M3 Pro)", Year: 2023, Family: "M3", CPU: "M3 Pro"},
	"Mac15,7":        {Name: "MacBook Pro 16 (M3 Pro)", Year: 2023, Family: "M3", CPU: "M3 Pro"},
	"Mac15,8":        {Name: "MacBook Pro 14 (M3 Max)", Year: 2023, Family: "M3", CPU: "M3 Max"},
	"Mac15,9":        {Name: "MacBook Pro 16 (M3 Max)", Year: 2023, Family: "M3", CPU: "M3 Max"},
	"Mac15,10":       {Name: "MacBook Pro 14 (M3 Max)", Year: 2023, Family: "M3", CPU: "M3 Max"},
	"Mac16,1":        {Name: "MacBook Pro 14 (M4)", Year: 2024, Family: "M4", CPU: "M4"},
	"Mac16,5":        {Name: "MacBook Pro 16 (M4 Max)", Year: 2024, Family: "M4", CPU: "M4 Max"},
	"Mac16,6":        {Name: "MacBook Pro 14 (M4 Max)", Year: 2024, Family: "M4", CPU: "M4 Max"},
	"Mac16,7":        {Name: "MacBook Pro 16 (M4 Pro)", Year: 2024, Family: "M4", CPU: "M4 Pro"},
	"Mac16,8":        {Name: "MacBook Pro 14 (M4 Pro)", Year: 2024, Family: "M4", CPU: "M4 Pro"},
	"Mac17,2":        {Name: "MacBook Pro 14 (M5)", Year: 2025, Family: "M5", CPU: "M5"},
	"Mac17,6":        {Name: "MacBook Pro 16 (M5 Max)", Year: 2026, Family: "M5", CPU: "M5 Max"},
	"Mac17,7":        {Name: "MacBook Pro 14 (M5 Max)", Year: 2026, Family: "M5", CPU: "M5 Max"},
	"Mac17,8":        {Name: "MacBook Pro 16 (M5 Pro)", Year: 2026, Family: "M5", CPU: "M5 Pro"},
	"Mac17,9":        {Name: "MacBook Pro 14 (M5 Pro)", Year: 2026, Family: "M5", CPU: "M5 Pro"},
}
