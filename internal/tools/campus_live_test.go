package tools

import (
	"context"
	"os"
	"testing"
	"time"
)

// A manually supplied test credential is never persisted or printed. Product
// code uses the Station authorization client, never a shared CLI config.
type liveCampusGrant string

func (g liveCampusGrant) AccessTokenFor(context.Context, string) (string, error) {
	return string(g), nil
}

func TestLiveCampusOfferingsAndSchedule(t *testing.T) {
	if os.Getenv("HDU_STATION_LIVE_CAMPUS") != "1" {
		t.Skip("explicit campus read acceptance only")
	}
	pat := os.Getenv("HDU_STATION_CAMPUS_TEST_TOKEN")
	if pat == "" {
		t.Fatal("supply the temporary test credential through the environment")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	s := NewCampusSession(NewCampusClient(liveCampusGrant(pat)), nil)
	input := OfferingInput{Courses: []string{"戏曲鉴赏", "影视音乐鉴赏"}, CoreKeywords: []CourseKeyword{{Course: "影视音乐鉴赏", Keyword: "影视音乐"}}}
	r, err := s.CheckOfferings(ctx, input)
	if err != nil || !r.Term.valid() || len(r.Queries) != 2 {
		t.Fatal("live offering lookup failed; content withheld")
	}
	exact, candidates := 0, 0
	for _, q := range r.Queries {
		if q.Warning != "" {
			t.Fatal("live offering lookup returned a warning; content withheld")
		}
		exact += len(q.Classes)
		candidates += len(q.Candidates)
	}
	if exact == 0 {
		t.Fatal("no exact teaching class to exercise live fit")
	}
	fitted, err := s.FitCourses(ctx, FitCoursesInput{Courses: []string{"戏曲鉴赏"}, SchoolYear: r.Term.SchoolYear, Semester: r.Term.Semester})
	if err != nil || !fitted.ScheduleComplete || len(fitted.Fits) == 0 {
		t.Fatal("live full-schedule fit failed; content withheld")
	}
	statuses := map[string]int{}
	for _, fit := range fitted.Fits {
		statuses[fit.Status]++
	}
	t.Logf("term=%s/%d; exact classes=%d; candidate course IDs=%d; complete schedule=%t; fit statuses=%v; requests=%d", r.Term.SchoolYear, r.Term.Semester, exact, candidates, fitted.ScheduleComplete, statuses, s.requests)
}
