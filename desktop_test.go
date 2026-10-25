//go:build desktop

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestDesktopPreferencesPersistDisplayOnlyAndRejectCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Co-pit", "window.json")
	p, err := loadDesktopPreferences(path)
	if err != nil || p != defaultDesktopPreferences() {
		t.Fatal("missing settings must use windowed auto layout", p, err)
	}
	p.Layout, p.Theme, p.Fullscreen, p.Scanlines = "portrait", "retro-ops", true, true
	if err := writeDesktopPreferences(path, p); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadDesktopPreferences(path)
	p.Fullscreen = false
	if err != nil || loaded != p {
		t.Fatal("display preferences did not survive restart", loaded, err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "mock") || strings.Contains(string(data), "armed") || strings.Contains(string(data), "fullscreen") || strings.Contains(string(data), "borderless") {
		t.Fatal("unsafe session state persisted")
	}
	for _, bad := range []string{`{"version":2,"theme":"retro-ops","layout":"auto"}`, `{"version":1,"theme":"retro-ops","layout":"sideways"}`, `{} garbage`, strings.Repeat("x", 4097)} {
		if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		loaded, err = loadDesktopPreferences(path)
		if err == nil || loaded != defaultDesktopPreferences() {
			t.Fatal("corrupt preferences must be visible and use safe defaults", loaded, err)
		}
	}
}

func TestDesktopFullscreenExitWorksWhenSettingsCannotBeWritten(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	a := newDesktopApp(defaultDesktopPreferences(), filepath.Join(blocker, "window.json"))
	a.prefs.Fullscreen = true
	var called bool
	a.fullscreenWindow = func(enabled bool) {
		called = true
		if enabled {
			t.Fatal("exit requested but fullscreen enabled")
		}
	}
	if err := a.SetFullscreen(false); err != nil {
		t.Fatal("runtime fullscreen exit must not write settings", err)
	}
	if !called || a.prefs.Fullscreen {
		t.Fatal("save failure trapped the user in fullscreen")
	}
	before := a.prefs
	changed := before
	changed.Theme = "flight-console"
	if err := a.SavePreferences(changed); err == nil || a.prefs != before {
		t.Fatal("failed appearance save changed saved preferences")
	}
}

func TestDesktopModeTransitionJoinsOldSessionAndShutdownRejectsRestart(t *testing.T) {
	a := newDesktopApp(defaultDesktopPreferences(), filepath.Join(t.TempDir(), "window.json"))
	a.ctx = context.Background()
	old := newSessionWork(a.ctx)
	a.work = old
	entered, cleaned := make(chan struct{}), make(chan struct{})
	go old.command(0, func() tea.Msg { close(entered); <-old.ctx.Done(); close(cleaned); return nil })()
	<-entered
	if err := a.SetMock(true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cleaned:
	default:
		t.Fatal("mock entered before old worker cleanup")
	}
	if a.work == old || a.model.opts.mode != "mock" || a.model.frame.ControlStatus == "ARMED" {
		t.Fatal("mode switch retained real session state")
	}
	a.SelectPage(pageEngineering)
	a.PageKey("Enter")
	if !a.model.goalEditing {
		t.Fatal("shared target editor unavailable")
	}
	a.SelectPage(pageShip)
	if a.model.goalEditing {
		t.Fatal("navigation kept an unfinished target edit")
	}
	done := make(chan struct{})
	go func() { a.shutdown(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not join workers")
	}
	if err := a.OpenStationSearch(0); err == nil {
		t.Fatal("closed app opened a browser")
	}
	if err := a.SetMock(true); err == nil || a.work != nil {
		t.Fatal("closed desktop restarted workers")
	}
}

func TestDesktopCockpitButtonsStayGuardedAndStopReplacesClick(t *testing.T) {
	a := newDesktopApp(defaultDesktopPreferences(), filepath.Join(t.TempDir(), "window.json"))
	a.ctx = context.Background()
	a.opts.settings, a.opts.goals, a.opts.firegroups = "", "", ""
	a.startSession(true)
	defer a.shutdown(context.Background())
	if err := a.PressControl("t3"); err != nil || !a.model.frame.State.has(lights) {
		t.Fatal("mock light button did not follow simulated feedback", err)
	}
	if err := a.PressControl("b1"); err != nil || !a.model.frame.State.has(nightVision) {
		t.Fatal("mock night vision button failed", err)
	}
	if err := a.PressControl("unknown"); err == nil {
		t.Fatal("unknown action accepted")
	}
	a.model.mock = nil
	a.model.opts.mode = "control"
	a.model.frame.ControlStatus = "DISARMED"
	if err := a.PressControl("t3"); err == nil || len(a.model.requests) != 0 {
		t.Fatal("UI click armed the controller")
	}
	a.model.frame.ControlStatus = "PAUSED"
	if err := a.PressControl("t3"); err != nil {
		t.Fatal(err)
	}
	queued := <-a.model.requests
	if queued.Kind != "action" || queued.Action != "t3" || time.Since(queued.Created) > time.Second {
		t.Fatal("click did not carry a short-lived action")
	}
	a.model.requests <- queued
	a.Disarm()
	if len(a.model.requests) != 1 || (<-a.model.requests).Kind != "stop" {
		t.Fatal("Stop left a waiting click")
	}
}

func TestDesktopEngineeringFormFiltersSelectsAndSavesTargets(t *testing.T) {
	dir := t.TempDir()
	a := newDesktopApp(defaultDesktopPreferences(), filepath.Join(dir, "window.json"))
	a.ctx = context.Background()
	a.opts.settings = ""
	a.opts.goals = filepath.Join(dir, "goals.json")
	a.opts.firegroups = ""
	a.startSession(true)
	defer a.shutdown(context.Background())
	a.SelectPage(pageEngineering)
	a.FilterMaterials("arsenic", false)
	view := a.GetState(80)
	if len(view.Engineering.Rows) != 1 || view.Engineering.Selected != "" || view.Engineering.Rows[0].Count != 12 {
		t.Fatal("visible search form did not find the material")
	}
	a.SelectMaterial("raw/arsenic")
	if err := a.SaveMaterialTarget("raw/arsenic", 20); err != nil {
		t.Fatal(err)
	}
	if a.GetState(80).Engineering.Rows[0].Target != 20 {
		t.Fatal("target form did not update")
	}
	if _, err := os.Stat(a.opts.goals); !os.IsNotExist(err) {
		t.Fatal("mock form wrote real preferences")
	}
	a.model.opts.mode = "live"
	if err := a.SaveMaterialTarget("raw/arsenic", 25); err != nil {
		t.Fatal(err)
	}
	saved, err := loadMaterialGoals(a.opts.goals)
	if err != nil || saved["raw/arsenic"] != 25 {
		t.Fatal("real checklist target did not save", err)
	}
	if err := a.SaveMaterialTarget("raw/arsenic", 1000000); err == nil {
		t.Fatal("invalid quantity accepted")
	}
	if err := a.SaveMaterialTarget("raw/arsenic", 0); err != nil {
		t.Fatal(err)
	}
	a.FilterMaterials("", true)
	if len(a.GetState(80).Engineering.Rows) != 0 {
		t.Fatal("removed material remained pinned")
	}
}
