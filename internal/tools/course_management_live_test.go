package tools

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jaketmoon/hdu-station/internal/campusauth"
	"github.com/jaketmoon/hdu-station/internal/config"
)

// Explicit read-only smoke check against saved Station credentials. Do not log
// tokens, course identities, original lists or raw response bodies.
func TestLiveCourseManagementReads(t *testing.T) {
	if os.Getenv("HDU_STATION_MANAGEMENT_READ_TEST") != "1" {
		t.Skip("explicit live read-only acceptance")
	}
	root, err := config.Root()
	if err != nil {
		t.Fatal("data root unavailable")
	}
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal("configuration unavailable")
	}
	auth, err := campusauth.New(root, cfg.CampusKey)
	if err != nil {
		t.Fatal("authorization unavailable")
	}
	defer auth.Close()
	for _, scope := range []string{campusauth.FavoriteReadScope, campusauth.FavoriteWriteScope, campusauth.SimulationReadScope, campusauth.SimulationWriteScope} {
		if !auth.HasScope(scope) {
			t.Fatal("required management grant missing")
		}
	}
	s := NewCampusSession(NewCampusClient(auth), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	f, err := s.ManageFavorites(ctx, FavoriteInput{Action: "read"})
	if err != nil || f.Status != "read" {
		t.Fatalf("favorite read: %s", f.Message)
	}
	t.Logf("favorites readable: %d entries", len(f.Entries))
	rank, err := s.ManageFavorites(ctx, FavoriteInput{Action: "rank"})
	if err != nil || rank.Status != "read" {
		t.Fatalf("favorite rank: %s", rank.Message)
	}
	r, err := s.ManageSimulation(ctx, SimulationInput{Action: "read"})
	if err != nil || r.Status != "read" {
		t.Fatalf("simulation read: %s", r.Message)
	}
	fit, fitErr := s.FitCourses(ctx, FitCoursesInput{Courses: []string{"西方建筑艺术赏析"}})
	if fitErr != nil || fit.ScheduleSource != "simulation" || !fit.ScheduleComplete || fit.SimulationRevision < 1 || len(fit.Fits) == 0 {
		t.Fatal("simulation-based live fit failed")
	}
	t.Logf("simulation fit verified: %d candidates; revision=%d", len(fit.Fits), fit.SimulationRevision)
	t.Logf("simulation readable: %d actual courses, %d simulation items; no write requested", len(r.Data.ActualCourses), len(r.Data.SimulationItems))
}
