package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type shipModule struct {
	Slot, Item                         string
	On                                 bool
	Health                             *float64
	Priority, AmmoInClip, AmmoInHopper *int
}

type shipOverview struct {
	Type, Name, Ident          string
	ID                         int
	Hull, Tank, MaxJump, Rebuy *float64
	CargoCapacity              *int
	Modules                    []shipModule
	Updated, HullAt, ModulesAt time.Time
}

type routePoint struct {
	StarSystem string
	StarClass  string
	StarPos    []float64
}

type navigationData struct {
	System, Body, Station, Target, TargetClass string
	Position                                   []float64
	Remaining                                  *int
	Route                                      []routePoint
	Updated, TargetAt, RouteAt                 time.Time
}

type missionData struct {
	MissionID                                                             uint64
	Name, LocalisedName, DestinationSystem, DestinationStation, Commodity string
	Count, Delivered, Collected                                           *int
	Reward                                                                *int64
	Expiry                                                                time.Time
	Details                                                               bool
}

type materialStock struct {
	Category, Name, Display string
	Count                   int
}

type companionData struct {
	Commander, FID, GameMode string
	Ship                     shipOverview
	Nav                      navigationData
	Missions                 map[uint64]missionData
	MissionsKnown            bool
	Materials                map[string]materialStock
	MaterialCategories       map[string]bool
	LastEvent, SessionStart  time.Time
	Source, Warning          string
}

func newCompanionData() companionData {
	return companionData{Missions: make(map[uint64]missionData), Materials: make(map[string]materialStock), MaterialCategories: make(map[string]bool)}
}

func (d companionData) clone() companionData {
	c := d
	c.Missions = make(map[uint64]missionData, len(d.Missions))
	for id, mission := range d.Missions {
		c.Missions[id] = mission
	}
	c.Materials = make(map[string]materialStock, len(d.Materials))
	for id, stock := range d.Materials {
		c.Materials[id] = stock
	}
	c.MaterialCategories = make(map[string]bool, len(d.MaterialCategories))
	for id, known := range d.MaterialCategories {
		c.MaterialCategories[id] = known
	}
	c.Ship.Modules = append([]shipModule(nil), d.Ship.Modules...)
	c.Nav.Position = append([]float64(nil), d.Nav.Position...)
	c.Nav.Route = append([]routePoint(nil), d.Nav.Route...)
	// Event-owned coordinate slices and optional scalar pointers are immutable.
	return c
}

func canonicalCategory(category string) string {
	category = strings.ToLower(strings.TrimSpace(category))
	category = strings.TrimSuffix(strings.TrimPrefix(category, "$microresource_category_"), ";")
	return category
}

func materialKey(category, name string) string {
	return canonicalCategory(category) + "/" + strings.ToLower(strings.TrimSpace(name))
}

type eventMaterial struct {
	Name, NameLocalised, Category, Material, MaterialLocalised string
	Count, Quantity                                            int
}

