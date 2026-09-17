package campusauth

import (
	"context"
	"strings"
	"testing"
)

func TestSimulationScopeUpgradePreservesOldGrant(t *testing.T) {
	root := t.TempDir()
	old := credentials{Version: 1, DeviceID: "test-device", Token: "test-private-token", Managed: true, Scopes: []string{CourseScope, ScheduleScope}}
	if err := saveCredentials(root, old); err != nil {
		t.Fatal(err)
	}
	c, err := New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.Status() != "saved" {
		t.Fatal("old login invalidated")
	}
	for _, scope := range old.Scopes {
		if _, err := c.AccessTokenFor(context.Background(), scope); err != nil {
			t.Fatal("old read permission lost")
		}
	}
	for _, scope := range []string{SimulationReadScope, SimulationWriteScope, FavoriteReadScope, FavoriteWriteScope} {
		if c.HasScope(scope) {
			t.Fatal("unapproved simulation permission advertised")
		}
		if _, err := c.AccessTokenFor(context.Background(), scope); err != ErrScope {
			t.Fatal("simulation permission bypassed")
		}
	}
}

func TestGrantedScopesRequireExactSetInAnyOrder(t *testing.T) {
	if !validGrantedScopes([]string{FavoriteWriteScope, SimulationWriteScope, ScheduleScope, FavoriteReadScope, SimulationReadScope, CourseScope}) {
		t.Fatal("reordered grant rejected")
	}
	for _, missing := range strings.Fields(requestedScope) {
		if validGrantedScopes(strings.Fields(strings.Replace(requestedScope, missing, "", 1))) {
			t.Fatal("incomplete grant accepted")
		}
	}
	if validGrantedScopes([]string{CourseScope, ScheduleScope, SimulationReadScope, SimulationReadScope, FavoriteReadScope, FavoriteWriteScope}) {
		t.Fatal("duplicate scope replaced required write grant")
	}
	if validGrantedScopes(append(strings.Fields(requestedScope), "academic:grade:read")) {
		t.Fatal("unrequested permission accepted")
	}
}

func TestFavoriteUpgradeKeepsSimulationPermissions(t *testing.T) {
	root := t.TempDir()
	old := credentials{Version: 1, DeviceID: "test-device", Token: "test-private-token", Managed: true, Scopes: []string{CourseScope, ScheduleScope, SimulationReadScope, SimulationWriteScope}}
	if err := saveCredentials(root, old); err != nil {
		t.Fatal(err)
	}
	c, err := New(root, "")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, scope := range old.Scopes {
		if _, err := c.AccessTokenFor(context.Background(), scope); err != nil {
			t.Fatal("existing permission lost")
		}
	}
	for _, scope := range []string{FavoriteReadScope, FavoriteWriteScope} {
		if c.HasScope(scope) {
			t.Fatal("unapproved favorite permission advertised")
		}
		if _, err := c.AccessTokenFor(context.Background(), scope); err != ErrScope {
			t.Fatal("favorite access bypassed upgrade")
		}
	}
}
