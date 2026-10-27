package main

import (
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

//go:embed data/materials.csv
var materialCSV string

//go:embed data/ships.csv
var shipCSV string

var materialCatalogue = parseMaterialCatalogue()

func shipDisplayName(symbol string) string {
	rows, err := csv.NewReader(strings.NewReader(shipCSV)).ReadAll()
	if err == nil {
		for _, row := range rows[1:] {
			if len(row) > 2 && strings.EqualFold(row[1], symbol) {
				return safeText(row[2])
			}
		}
	}
	return reportedText(symbol)
}

func parseMaterialCatalogue() []materialStock {
	rows, err := csv.NewReader(strings.NewReader(materialCSV)).ReadAll()
	if err != nil {
		panic("invalid bundled material catalogue: " + err.Error())
	}
	var stocks []materialStock
	for _, row := range rows[1:] {
		if len(row) < 6 {
			continue
		}
		stocks = append(stocks, materialStock{Category: canonicalCategory(row[3]), Name: strings.ToLower(row[1]), Display: strings.TrimSpace(row[5])})
	}
	return stocks
}

type materialGoals struct {
	Version int            `json:"version"`
	Targets map[string]int `json:"targets"`
}

func validGoal(key string, target int) bool {
	parts := strings.Split(key, "/")
	if len(parts) != 2 || (parts[0] != "raw" && parts[0] != "manufactured" && parts[0] != "encoded") || len(parts[1]) == 0 || len(parts[1]) > 128 || target < 1 || target > 999999 {
		return false
	}
	for _, c := range parts[1] {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}

func loadMaterialGoals(path string) (map[string]int, error) {
	targets := make(map[string]int)
	if path == "" {
		return targets, nil
	}
	data, err := readSmallFile(path, 64<<10)
	if os.IsNotExist(err) {
		return targets, nil
	}
	if err != nil {
		return targets, err
	}
	var goals materialGoals
	if err := json.Unmarshal(data, &goals); err != nil {
		return targets, err
	}
	if goals.Version != 1 || len(goals.Targets) > 1000 {
		return targets, errors.New("unsupported engineering goals file")
	}
	for key, target := range goals.Targets {
		if !validGoal(key, target) {
			return targets, errors.New("invalid material target")
		}
	}
	if goals.Targets != nil {
		targets = goals.Targets
	}
	return targets, nil
}

func saveMaterialGoals(path string, targets map[string]int) error {
	if len(targets) > 1000 {
		return errors.New("too many material targets")
	}
	for key, target := range targets {
		if !validGoal(key, target) {
			return errors.New("invalid material target")
		}
	}
	return writeJSONAtomic(path, materialGoals{Version: 1, Targets: targets})
}

type materialRow struct {
	materialStock
	Key    string
	Known  bool
	Target int
}

func (m cockpitModel) materialRows() []materialRow {
	stocks := make(map[string]materialStock)
	for _, stock := range materialCatalogue {
		stocks[materialKey(stock.Category, stock.Name)] = stock
	}
	for key, stock := range m.companion.Materials {
		base := stocks[key]
		if stock.Display == "" {
			stock.Display = base.Display
		}
		stocks[key] = stock
	}
	for key := range m.goals {
		if _, ok := stocks[key]; !ok {
			parts := strings.SplitN(key, "/", 2)
			if len(parts) == 2 {
				stocks[key] = materialStock{Category: parts[0], Name: parts[1], Display: parts[1]}
			}
		}
	}
	var rows []materialRow
	for key, stock := range stocks {
		if stock.Display == "" {
			stock.Display = stock.Name
		}
		target := m.goals[key]
		if m.onlyPinned && target == 0 {
			continue
		}
		if m.materialFilter != "" && !strings.Contains(strings.ToLower(stock.Display+" "+stock.Name+" "+stock.Category), strings.ToLower(m.materialFilter)) {
			continue
		}
		rows = append(rows, materialRow{materialStock: stock, Key: key, Known: m.companion.MaterialCategories[stock.Category], Target: target})
	}
	sort.Slice(rows, func(i, j int) bool {
		if (rows[i].Target > 0) != (rows[j].Target > 0) {
			return rows[i].Target > 0
		}
		if rows[i].Display == rows[j].Display {
			return rows[i].Key < rows[j].Key
		}
		return rows[i].Display < rows[j].Display
	})
	return rows
}

func (m *cockpitModel) saveTarget(key string, target int) {
	targets := make(map[string]int, len(m.goals)+1)
	for id, count := range m.goals {
		targets[id] = count
	}
	if target == 0 {
		delete(targets, key)
	} else {
		targets[key] = target
	}
	if m.opts.mode != "mock" {
		if err := saveMaterialGoals(m.opts.goals, targets); err != nil {
			m.goalMessage = "Could not save target: " + err.Error()
			return
		}
	}
	m.goals, m.goalMessage = targets, "Material targets saved"
	if m.opts.mode == "mock" {
		m.goalMessage = "Simulator target updated (not saved)"
	}
	for i, row := range m.materialRows() {
		if row.Key == key {
			m.materialIndex = i
			break
		}
	}
}

func (m *cockpitModel) engineeringKey(key tea.Key) bool {
	if key.Code == tea.KeyTab || key.Code == tea.KeyF1 || key.Code == tea.KeyF4 || key.Code >= tea.KeyF5 && key.Code <= tea.KeyF10 {
		return false
	}
	if key.Code == 'c' && key.Mod&tea.ModCtrl != 0 {
		return false
	}
	if m.goalEditing {
		switch key.Code {
		case tea.KeyEscape:
			m.goalEditing = false
			m.goalMessage = "Target edit cancelled"
		case tea.KeyEnter, tea.KeyKpEnter:
			target, err := strconv.Atoi(string(m.goalDigits))
			if err != nil || target < 1 || target > 999999 {
				m.goalMessage = "Enter a target from 1 to 999999"
				return true
			}
			m.saveTarget(m.goalKey, target)
			if !strings.HasPrefix(m.goalMessage, "Could not") {
				m.goalEditing = false
			}
		case tea.KeyBackspace:
			if m.goalReplace {
				m.goalDigits = nil
			} else if len(m.goalDigits) > 0 {
				m.goalDigits = m.goalDigits[:len(m.goalDigits)-1]
			}
			m.goalReplace = false
		default:
			if key.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) == 0 && key.Text != "" && strings.Trim(key.Text, "0123456789") == "" {
				if m.goalReplace {
					m.goalDigits = nil
					m.goalReplace = false
				}
				if len(m.goalDigits)+len(key.Text) <= 6 {
					m.goalDigits = append(m.goalDigits, []rune(key.Text)...)
				}
			}
		}
		return true
	}
	rows := m.materialRows()
	m.materialIndex = min(max(0, m.materialIndex), max(0, len(rows)-1))
	switch key.Code {
	case tea.KeyUp, tea.KeyDown, tea.KeyPgUp, tea.KeyPgDown:
		step := 1
		if key.Code == tea.KeyPgUp || key.Code == tea.KeyPgDown {
			step = 8
		}
		if key.Code == tea.KeyUp || key.Code == tea.KeyPgUp {
			step = -step
		}
		m.materialIndex = min(max(0, m.materialIndex+step), max(0, len(rows)-1))
		return true
	case '/':
		m.materialSearching, m.materialFilter, m.materialIndex = true, "", 0
		return true
	case tea.KeyF2:
		m.onlyPinned, m.materialIndex = !m.onlyPinned, 0
		return true
	case tea.KeyEscape:
		if m.materialFilter != "" || m.materialSearching {
			m.materialFilter, m.materialSearching, m.materialIndex = "", false, 0
			return true
		}
		return false
	case tea.KeyBackspace:
		if runes := []rune(m.materialFilter); len(runes) > 0 {
			m.materialFilter = string(runes[:len(runes)-1])
		}
		m.materialSearching, m.materialIndex = m.materialFilter != "", 0
		return true
	case tea.KeyEnter, tea.KeyKpEnter:
		if len(rows) > 0 {
			row := rows[m.materialIndex]
			m.materialSearching = false
			m.goalEditing, m.goalReplace, m.goalKey = true, true, row.Key
			target := row.Target
			if target == 0 {
				target = max(1, row.Count+1)
			}
			m.goalDigits, m.goalMessage = []rune(strconv.Itoa(target)), "Enter target quantity; Enter saves, Esc cancels"
		}
		return true
	case tea.KeyDelete:
		if len(rows) > 0 {
			m.saveTarget(rows[m.materialIndex].Key, 0)
		}
		return true
	case '+', '-':
		if len(rows) > 0 {
			row := rows[m.materialIndex]
			target := row.Target + 1
			if key.Code == '-' {
				target = max(0, row.Target-1)
			}
			if target <= 999999 {
				m.saveTarget(row.Key, target)
			}
		}
		return true
	}
	if key.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) == 0 && key.Text != "" {
		m.materialFilter += safeText(key.Text)
		if runes := []rune(m.materialFilter); len(runes) > 80 {
			m.materialFilter = string(runes[:80])
		}
		m.materialSearching, m.materialIndex = true, 0
		return true
	}
	return false
}

func readSmallFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	return data, err
}
