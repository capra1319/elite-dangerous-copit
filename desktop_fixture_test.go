//go:build desktop

package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestExportDesktopUpgradeFixtures(t *testing.T) {
	path := os.Getenv("COPIT_FRONTEND_FIXTURE")
	if path == "" {
		t.Skip("opt-in simulated frontend fixtures")
	}
	a := layoutTestApp(t)
	if err := a.SaveControlLayout([]string{"b5", "b6"}); err != nil {
		t.Fatal(err)
	}
	states := map[int]DesktopState{}
	for _, page := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8} {
		a.model.page = page
		states[page] = a.GetState(110)
	}
	data, err := json.Marshal(states)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
