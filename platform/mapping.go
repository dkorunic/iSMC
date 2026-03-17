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
	Chip   string
	Year   int
}

// https://en.wikipedia.org/wiki/List_of_Mac_models
// https://github.com/exelban/stats/blob/master/Kit/plugins/SystemKit.swift
var products = map[string]Product{
	// Mac Mini
	"Macmini1,1": {Name: "Mac mini", Year: 2006, Family: "Intel"},
	"Macmini2,1": {Name: "Mac mini", Year: 2007, Family: "Intel"},
	"Macmini3,1": {Name: "Mac mini", Year: 2009, Family: "Intel"},
	"Macmini4,1": {Name: "Mac mini", Year: 2010, Family: "Intel"},
	"Macmini5,1": {Name: "Mac mini", Year: 2011, Family: "Intel"},
	"Macmini5,2": {Name: "Mac mini", Year: 2011, Family: "Intel"},
	"Macmini5,3": {Name: "Mac mini", Year: 2011, Family: "Intel"},
	"Macmini6,1": {Name: "Mac mini", Year: 2012, Family: "Intel"},
	"Macmini6,2": {Name: "Mac mini", Year: 2012, Family: "Intel"},
	"Macmini7,1": {Name: "Mac mini", Year: 2014, Family: "Intel"},
	"Macmini8,1": {Name: "Mac mini", Year: 2018, Family: "Intel"},
	"Macmini9,1": {Name: "Mac mini (M1)", Year: 2020, Family: "M1", Chip: "M1"},
	"Mac14,3":    {Name: "Mac mini (M2)", Year: 2023, Family: "M2", Chip: "M2"},
	"Mac14,12":   {Name: "Mac mini (M2 Pro)", Year: 2023, Family: "M2", Chip: "M2 Pro"},
	"Mac16,10":   {Name: "Mac mini (M4)", Year: 2024, Family: "M4", Chip: "M4"},
	"Mac16,11":   {Name: "Mac mini (M4 Pro)", Year: 2024, Family: "M4", Chip: "M4 Pro"},

	// Mac Studio
	"Mac13,1":  {Name: "Mac Studio (M1 Max)", Year: 2022, Family: "M1", Chip: "M1 Max"},
	"Mac13,2":  {Name: "Mac Studio (M1 Ultra)", Year: 2022, Family: "M1", Chip: "M1 Ultra"},
	"Mac14,13": {Name: "Mac Studio (M2 Max)", Year: 2023, Family: "M2", Chip: "M2 Max"},
	"Mac14,14": {Name: "Mac Studio (M2 Ultra)", Year: 2023, Family: "M2", Chip: "M2 Ultra"},
	"Mac15,14": {Name: "Mac Studio (M3 Max)", Year: 2023, Family: "M3", Chip: "M3 Max"},
	"Mac16,9":  {Name: "Mac Studio (M4 Max)", Year: 2024, Family: "M4", Chip: "M4 Max"},

	// Mac Pro
	"MacPro1,1": {Name: "Mac Pro", Year: 2006, Family: "Intel"},
	"MacPro2,1": {Name: "Mac Pro", Year: 2007, Family: "Intel"},
	"MacPro3,1": {Name: "Mac Pro", Year: 2008, Family: "Intel"},
	"MacPro4,1": {Name: "Mac Pro", Year: 2009, Family: "Intel"},
	"MacPro5,1": {Name: "Mac Pro", Year: 2010, Family: "Intel"},
	"MacPro6,1": {Name: "Mac Pro", Year: 2016, Family: "Intel"},
	"MacPro7,1": {Name: "Mac Pro", Year: 2019, Family: "Intel"},
	"Mac14,8":   {Name: "Mac Pro (M2 Ultra)", Year: 2023, Family: "M2", Chip: "M2 Ultra"},

	// iMac
	"iMac10,1": {Name: "iMac 21.5-Inch", Year: 2009, Family: "Intel"},
	"iMac11,2": {Name: "iMac 21.5-Inch", Year: 2010, Family: "Intel"},
	"iMac11,3": {Name: "iMac 27-Inch", Year: 2010, Family: "Intel"},
	"iMac12,1": {Name: "iMac 21.5-Inch", Year: 2011, Family: "Intel"},
	"iMac12,2": {Name: "iMac 27-Inch", Year: 2011, Family: "Intel"},
	"iMac13,1": {Name: "iMac 21.5-Inch", Year: 2012, Family: "Intel"},
	"iMac13,2": {Name: "iMac 27-Inch", Year: 2012, Family: "Intel"},
	"iMac14,1": {Name: "iMac 21.5-Inch", Year: 2013, Family: "Intel"},
	"iMac14,2": {Name: "iMac 27-Inch", Year: 2013, Family: "Intel"},
	"iMac14,3": {Name: "iMac 21.5-Inch", Year: 2013, Family: "Intel"},
	"iMac14,4": {Name: "iMac 21.5-Inch", Year: 2014, Family: "Intel"},
	"iMac15,1": {Name: "iMac 27-Inch", Year: 2014, Family: "Intel"},
	"iMac16,1": {Name: "iMac 21.5-Inch", Year: 2015, Family: "Intel"},
	"iMac16,2": {Name: "iMac 21.5-Inch", Year: 2015, Family: "Intel"},
	"iMac17,1": {Name: "iMac 27-Inch", Year: 2015, Family: "Intel"},
	"iMac18,1": {Name: "iMac 21.5-Inch", Year: 2017, Family: "Intel"},
	"iMac18,2": {Name: "iMac 21.5-Inch", Year: 2017, Family: "Intel"},
	"iMac18,3": {Name: "iMac 27-Inch", Year: 2017, Family: "Intel"},
	"iMac19,1": {Name: "iMac 27-Inch", Year: 2019, Family: "Intel"},
	"iMac19,2": {Name: "iMac 21.5-Inch", Year: 2019, Family: "Intel"},
	"iMac20,1": {Name: "iMac 27-Inch", Year: 2020, Family: "Intel"},
	"iMac20,2": {Name: "iMac 27-Inch", Year: 2020, Family: "Intel"},
	"iMac21,1": {Name: "iMac 24-Inch (M1)", Year: 2021, Family: "M1", Chip: "M1"},
	"iMac21,2": {Name: "iMac 24-Inch (M1)", Year: 2021, Family: "M1", Chip: "M1"},
	"Mac15,4":  {Name: "iMac 24-Inch (M3, 8 CPU/8 GPU)", Year: 2023, Family: "M3", Chip: "M3"},
	"Mac15,5":  {Name: "iMac 24-Inch (M3, 8 CPU/10 GPU)", Year: 2023, Family: "M3", Chip: "M3"},
	"Mac16,2":  {Name: "iMac 24-Inch (M4, 8 CPU/8 GPU)", Year: 2024, Family: "M4", Chip: "M4"},
	"Mac16,3":  {Name: "iMac 24-Inch (M4, 10 CPU/10 GPU)", Year: 2024, Family: "M4", Chip: "M4"},

	// iMac Pro
	"iMacPro1,1": {Name: "iMac Pro", Year: 2017, Family: "Intel"},

	// MacBook
	"MacBook8,1":  {Name: "MacBook", Year: 2015, Family: "Intel"},
	"MacBook9,1":  {Name: "MacBook", Year: 2016, Family: "Intel"},
	"MacBook10,1": {Name: "MacBook", Year: 2017, Family: "Intel"},

	// MacBook Neo
	"Mac17,5": {Name: "MacBook Neo", Year: 2026, Family: "A18", Chip: "A18 Pro"},

	// MacBook Air
	"MacBookAir1,1":  {Name: "MacBook Air 13", Year: 2008, Family: "Intel"},
	"MacBookAir2,1":  {Name: "MacBook Air 13", Year: 2009, Family: "Intel"},
	"MacBookAir3,1":  {Name: "MacBook Air 11", Year: 2010, Family: "Intel"},
	"MacBookAir3,2":  {Name: "MacBook Air 13", Year: 2010, Family: "Intel"},
	"MacBookAir4,1":  {Name: "MacBook Air 11", Year: 2011, Family: "Intel"},
	"MacBookAir4,2":  {Name: "MacBook Air 13", Year: 2011, Family: "Intel"},
	"MacBookAir5,1":  {Name: "MacBook Air 11", Year: 2012, Family: "Intel"},
	"MacBookAir5,2":  {Name: "MacBook Air 13", Year: 2012, Family: "Intel"},
	"MacBookAir6,1":  {Name: "MacBook Air 11", Year: 2014, Family: "Intel"},
	"MacBookAir6,2":  {Name: "MacBook Air 13", Year: 2014, Family: "Intel"},
	"MacBookAir7,1":  {Name: "MacBook Air 11", Year: 2015, Family: "Intel"},
	"MacBookAir7,2":  {Name: "MacBook Air 13", Year: 2015, Family: "Intel"},
	"MacBookAir8,1":  {Name: "MacBook Air 13", Year: 2018, Family: "Intel"},
	"MacBookAir8,2":  {Name: "MacBook Air 13", Year: 2019, Family: "Intel"},
	"MacBookAir9,1":  {Name: "MacBook Air 13", Year: 2020, Family: "Intel"},
	"MacBookAir10,1": {Name: "MacBook Air 13 (M1)", Year: 2020, Family: "M1", Chip: "M1"},
	"Mac14,2":        {Name: "MacBook Air 13 (M2)", Year: 2022, Family: "M2", Chip: "M2"},
	"Mac14,15":       {Name: "MacBook Air 15 (M2)", Year: 2023, Family: "M2", Chip: "M2"},
	"Mac15,12":       {Name: "MacBook Air 13 (M3)", Year: 2024, Family: "M3", Chip: "M3"},
	"Mac15,13":       {Name: "MacBook Air 15 (M3)", Year: 2024, Family: "M3", Chip: "M3"},
	"Mac16,12":       {Name: "MacBook Air 13 (M4)", Year: 2025, Family: "M4", Chip: "M4"},
	"Mac16,13":       {Name: "MacBook Air 15 (M4)", Year: 2025, Family: "M4", Chip: "M4"},
	"Mac17,2":        {Name: "MacBook Air 14 (M5)", Year: 2026, Family: "M5", Chip: "M5"},
	"Mac17,3":        {Name: "MacBook Air 13 (M5)", Year: 2026, Family: "M5", Chip: "M5"},
	"Mac17,4":        {Name: "MacBook Air 15 (M5)", Year: 2026, Family: "M5", Chip: "M5"},

	// MacBook Pro
	"MacBookPro1,1":  {Name: "MacBook Pro 15", Year: 2006, Family: "Intel"},
	"MacBookPro1,2":  {Name: "MacBook Pro 17", Year: 2006, Family: "Intel"},
	"MacBookPro2,1":  {Name: "MacBook Pro 17", Year: 2006, Family: "Intel"},
	"MacBookPro2,2":  {Name: "MacBook Pro 15", Year: 2006, Family: "Intel"},
	"MacBookPro3,1":  {Name: "MacBook Pro", Year: 2007, Family: "Intel"},
	"MacBookPro4,1":  {Name: "MacBook Pro", Year: 2008, Family: "Intel"},
	"MacBookPro5,1":  {Name: "MacBook Pro 15", Year: 2008, Family: "Intel"},
	"MacBookPro5,2":  {Name: "MacBook Pro 17", Year: 2009, Family: "Intel"},
	"MacBookPro5,3":  {Name: "MacBook Pro 15", Year: 2009, Family: "Intel"},
	"MacBookPro5,4":  {Name: "MacBook Pro 15", Year: 2009, Family: "Intel"},
	"MacBookPro5,5":  {Name: "MacBook Pro 13", Year: 2009, Family: "Intel"},
	"MacBookPro6,1":  {Name: "MacBook Pro 17", Year: 2010, Family: "Intel"},
	"MacBookPro6,2":  {Name: "MacBook Pro 15", Year: 2010, Family: "Intel"},
	"MacBookPro7,1":  {Name: "MacBook Pro 13", Year: 2010, Family: "Intel"},
	"MacBookPro8,1":  {Name: "MacBook Pro 13", Year: 2011, Family: "Intel"},
	"MacBookPro8,2":  {Name: "MacBook Pro 15", Year: 2011, Family: "Intel"},
	"MacBookPro8,3":  {Name: "MacBook Pro 17", Year: 2011, Family: "Intel"},
	"MacBookPro9,1":  {Name: "MacBook Pro 15", Year: 2012, Family: "Intel"},
	"MacBookPro9,2":  {Name: "MacBook Pro 13", Year: 2012, Family: "Intel"},
	"MacBookPro10,1": {Name: "MacBook Pro 15", Year: 2012, Family: "Intel"},
	"MacBookPro10,2": {Name: "MacBook Pro 13", Year: 2012, Family: "Intel"},
	"MacBookPro11,1": {Name: "MacBook Pro 13", Year: 2014, Family: "Intel"},
	"MacBookPro11,2": {Name: "MacBook Pro 15", Year: 2014, Family: "Intel"},
	"MacBookPro11,3": {Name: "MacBook Pro 15", Year: 2014, Family: "Intel"},
	"MacBookPro11,4": {Name: "MacBook Pro 15", Year: 2015, Family: "Intel"},
	"MacBookPro11,5": {Name: "MacBook Pro 15", Year: 2015, Family: "Intel"},
	"MacBookPro12,1": {Name: "MacBook Pro 13", Year: 2015, Family: "Intel"},
	"MacBookPro13,1": {Name: "MacBook Pro 13", Year: 2016, Family: "Intel"},
	"MacBookPro13,2": {Name: "MacBook Pro 13", Year: 2016, Family: "Intel"},
	"MacBookPro13,3": {Name: "MacBook Pro 15", Year: 2016, Family: "Intel"},
	"MacBookPro14,1": {Name: "MacBook Pro 13", Year: 2017, Family: "Intel"},
	"MacBookPro14,2": {Name: "MacBook Pro 13", Year: 2017, Family: "Intel"},
	"MacBookPro14,3": {Name: "MacBook Pro 15", Year: 2017, Family: "Intel"},
	"MacBookPro15,1": {Name: "MacBook Pro 15", Year: 2018, Family: "Intel"},
	"MacBookPro15,2": {Name: "MacBook Pro 13", Year: 2019, Family: "Intel"},
	"MacBookPro15,3": {Name: "MacBook Pro 15", Year: 2019, Family: "Intel"},
	"MacBookPro15,4": {Name: "MacBook Pro 13", Year: 2019, Family: "Intel"},
	"MacBookPro16,1": {Name: "MacBook Pro 16", Year: 2019, Family: "Intel"},
	"MacBookPro16,2": {Name: "MacBook Pro 13", Year: 2019, Family: "Intel"},
	"MacBookPro16,3": {Name: "MacBook Pro 13", Year: 2020, Family: "Intel"},
	"MacBookPro16,4": {Name: "MacBook Pro 16", Year: 2019, Family: "Intel"},
	"MacBookPro17,1": {Name: "MacBook Pro 13 (M1)", Year: 2020, Family: "M1", Chip: "M1"},
	"MacBookPro18,1": {Name: "MacBook Pro 16 (M1 Pro)", Year: 2021, Family: "M1", Chip: "M1 Pro"},
	"MacBookPro18,2": {Name: "MacBook Pro 16 (M1 Max)", Year: 2021, Family: "M1", Chip: "M1 Max"},
	"MacBookPro18,3": {Name: "MacBook Pro 14 (M1 Pro)", Year: 2021, Family: "M1", Chip: "M1 Pro"},
	"MacBookPro18,4": {Name: "MacBook Pro 14 (M1 Max)", Year: 2021, Family: "M1", Chip: "M1 Max"},
	"Mac14,7":        {Name: "MacBook Pro 13 (M2)", Year: 2022, Family: "M2", Chip: "M2"},
	"Mac14,5":        {Name: "MacBook Pro 14 (M2 Max)", Year: 2023, Family: "M2", Chip: "M2 Max"},
	"Mac14,6":        {Name: "MacBook Pro 16 (M2 Max)", Year: 2023, Family: "M2", Chip: "M2 Max"},
	"Mac14,9":        {Name: "MacBook Pro 14 (M2 Pro)", Year: 2023, Family: "M2", Chip: "M2 Pro"},
	"Mac14,10":       {Name: "MacBook Pro 16 (M2 Pro)", Year: 2023, Family: "M2", Chip: "M2 Pro"},
	"Mac15,3":        {Name: "MacBook Pro 14 (M3)", Year: 2023, Family: "M3", Chip: "M3"},
	"Mac15,6":        {Name: "MacBook Pro 14 (M3 Pro)", Year: 2023, Family: "M3", Chip: "M3 Pro"},
	"Mac15,7":        {Name: "MacBook Pro 16 (M3 Pro)", Year: 2023, Family: "M3", Chip: "M3 Pro"},
	"Mac15,8":        {Name: "MacBook Pro 14 (M3 Max)", Year: 2023, Family: "M3", Chip: "M3 Max"},
	"Mac15,9":        {Name: "MacBook Pro 16 (M3 Max)", Year: 2023, Family: "M3", Chip: "M3 Max"},
	"Mac15,10":       {Name: "MacBook Pro 14 (M3 Max)", Year: 2023, Family: "M3", Chip: "M3 Max"},
	"Mac16,1":        {Name: "MacBook Pro 14 (M4)", Year: 2024, Family: "M4", Chip: "M4"},
	"Mac16,5":        {Name: "MacBook Pro 16 (M4 Max)", Year: 2024, Family: "M4", Chip: "M4 Max"},
	"Mac16,6":        {Name: "MacBook Pro 14 (M4 Max)", Year: 2024, Family: "M4", Chip: "M4 Max"},
	"Mac16,7":        {Name: "MacBook Pro 16 (M4 Pro)", Year: 2024, Family: "M4", Chip: "M4 Pro"},
	"Mac16,8":        {Name: "MacBook Pro 14 (M4 Pro)", Year: 2024, Family: "M4", Chip: "M4 Pro"},
	"Mac17,6":        {Name: "MacBook Pro 16 (M5 Max)", Year: 2026, Family: "M5", Chip: "M5 Max"},
	"Mac17,7":        {Name: "MacBook Pro 14 (M5 Max)", Year: 2026, Family: "M5", Chip: "M5 Max"},
	"Mac17,8":        {Name: "MacBook Pro 16 (M5 Pro)", Year: 2026, Family: "M5", Chip: "M5 Pro"},
	"Mac17,9":        {Name: "MacBook Pro 14 (M5 Pro)", Year: 2026, Family: "M5", Chip: "M5 Pro"},
}
