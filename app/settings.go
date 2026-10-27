package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"io"
	"os"
	"path/filepath"

	"charm.land/lipgloss/v2"
)

type panelBorder struct{ topLeft, topRight, bottomLeft, bottomRight, horizontal, vertical string }

type tuiTheme struct {
	ID, Name, Description                                                       string
	BG, Text, Muted, Border, Accent, Success, Info, Warning, Danger, Simulation color.Color
	Frame                                                                       panelBorder
}

// Palettes are immutable; each model owns its selection, including previews.
var tuiThemes = []tuiTheme{
	{
		ID: "command-deck", Name: "Command Deck", Description: "Amber highlights, charcoal surfaces, rounded instrument panels.",
		BG: lipgloss.Color("#11100D"), Text: lipgloss.Color("#EEE8DC"), Muted: lipgloss.Color("#B1A18A"),
		Border: lipgloss.Color("#8B6535"), Accent: lipgloss.Color("#FFB45B"), Success: lipgloss.Color("#7EE2A8"),
		Info: lipgloss.Color("#70D9DF"), Warning: lipgloss.Color("#FFD080"), Danger: lipgloss.Color("#FF7F87"), Simulation: lipgloss.Color("#C5AEFF"),
		Frame: panelBorder{"╭", "╮", "╰", "╯", "─", "│"},
	},
	{
		ID: "flight-console", Name: "Flight Console", Description: "Cyan highlights, deep navy surfaces, clean square panels.",
		BG: lipgloss.Color("#0A141E"), Text: lipgloss.Color("#DDEEF5"), Muted: lipgloss.Color("#93B5C5"),
		Border: lipgloss.Color("#426F85"), Accent: lipgloss.Color("#70E1F5"), Success: lipgloss.Color("#83E5C3"),
		Info: lipgloss.Color("#89CFFF"), Warning: lipgloss.Color("#FFD080"), Danger: lipgloss.Color("#FF8797"), Simulation: lipgloss.Color("#CDB5FF"),
		Frame: panelBorder{"┌", "┐", "└", "┘", "─", "│"},
	},
	{
		ID: "retro-ops", Name: "Retro Operations", Description: "Phosphor greens, dark screen, double-line console panels.",
		BG: lipgloss.Color("#080F09"), Text: lipgloss.Color("#C4F5BE"), Muted: lipgloss.Color("#8EB886"),
		Border: lipgloss.Color("#54854B"), Accent: lipgloss.Color("#A4F58B"), Success: lipgloss.Color("#A4F58B"),
		Info: lipgloss.Color("#B5EFA7"), Warning: lipgloss.Color("#F4D580"), Danger: lipgloss.Color("#FF997F"), Simulation: lipgloss.Color("#D5BAF5"),
		Frame: panelBorder{"╔", "╗", "╚", "╝", "═", "║"},
	},
}

type uiSettings struct {
	Version int    `json:"version"`
	Theme   string `json:"theme"`
}

func themeIndex(id string) (int, bool) {
	for i, theme := range tuiThemes {
		if theme.ID == id {
			return i, true
		}
	}
	return 0, false
}

func (m cockpitModel) palette() tuiTheme {
	if m.themeChoice < 0 || m.themeChoice >= len(tuiThemes) {
		return tuiThemes[0]
	}
	return tuiThemes[m.themeChoice]
}

func defaultSettingsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "settings.json"
	}
	return filepath.Join(dir, "EliteAux", "settings.json")
}

func loadUISettings(path string) (int, error) {
	if path == "" {
		return 0, nil
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if info.Size() > 4096 {
		return 0, errors.New("appearance settings exceed 4 KiB")
	}
	decoder := json.NewDecoder(io.LimitReader(f, 4096))
	var settings uiSettings
	if err := decoder.Decode(&settings); err != nil {
		return 0, fmt.Errorf("invalid appearance settings: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return 0, errors.New("expected one appearance settings object")
	}
	i, ok := themeIndex(settings.Theme)
	if !ok || settings.Version != 1 {
		return 0, errors.New("unsupported appearance settings version or theme")
	}
	return i, nil
}

func saveUISettings(path string, selection int) error {
	if selection < 0 || selection >= len(tuiThemes) {
		return errors.New("unknown theme")
	}
	return writeJSONAtomic(path, uiSettings{Version: 1, Theme: tuiThemes[selection].ID})
}

func writeJSONAtomic(path string, value any) error {
	if path == "" {
		return errors.New("no settings path configured")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".settings-*.json")
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(value)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

func (m *cockpitModel) leaveSettings() {
	if m.page == pageSettings {
		m.themeChoice = m.savedTheme
	}
}

func (m cockpitModel) appearanceSettings(width int) string {
	lines := []string{m.section("APPEARANCE / THEMES"), "Up/Down: preview   Enter: apply & save   Esc: cancel", ""}
	for i, theme := range tuiThemes {
		marker, saved := "  ", ""
		if i == m.themeChoice {
			marker = "> "
		}
		if i == m.savedTheme {
			saved = " [saved]"
		}
		label := marker + theme.Name + saved
		if i == m.themeChoice {
			label = bold(label, m.palette().Accent)
		}
		lines = append(lines, label)
	}
	lines = append(lines, "", m.palette().Description)
	body := m.panel(width, m.palette().Border, lines...)
	states := bold("ACTIVE", m.palette().Success) + "  " + ink("INACTIVE", m.palette().Muted) + "  " + bold("WAIT", m.palette().Warning) + "  " + bold("WARNING", m.palette().Danger)
	if m.height < 24 {
		return body
	}
	if m.height < 32 {
		return body + "\n" + states + "\nCRT scan lines/glow: terminal retro effect."
	}
	preview := m.panel(width, m.palette().Border, m.section("LIVE STYLE PREVIEW"),
		states,
		"All pages and the connection scanner use this theme.")
	return body + "\n" + preview + "\n" +
		"CRT scan lines/glow: use your terminal's retro effect.\n" +
		"Settings file: " + m.opts.settings
}
