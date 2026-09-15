package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func showStateInput(t *testing.T, raw string) ShowCoursesInput {
	t.Helper()
	var in ShowCoursesInput
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func TestRepeatedOfferingShowRetainsOmittedAndClearsExplicitPreferences(t *testing.T) {
	s, _ := candidateStateFixture(t)
	_, _ = s.CheckOfferings(context.Background(), candidateAliasInput())
	before := s.requests
	for _, tc := range []struct {
		raw            string
		days, sections []int
	}{
		{`{"courseIDs":["001"],"allowedDays":[2],"timeOfDay":["evening"]}`, []int{2}, []int{10, 11, 12, 13}},
		{`{}`, []int{2}, []int{10, 11, 12, 13}},
		{`{"allowedDays":[3]}`, []int{3}, []int{10, 11, 12, 13}},
		{`{"allowedDays":[]}`, []int{}, []int{10, 11, 12, 13}},
		{`{"timeOfDay":[]}`, []int{}, []int{}},
		{`{"allowedSections":[1,2]}`, []int{}, []int{1, 2}},
		{`{"allowedSections":[]}`, []int{}, []int{}},
	} {
		if _, err := s.ShowCourses(context.Background(), showStateInput(t, tc.raw)); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(s.displayPreferences.AllowedDays, tc.days) || !reflect.DeepEqual(s.displayPreferences.AllowedSections, tc.sections) {
			t.Fatalf("%s lost/failed to clear preferences: %+v", tc.raw, s.displayPreferences)
		}
		if s.requests != before || s.lastFit != nil || s.scheduleRequested {
			t.Fatal("offering display read timetable")
		}
	}
}

func TestRepeatedScheduleOnlyShowUsesItsOwnPreferences(t *testing.T) {
	s, _ := candidateStateFixture(t)
	// A previous offering-only focus must not replace the timetable focus.
	s.displayPreferences = FitCoursesInput{AllowedDays: []int{5}, AllowedSections: []int{1, 2}}
	_, _ = s.FitCourses(context.Background(), FitCoursesInput{})
	before := s.requests
	for _, tc := range []struct{ raw, want, absent string }{
		{`{"allowedDays":[2],"timeOfDay":["afternoon"]}`, "周二第6–9节", "周五"},
		{`{}`, "周二第6–9节", "周五"},
		{`{"timeOfDay":["evening"]}`, "周二第10–13节", "周二第6"},
		{`{"timeOfDay":[]}`, "周二第1–14节", "周五"},
		{`{"allowedDays":[]}`, "教务返回空课表", "| 时间 |"},
	} {
		shown, err := s.ShowCourses(context.Background(), showStateInput(t, tc.raw))
		if err != nil || !strings.Contains(shown, tc.want) || strings.Contains(shown, tc.absent) {
			t.Fatalf("%s: %s (%v)", tc.raw, shown, err)
		}
		if s.requests != before || s.HasCourseResults() {
			t.Fatal("schedule focus issued a new query")
		}
	}
	if !reflect.DeepEqual(s.displayPreferences.AllowedDays, []int{5}) || !reflect.DeepEqual(s.displayPreferences.AllowedSections, []int{1, 2}) {
		t.Fatal("schedule focus polluted offering preferences")
	}
}

func TestRepeatedFittedShowRecomputesAfterPreferenceChangesAndClears(t *testing.T) {
	s, _ := candidateStateFixture(t)
	in := candidateAliasInput()
	_, _ = s.FitCourses(context.Background(), FitCoursesInput{Courses: in.Courses, CoreKeywords: in.CoreKeywords})
	before := s.requests
	for _, tc := range []struct {
		raw        string
		fits, plan int
	}{
		{`{"courseIDs":["001"],"allowedDays":[2],"timeOfDay":["evening"]}`, 1, 1},
		{`{}`, 1, 1},
		{`{"allowedDays":[2]}`, 1, 1},
		{`{"timeOfDay":[]}`, 2, 1},
		{`{"allowedSections":[10,11]}`, 1, 1},
		{`{"allowedSections":[]}`, 2, 1},
		{`{"allowedDays":[]}`, 3, 2},
	} {
		if _, err := s.ShowCourses(context.Background(), showStateInput(t, tc.raw)); err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, f := range s.lastFit.Fits {
			if f.Status == "fits" {
				count++
			}
		}
		if count != tc.fits || len(s.lastFit.SuggestedPlan) != tc.plan {
			t.Fatalf("%s did not recompute fits/plan: %d %v", tc.raw, count, s.lastFit.SuggestedPlan)
		}
		if s.requests != before {
			t.Fatal("time preference update reread cached official data")
		}
	}
}