// Explicit JSON tags are needed for the journal's localisation underscores.
func (m *eventMaterial) UnmarshalJSON(data []byte) error {
	type plain eventMaterial
	var v struct {
		plain
		NameLocalised     string `json:"Name_Localised"`
		MaterialLocalised string `json:"Material_Localised"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*m = eventMaterial(v.plain)
	m.NameLocalised, m.MaterialLocalised = v.NameLocalised, v.MaterialLocalised
	return nil
}

type journalEvent struct {
	Timestamp                                                                                         time.Time
	Event, FID, Commander, Name, GameMode                                                             string
	Ship, ShipName, ShipIdent                                                                         string
	ShipID                                                                                            int
	HullHealth, Health, MaxJumpRange, Rebuy                                                           *float64
	PlayerShip                                                                                        *bool
	Fighter                                                                                           bool
	FuelCapacity                                                                                      json.RawMessage
	CargoCapacity                                                                                     *int
	Modules                                                                                           []shipModule
	StarSystem, Body, StationName, StarClass                                                          string
	StarPos                                                                                           []float64
	RemainingJumpsInRoute                                                                             *int
	Route                                                                                             []routePoint
	MissionID                                                                                         uint64
	LocalisedName, DestinationSystem, DestinationStation, NewDestinationSystem, NewDestinationStation string
	Commodity                                                                                         string
	CommodityLocalised                                                                                string `json:"Commodity_Localised"`
	Count, ItemsDelivered, ItemsCollected, TotalItemsToDeliver                                        *int
	Reward                                                                                            *int64
	Expiry                                                                                            time.Time
	Expires                                                                                           *int
	Active                                                                                            *[]struct {
		MissionID uint64
		Name      string
		Expires   *int
	}
	Raw, Manufactured, Encoded              *[]eventMaterial
	Category                                string
	NameLocalised                           string `json:"Name_Localised"`
	Paid, Received                          eventMaterial
	Ingredients, Materials, MaterialsReward []eventMaterial
	Material                                string
	Quantity                                int
}

func (d *companionData) identity(fid, commander string) {
	if (fid != "" && d.FID != "" && fid != d.FID) || (fid == "" && commander != "" && d.Commander != "" && commander != d.Commander) {
		source := d.Source
		*d = newCompanionData()
		d.Source = source
	}
	if fid != "" {
		d.FID = fid
	}
	if commander != "" {
		d.Commander = commander
	}
}

func (d *companionData) stockDelta(category, name, display string, delta int) {
	category = canonicalCategory(category)
	if category != "raw" && category != "manufactured" && category != "encoded" {
		// Craft ingredients lack a category; resolve only a unique known symbol.
		matches := 0
		for _, stock := range materialCatalogue {
			if strings.EqualFold(stock.Name, name) {
				category = stock.Category
				matches++
			}
		}
		if matches != 1 {
			return
		}
	}
	key := materialKey(category, name)
	stock := d.Materials[key]
	stock.Category, stock.Name = category, strings.ToLower(name)
	if display != "" {
		stock.Display = display
	}
	stock.Count = max(0, stock.Count+delta)
	d.Materials[key] = stock
}

func (d *companionData) apply(e journalEvent) {
	if e.Timestamp.IsZero() {
		return
	}
	if e.Event == "Commander" {
		d.identity(e.FID, e.Name)
	}
	if e.Event == "LoadGame" {
		d.identity(e.FID, e.Commander)
		d.GameMode, d.SessionStart = e.GameMode, e.Timestamp
	}
	if e.Timestamp.After(d.LastEvent) {
		d.LastEvent = e.Timestamp
	}
	switch e.Event {
	case "LoadGame", "Loadout":
		// SRV/fighter loadouts must not replace the mothership overview.
		shipType := strings.ToLower(e.Ship)
		if shipType == "" || strings.Contains(shipType, "buggy") || strings.Contains(shipType, "fighter") || strings.Contains(shipType, "srv") || shipType == "scorpion" {
			return
		}
		if e.ShipID != d.Ship.ID || !strings.EqualFold(e.Ship, d.Ship.Type) {
			d.Ship = shipOverview{ID: e.ShipID, Type: e.Ship}
		}
		d.Ship.Type, d.Ship.Name, d.Ship.Ident = e.Ship, strings.TrimSpace(e.ShipName), strings.TrimSpace(e.ShipIdent)
		d.Ship.Updated = e.Timestamp
		if len(e.FuelCapacity) > 0 {
			var main float64
			if json.Unmarshal(e.FuelCapacity, &main) == nil {
				d.Ship.Tank = &main
			} else {
				var tank struct{ Main *float64 }
				if json.Unmarshal(e.FuelCapacity, &tank) == nil {
					d.Ship.Tank = tank.Main
				}
			}
		}
		if e.Event == "Loadout" {
			d.Ship.Hull, d.Ship.HullAt = e.HullHealth, e.Timestamp
			d.Ship.MaxJump, d.Ship.Rebuy, d.Ship.CargoCapacity = e.MaxJumpRange, e.Rebuy, e.CargoCapacity
			d.Ship.Modules = append([]shipModule(nil), e.Modules...)
			d.Ship.ModulesAt = e.Timestamp
		}
	case "HullDamage":
		if e.PlayerShip != nil && *e.PlayerShip && !e.Fighter && e.Health != nil {
			d.Ship.Hull, d.Ship.HullAt = e.Health, e.Timestamp
		}
	case "Location", "FSDJump", "CarrierJump":
		d.Nav.System, d.Nav.Body, d.Nav.Station = e.StarSystem, e.Body, e.StationName
		d.Nav.Position, d.Nav.Updated = e.StarPos, e.Timestamp
		if d.Nav.Target == e.StarSystem {
			d.Nav.Target, d.Nav.TargetClass, d.Nav.Remaining = "", "", nil
		}
	case "Docked":
		if e.StarSystem != "" {
			d.Nav.System = e.StarSystem
		}
		d.Nav.Station, d.Nav.Updated = e.StationName, e.Timestamp
	case "Undocked":
		d.Nav.Station, d.Nav.Updated = "", e.Timestamp
	case "SupercruiseExit", "ApproachBody":
		d.Nav.Body = e.Body
	case "FSDTarget":
		d.Nav.Target, d.Nav.TargetClass, d.Nav.Remaining, d.Nav.TargetAt = e.Name, e.StarClass, e.RemainingJumpsInRoute, e.Timestamp
	case "NavRouteClear":
		d.Nav.Route, d.Nav.RouteAt = nil, e.Timestamp
		if !e.Timestamp.Before(d.Nav.TargetAt) {
			d.Nav.Target, d.Nav.TargetClass, d.Nav.Remaining = "", "", nil
		}
	case "NavRoute":
		if e.Route != nil && !e.Timestamp.Before(d.Nav.RouteAt) {
			d.Nav.Route, d.Nav.RouteAt = e.Route, e.Timestamp
		}
	case "Missions":
		if e.Active != nil {
			active := make(map[uint64]missionData)
			for _, summary := range *e.Active {
				mission := d.Missions[summary.MissionID]
				mission.MissionID = summary.MissionID
				if mission.Name == "" {
					mission.Name = summary.Name
				}
				if summary.Expires != nil && *summary.Expires >= 0 {
					mission.Expiry = e.Timestamp.Add(time.Duration(*summary.Expires) * time.Second)
				}
				active[summary.MissionID] = mission
			}
			d.Missions, d.MissionsKnown = active, true
		}
	case "MissionAccepted":
		if e.MissionID == 0 {
			break
		}
		mission := missionData{MissionID: e.MissionID, Name: e.Name, LocalisedName: e.LocalisedName,
			DestinationSystem: e.DestinationSystem, DestinationStation: e.DestinationStation,
			Commodity: e.Commodity, Count: e.Count, Reward: e.Reward, Expiry: e.Expiry, Details: true}
		if e.CommodityLocalised != "" {
			mission.Commodity = e.CommodityLocalised
		}
		d.Missions[e.MissionID] = mission
	case "MissionCompleted", "MissionAbandoned", "MissionFailed":
		delete(d.Missions, e.MissionID)
		for _, reward := range e.MaterialsReward {
			d.stockDelta(reward.Category, reward.Name, reward.NameLocalised, reward.Count)
		}
	case "MissionRedirected":
		if mission, ok := d.Missions[e.MissionID]; ok {
			mission.DestinationSystem, mission.DestinationStation = e.NewDestinationSystem, e.NewDestinationStation
			d.Missions[e.MissionID] = mission
		}
	case "CargoDepot":
		if mission, ok := d.Missions[e.MissionID]; ok {
			mission.Delivered, mission.Collected = e.ItemsDelivered, e.ItemsCollected
			if e.TotalItemsToDeliver != nil {
				mission.Count = e.TotalItemsToDeliver
			}
			d.Missions[e.MissionID] = mission
		}
	case "Materials":
		for category, entries := range map[string]*[]eventMaterial{"raw": e.Raw, "manufactured": e.Manufactured, "encoded": e.Encoded} {
			if entries == nil {
				continue
			}
			for key, stock := range d.Materials {
				if stock.Category == category {
					delete(d.Materials, key)
				}
			}
			for _, entry := range *entries {
				d.stockDelta(category, entry.Name, entry.NameLocalised, entry.Count)
			}
			d.MaterialCategories[category] = true
		}
	case "MaterialCollected", "MaterialDiscarded":
		if e.Count != nil {
			delta := *e.Count
			if e.Event == "MaterialDiscarded" {
				delta = -delta
			}
			d.stockDelta(e.Category, e.Name, e.NameLocalised, delta)
		}
	case "MaterialTrade":
		d.stockDelta(e.Paid.Category, e.Paid.Material, e.Paid.MaterialLocalised, -e.Paid.Quantity)
		d.stockDelta(e.Received.Category, e.Received.Material, e.Received.MaterialLocalised, e.Received.Quantity)
	case "EngineerCraft", "Synthesis", "TechnologyBroker":
		for _, material := range append(append([]eventMaterial(nil), e.Ingredients...), e.Materials...) {
			quantity := material.Count
			if quantity == 0 {
				quantity = material.Quantity
			}
			d.stockDelta(material.Category, material.Name, material.NameLocalised, -quantity)
		}
	case "EngineerContribution":
		if e.Material != "" {
			d.stockDelta("", e.Material, "", -e.Quantity)
		}
	}
}

type journalCursor struct {
	path   string
	offset int64
	info   os.FileInfo
}
type journalReader struct {
	dir       string
	ctx       context.Context
	data      companionData
	cursor    journalCursor
	started   bool
	routeInfo os.FileInfo
}

type companionMsg struct {
	Data  companionData
	Error string
}

func newJournalReader(dir string) *journalReader {
	return &journalReader{dir: dir, ctx: context.Background(), data: newCompanionData()}
}

func (r *journalReader) readJournal(path string, cursor *journalCursor) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() > 64<<20 {
		return errors.New("journal exceeds 64 MiB")
	}
	if _, err := f.Seek(cursor.offset, io.SeekStart); err != nil {
		return err
	}
	reader := bufio.NewReader(io.LimitReader(f, (64<<20)-cursor.offset+1))
	for {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		line, consumed, oversized, err := readJournalRecord(r.ctx, reader)
		if cursor.offset+consumed > 64<<20 {
			return errors.New("journal exceeds 64 MiB")
		}
		if err == io.EOF {
			break
		} // Retain the offset before an unfinished record.
		if err != nil {
			return err
		}
		cursor.offset += consumed
		if oversized {
			r.data.Warning = "Oversized journal record skipped"
			continue
		}
		var event journalEvent
		if err := json.Unmarshal(line, &event); err != nil {
			r.data.Warning = "Malformed journal record skipped; some details may be unavailable"
			continue
		}
		r.data.apply(event)
	}
	cursor.path, cursor.info = path, info
	r.data.Source = filepath.Base(path)
	return nil
}

// Bound allocation while reading, rather than checking a potentially huge
// line after ReadBytes has already allocated it. Check cancellation per chunk.
func readJournalRecord(ctx context.Context, reader *bufio.Reader) ([]byte, int64, bool, error) {
	var line []byte
	var consumed int64
	oversized := false
	for {
		if err := ctx.Err(); err != nil {
			return nil, consumed, oversized, err
		}
		chunk, err := reader.ReadSlice('\n')
		consumed += int64(len(chunk))
		if consumed > 4<<20 {
			oversized, line = true, nil
		}
		if !oversized {
			line = append(line, chunk...)
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		return line, consumed, oversized, err
	}
}

func (r *journalReader) poll() companionMsg {
	err := r.update()
	msg := companionMsg{Data: r.data.clone()}
	if err != nil {
		msg.Error = err.Error()
	}
	return msg
}

func (r *journalReader) update() error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	paths, err := filepath.Glob(filepath.Join(r.dir, "Journal.*.log"))
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return errors.New("Waiting for a Player Journal in " + r.dir)
	}
	sort.Slice(paths, func(i, j int) bool { return journalOrder(paths[i]) < journalOrder(paths[j]) })
	if len(paths) > 32 {
		paths = paths[len(paths)-32:]
	}
	if r.started {
		info, err := os.Stat(r.cursor.path)
		if err != nil || info.Size() < r.cursor.offset || !os.SameFile(info, r.cursor.info) {
			r.started, r.data, r.cursor, r.routeInfo = false, newCompanionData(), journalCursor{}, nil
		}
	}
	if !r.started {
		r.data = newCompanionData()
		for _, path := range paths {
			cursor := journalCursor{}
			if err := r.readJournal(path, &cursor); err != nil {
				return err
			}
			r.cursor = cursor
		}
		r.started = true
	} else {
		if err := r.readJournal(r.cursor.path, &r.cursor); err != nil {
			return err
		}
		for _, path := range paths {
			if journalOrder(path) <= journalOrder(r.cursor.path) {
				continue
			}
			cursor := journalCursor{}
			if err := r.readJournal(path, &cursor); err != nil {
				return err
			}
			r.cursor = cursor
		}
	}
	return r.readRoute()
}

func journalOrder(path string) string {
	name := strings.TrimPrefix(filepath.Base(path), "Journal.")
	parts := strings.Split(name, ".")
	if len(parts) > 1 {
		for _, layout := range []string{"2006-01-02T150405", "060102150405"} {
			if stamp, err := time.Parse(layout, parts[0]); err == nil {
				return stamp.Format("20060102150405") + "." + parts[1]
			}
		}
	}
	return name
}

func (r *journalReader) readRoute() error {
	path := filepath.Join(r.dir, "NavRoute.json")
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if r.routeInfo != nil && info.Size() == r.routeInfo.Size() && info.ModTime() == r.routeInfo.ModTime() {
		return nil
	}
	if info.Size() > 4<<20 {
		return errors.New("route snapshot exceeds 4 MiB")
	}
	if err := r.ctx.Err(); err != nil {
		return err
	}
	data, err := readSmallFile(path, 4<<20)
	if err != nil {
		return err
	}
	var event journalEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("Route file incomplete; retaining last reported route: %w", err)
	}
	if event.Event != "NavRoute" && event.Event != "NavRouteClear" {
		return errors.New("unexpected route snapshot event")
	}
	if event.Timestamp.IsZero() || (event.Event == "NavRoute" && event.Route == nil) {
		return errors.New("route snapshot is missing its timestamp or route")
	}
	if event.Timestamp.Before(r.data.Nav.RouteAt) {
		r.routeInfo = info
		return nil
	}
	r.data.apply(event)
	r.routeInfo = info
	return nil
}

func routeDistance(from, to []float64) (float64, bool) {
	if len(from) != 3 || len(to) != 3 {
		return 0, false
	}
	distance := 0.0
	for i := range from {
		distance += (from[i] - to[i]) * (from[i] - to[i])
	}
	return math.Sqrt(distance), true
}
