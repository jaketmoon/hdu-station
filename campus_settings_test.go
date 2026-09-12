package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jaketmoon/hdu-station/internal/campusauth"
	"github.com/jaketmoon/hdu-station/internal/config"
)

func TestCampusWebSettingsKeepCredentialsPrivateAndLogoutIsPersistent(t *testing.T) {
	a := testApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.ctx = ctx
	var err error
	a.campus, err = campusauth.New(a.root, "campus-private-sentinel")
	if err != nil {
		t.Fatal(err)
	}
	settings, err := a.SaveSettings(SettingsInput{BaseURL: config.Default().Model.BaseURL})
	if err != nil || !settings.Campus.HasCredential {
		t.Fatal("saving model settings disturbed campus login")
	}
	if settings.Campus.Status != "saved" || a.CheckCampus().Status != "saved" {
		t.Fatal("local login status still depends on a canceled network request")
	}
	data, _ := json.Marshal(settings)
	if strings.Contains(string(data), "private-sentinel") || strings.Contains(string(data), "accessToken") {
		t.Fatal("campus credential exposed")
	}
	connection, err := a.LogoutCampus()
	if err != nil || connection.HasCredential || connection.Status != "logged_out" {
		t.Fatal("campus logout failed")
	}
	reloaded, err := campusauth.New(a.root, "campus-private-sentinel")
	if err != nil {
		t.Fatal(err)
	}
	defer reloaded.Close()
	if reloaded.Configured() {
		t.Fatal("logout restored migrated credential")
	}
}

func TestCampusAccountChangesAreBlockedDuringAnAnswer(t *testing.T) {
	a := testApp(t)
	a.active = &activeTurn{}
	if _, err := a.BeginCampusLogin(); err == nil {
		t.Fatal("authorization allowed during an answer")
	}
	if _, err := a.LogoutCampus(); err == nil {
		t.Fatal("logout allowed during an answer")
	}
	a.active = nil
	a.openCampusBrowser = nil
	if _, err := a.BeginCampusLogin(); err == nil {
		t.Fatal("unavailable browser accepted")
	}
	if a.sourceBusy {
		t.Fatal("failed start left settings busy")
	}
	if result := a.PollCampusLogin("old"); result.Status != "cancelled" {
		t.Fatal("unknown session not canceled")
	}
}
