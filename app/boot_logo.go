package main

import (
	_ "embed"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Original terminal artwork inspired by a winged eagle medal. Keep the
// silhouettes as text assets so they work without images or special fonts.
//
//go:embed data/eagle-medal.txt
var eagleMedalLarge string

//go:embed data/eagle-medal-compact.txt
var eagleMedalCompact string

//go:embed data/eagle-medal-mini.txt
var eagleMedalMini string

func eagleMedalLogo(width, height int) []string {
	for _, art := range []string{eagleMedalLarge, eagleMedalCompact, eagleMedalMini} {
		lines := strings.Split(strings.ReplaceAll(strings.Trim(art, "\r\n"), "\r", ""), "\n")
		logoWidth := 0
		for _, line := range lines {
			logoWidth = max(logoWidth, ansi.StringWidth(line))
		}
		if len(lines) > height || logoWidth > width {
			continue
		}
		padding := strings.Repeat(" ", (width-logoWidth)/2)
		for i := range lines {
			lines[i] = padding + lines[i]
		}
		return lines
	}
	return []string{padLine("ELITE AUX", width)}
}
