package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func journalRecord(t *testing.T, body string) string {
	t.Helper()
	var values map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &values); err != nil {
		t.Fatal(err)
	}
	if _, ok := values["timestamp"]; !ok {
		values["timestamp"] = json.RawMessage(`"` + testNow.Format(time.RFC3339) + `"`)
	}
	data, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	return string(data) + "\n"
}

func applyRecord(t *testing.T, data *companionData, body string) {
	t.Helper()
	var event journalEvent
	if err := json.Unmarshal([]byte(journalRecord(t, body)), &event); err != nil {
		t.Fatal(err)
	}
	data.apply(event)
}

func writeFixture(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func appendFixture(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(text)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
}

func TestJournalBootstrapBackfillsMissionDetailsAndFollowsPartialAppend(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "Journal.2026-10-22T120000.01.log")
	current := filepath.Join(dir, "Journal.2026-10-23T120000.01.log")
	writeFixture(t, old, journalRecord(t, `{"event":"Commander","Name":"Example","FID":"F1"}`)+
		journalRecord(t, `{"event":"MissionAccepted","MissionID":42,"LocalisedName":"Deliver supplies","DestinationSystem":"Destination","DestinationStation":"Dock","Count":36}`))
	writeFixture(t, current, journalRecord(t, `{"event":"Commander","Name":"Example","FID":"F1"}`)+
		journalRecord(t, `{"event":"Materials","Raw":[{"Name":"arsenic","Count":12}],"Manufactured":[],"Encoded":[]}`)+
		journalRecord(t, `{"event":"LoadGame","Commander":"Example","FID":"F1","Ship":"Krait_MkII","ShipID":1,"FuelCapacity":32}`)+
		journalRecord(t, `{"event":"Loadout","Ship":"Krait_MkII","ShipID":1,"ShipName":"Wayfarer","HullHealth":0.94,"FuelCapacity":{"Main":32},"CargoCapacity":64,"Modules":[{"Slot":"FrameShiftDrive","Item":"hyperdrive","On":true,"Health":1}]}`)+
		journalRecord(t, `{"event":"Missions","Active":[{"MissionID":42,"Name":"delivery","Expires":3600}]}`)+
		journalRecord(t, `{"event":"Location","StarSystem":"Origin","StarPos":[0,0,0]}`))
	r := newJournalReader(dir)
	msg := r.poll()
	if msg.Error != "" || msg.Data.Commander != "Example" || msg.Data.Ship.Name != "Wayfarer" || msg.Data.Ship.Tank == nil || *msg.Data.Ship.Tank != 32 {
		t.Fatal("startup did not reconstruct ship", msg.Error)
	}
	if mission := msg.Data.Missions[42]; mission.LocalisedName != "Deliver supplies" || mission.Expiry.Sub(testNow) != time.Hour {
		t.Fatal("active summary lost acceptance details or expiry")
	}
	partial := strings.TrimSuffix(journalRecord(t, `{"event":"MaterialCollected","Category":"Raw","Name":"arsenic","Count":3}`), "\n")
	appendFixture(t, current, partial[:len(partial)/2])
	if data := r.poll().Data; data.Materials["raw/arsenic"].Count != 12 {
		t.Fatal("partial record changed stock")
	}
	appendFixture(t, current, partial[len(partial)/2:]+"\n")
	if data := r.poll().Data; data.Materials["raw/arsenic"].Count != 15 {
		t.Fatal("finished record did not apply once")
	}
	if data := r.poll().Data; data.Materials["raw/arsenic"].Count != 15 {
		t.Fatal("poll replayed a delta")
	}
	if msg.Data.Materials["raw/arsenic"].Count != 12 {
		t.Fatal("published snapshot shares mutable inventory")
	}
}

