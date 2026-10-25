//go:build desktop

package main

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func desktopOutfitTestApp(t *testing.T, client *outfittingClient) *DesktopApp {
	t.Helper()
	a := newDesktopApp(defaultDesktopPreferences(), filepath.Join(t.TempDir(), "window.json"))
	a.ctx = context.Background()
	a.opts.settings, a.opts.goals, a.opts.firegroups = "", "", ""
	a.startSession(true)
	a.model.opts.mode = "live" // Read-only fixture HTTP; no real control worker is started.
	a.model.outfitting.client = client
	t.Cleanup(func() { a.shutdown(context.Background()) })
	return a
}
func waitDesktopOutfit(t *testing.T, a *DesktopApp, predicate func(outfitViewData) bool) outfitViewData {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		view := a.GetState(80).Outfitting
		if predicate(view) {
			return view
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("desktop outfitting state did not settle")
	return outfitViewData{}
}

func TestDesktopOutfittingSelectionAndFiltersUseSharedReadOnlyClient(t *testing.T) {
	var calls atomic.Int32
	client := testOutfitHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/api-system-v1/stations" {
			fmt.Fprint(w, outfitStationFixture)
		} else {
			fmt.Fprint(w, outfitModuleFixture)
		}
	})
	a := desktopOutfitTestApp(t, client)
	a.ShowOutfitting(true)
	if calls.Load() != 0 {
		t.Fatal("opening view made a passive network request")
	}
	if err := a.SearchOutfittingStations("Sol", false); err != nil {
		t.Fatal(err)
	}
	stations := waitDesktopOutfit(t, a, func(o outfitViewData) bool { return !o.Loading && len(o.Stations) == 2 })
	if err := a.SelectOutfittingStation("old-system/wrong-key", false); err == nil {
		t.Fatal("stale station selection accepted")
	}
	if err := a.SelectOutfittingStation(stations.Stations[0].Key, false); err != nil {
		t.Fatal(err)
	}
	view := waitDesktopOutfit(t, a, func(o outfitViewData) bool { return !o.Loading && len(o.Modules) == 4 })
	if view.Station == nil || view.Station.Name != "Example Orbital" || view.UpdatedLabel == "" {
		t.Fatal("station identity/age was lost")
	}
	if err := a.FilterOutfitting("", "drive", "Internal modules", "5", "A"); err != nil {
		t.Fatal(err)
	}
	view = a.GetState(80).Outfitting
	if len(view.Modules) != 1 || calls.Load() != 2 || len(a.model.requests) != 0 {
		t.Fatal("filter requested network/flight input or did not filter")
	}
	if err := a.FilterOutfitting("", "", "bad", "", "A"); err == nil {
		t.Fatal("invalid type accepted")
	}
	if err := a.FilterOutfitting("", "", "All types", "5", "Z"); err == nil {
		t.Fatal("invalid rating accepted")
	}
	a.ShowOutfitting(false)
	if a.GetState(80).Outfitting.Active {
		t.Fatal("closed viewer remained visible")
	}
}

func TestDesktopOutfittingRequestsStopOnLeaveModeChangeAndShutdown(t *testing.T) {
	for _, action := range []string{"close-view", "leave-page", "mode-switch", "shutdown"} {
		t.Run(action, func(t *testing.T) {
			entered, closed := make(chan struct{}), make(chan struct{})
			client := testOutfitHTTP(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
				fmt.Fprint(w, `{"name":"Sol","stations":[`)
				w.(http.Flusher).Flush()
				close(entered)
				<-r.Context().Done()
				close(closed)
			})
			a := desktopOutfitTestApp(t, client)
			if err := a.SearchOutfittingStations("Sol", false); err != nil {
				t.Fatal(err)
			}
			<-entered
			switch action {
			case "close-view":
				a.ShowOutfitting(false)
			case "leave-page":
				a.SelectPage(pageShip)
			case "mode-switch":
				if err := a.SetMock(true); err != nil {
					t.Fatal(err)
				}
			case "shutdown":
				a.shutdown(context.Background())
			}
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("closed view/session kept an active HTTP connection")
			}
			if a.GetState(80).Outfitting.Active {
				t.Fatal("cancelled response reopened the old viewer")
			}
			if len(a.model.requests) != 0 {
				t.Fatal("outfitting lookup emitted game controls")
			}
		})
	}
}