func TestJournalRotationTruncationAndCommanderIsolation(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "Journal.251022120000.01.log")
	current := filepath.Join(dir, "Journal.2026-10-23T120000.01.log")
	writeFixture(t, legacy, journalRecord(t, `{"event":"Commander","Name":"Old","FID":"OLD"}`)+journalRecord(t, `{"event":"MissionAccepted","MissionID":7,"Name":"old mission"}`))
	writeFixture(t, current, journalRecord(t, `{"event":"Commander","Name":"New","FID":"NEW"}`)+journalRecord(t, `{"event":"Materials","Raw":[{"Name":"iron","Count":10}],"Manufactured":[],"Encoded":[]}`))
	r := newJournalReader(dir)
	msg := r.poll()
	if msg.Error != "" || msg.Data.Commander != "New" || len(msg.Data.Missions) != 0 {
		t.Fatal("legacy ordering or commander isolation failed", msg)
	}
	next := filepath.Join(dir, "Journal.2026-10-23T120000.02.log")
	writeFixture(t, next, journalRecord(t, `{"event":"MaterialCollected","Category":"Raw","Name":"iron","Count":3}`))
	if msg := r.poll(); msg.Data.Materials["raw/iron"].Count != 13 {
		t.Fatal("rotation did not continue inventory", msg.Error)
	}
	writeFixture(t, next, journalRecord(t, `{"event":"Missions","Active":[]}`))
	if msg := r.poll(); msg.Error != "" || msg.Data.Materials["raw/iron"].Count != 10 || !msg.Data.MissionsKnown {
		t.Fatal("truncation did not rebuild without replaying removed deltas", msg.Error)
	}
	appendFixture(t, next, journalRecord(t, `{"event":"Commander","Name":"Third","FID":"THIRD"}`))
	msg = r.poll()
	if msg.Data.Commander != "Third" || len(msg.Data.Materials) != 0 || msg.Data.MissionsKnown || msg.Data.Ship.Type != "" {
		t.Fatal("commander switch retained another account's data")
	}
}

func TestJournalRoutePartialRecoveryAndClearCannotResurrectCachedRoute(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Journal.2026-10-23T120000.01.log")
	writeFixture(t, path, journalRecord(t, `{"event":"Location","StarSystem":"Origin","StarPos":[0,0,0]}`))
	route := filepath.Join(dir, "NavRoute.json")
	writeFixture(t, route, journalRecord(t, `{"event":"NavRoute","Route":[{"StarSystem":"Origin","StarPos":[0,0,0]},{"StarSystem":"Next","StarClass":"K","StarPos":[3,4,0]}]}`))
	r := newJournalReader(dir)
	if msg := r.poll(); msg.Error != "" || len(msg.Data.Nav.Route) != 2 {
		t.Fatal("route not loaded", msg.Error)
	}
	writeFixture(t, route, `{"event":`)
	if msg := r.poll(); msg.Error == "" || len(msg.Data.Nav.Route) != 2 {
		t.Fatal("partial route should retain last known route with error")
	}
	writeFixture(t, route, journalRecord(t, `{"event":"NavRouteClear","timestamp":"2026-10-23T07:00:01Z","Route":[]}`))
	if msg := r.poll(); msg.Error != "" || len(msg.Data.Nav.Route) != 0 {
		t.Fatal("route clear did not recover")
	}
	writeFixture(t, route, journalRecord(t, `{"event":"NavRoute","Route":[{"StarSystem":"Cached"}]}`))
	if msg := r.poll(); len(msg.Data.Nav.Route) != 0 {
		t.Fatal("old route resurrected after clear")
	}
	appendFixture(t, path, journalRecord(t, `{"event":"FSDTarget","timestamp":"2026-10-23T07:00:03Z","Name":"Direct target","StarClass":"K"}`))
	writeFixture(t, route, journalRecord(t, `{"event":"NavRouteClear","timestamp":"2026-10-23T07:00:02Z","Route":[]}`))
	if msg := r.poll(); msg.Data.Nav.Target != "Direct target" {
		t.Fatal("cached clear removed a more recent selected target")
	}
	if distance, ok := routeDistance([]float64{0, 0, 0}, []float64{3, 4, 0}); !ok || distance != 5 {
		t.Fatal("route distance incorrect")
	}
}

func TestJournalMissionsLifecyclePreservesIDsProgressAndUnknowns(t *testing.T) {
	d := newCompanionData()
	applyRecord(t, &d, `{"event":"MissionAccepted","MissionID":12345678901234567890,"LocalisedName":"Delivery","DestinationSystem":"A","Count":36,"Expiry":"2026-10-24T07:00:00Z"}`)
	applyRecord(t, &d, `{"event":"CargoDepot","MissionID":12345678901234567890,"ItemsDelivered":24,"ItemsCollected":36,"TotalItemsToDeliver":36}`)
	applyRecord(t, &d, `{"event":"MissionRedirected","MissionID":12345678901234567890,"NewDestinationSystem":"B","NewDestinationStation":"Dock"}`)
	mission := d.Missions[12345678901234567890]
	if mission.Delivered == nil || *mission.Delivered != 24 || mission.DestinationSystem != "B" {
		t.Fatal("delivery progress or redirect lost")
	}
	applyRecord(t, &d, `{"event":"Missions","Active":[{"MissionID":12345678901234567890,"Name":"Delivery","Expires":3600},{"MissionID":8,"Name":"Unknown","Expires":60}]}`)
	if !d.MissionsKnown || d.Missions[8].Details || !d.Missions[12345678901234567890].Details {
		t.Fatal("startup summary invented or lost details")
	}
	applyRecord(t, &d, `{"event":"MissionCompleted","MissionID":12345678901234567890}`)
	applyRecord(t, &d, `{"event":"MissionFailed","MissionID":8}`)
	if len(d.Missions) != 0 {
		t.Fatal("finished missions remain active")
	}
	applyRecord(t, &d, `{"event":"MissionAccepted","MissionID":9,"Name":"Abandoned"}`)
	applyRecord(t, &d, `{"event":"MissionAbandoned","MissionID":9}`)
	if len(d.Missions) != 0 {
		t.Fatal("abandoned mission remains active")
	}
	if !strings.Contains(missionDeadline(testNow.Add(-time.Minute), testNow), "awaiting") {
		t.Fatal("timer must not invent a mission failure event")
	}
}

func TestJournalMaterialsSnapshotsTradesCraftingAndRewards(t *testing.T) {
	d := newCompanionData()
	applyRecord(t, &d, `{"event":"Materials","Raw":[{"Name":"arsenic","Count":20}],"Manufactured":[],"Encoded":[{"Name":"dataminedwake","Name_Localised":"Datamined Wake Exceptions","Count":5}]}`)
	applyRecord(t, &d, `{"event":"MaterialCollected","Category":"Raw","Name":"arsenic","Count":3}`)
	applyRecord(t, &d, `{"event":"MaterialDiscarded","Category":"Raw","Name":"arsenic","Count":1}`)
	applyRecord(t, &d, `{"event":"MaterialTrade","Paid":{"Material":"arsenic","Category":"Raw","Quantity":6},"Received":{"Material":"iron","Category":"Raw","Quantity":3}}`)
	applyRecord(t, &d, `{"event":"EngineerCraft","Ingredients":[{"Name":"arsenic","Count":2},{"Name":"dataminedwake","Count":1}]}`)
	applyRecord(t, &d, `{"event":"Synthesis","Materials":[{"Name":"iron","Count":1}]}`)
	applyRecord(t, &d, `{"event":"MissionCompleted","MissionID":1,"MaterialsReward":[{"Name":"arsenic","Category":"$MICRORESOURCE_CATEGORY_Raw;","Count":4}]}`)
	if d.Materials["raw/arsenic"].Count != 18 || d.Materials["raw/iron"].Count != 2 || d.Materials["encoded/dataminedwake"].Count != 4 {
		t.Fatal("material arithmetic incorrect", d.Materials)
	}
	if d.Materials["encoded/dataminedwake"].Display != "Datamined Wake Exceptions" {
		t.Fatal("localised name was lost")
	}
	applyRecord(t, &d, `{"event":"Materials","Raw":[],"Manufactured":[],"Encoded":[]}`)
	if len(d.Materials) != 0 || !d.MaterialCategories["raw"] {
		t.Fatal("empty snapshot must reset counts and remain known")
	}
}

func TestJournalShipContextBadRecordsAndCancellation(t *testing.T) {
	d := newCompanionData()
	applyRecord(t, &d, `{"event":"Loadout","Ship":"Krait_MkII","ShipID":1,"HullHealth":0.9}`)
	applyRecord(t, &d, `{"event":"Loadout","Ship":"federation_fighter","ShipID":2,"HullHealth":0.1}`)
	applyRecord(t, &d, `{"event":"HullDamage","PlayerShip":true,"Fighter":true,"Health":0.1}`)
	if d.Ship.Type != "Krait_MkII" || *d.Ship.Hull != .9 {
		t.Fatal("fighter data replaced mothership")
	}
	applyRecord(t, &d, `{"event":"HullDamage","PlayerShip":true,"Fighter":false,"Health":0.8}`)
	if *d.Ship.Hull != .8 {
		t.Fatal("player hull report did not update")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "Journal.2026-10-23T120000.01.log")
	writeFixture(t, path, "bad json\n"+journalRecord(t, `{"event":"Commander","Name":"Recovered","FID":"F1"}`))
	r := newJournalReader(dir)
	if msg := r.poll(); msg.Error != "" || msg.Data.Commander != "Recovered" || msg.Data.Warning == "" {
		t.Fatal("malformed completed record must not stop the tail")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.ctx = ctx
	if msg := r.poll(); !strings.Contains(msg.Error, "canceled") {
		t.Fatal("journal reader ignored cancellation")
	}
}
