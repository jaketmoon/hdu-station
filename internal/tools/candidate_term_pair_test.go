package tools

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestCandidateReuseNeverCompletesPartialExplicitTerm(t *testing.T) {
	for _, in := range []FitCoursesInput{{Semester: 1}, {Semester: 2}, {SchoolYear: "2025-2026"}} {
		s, _ := candidateStateFixture(t)
		ctx := context.Background()
		_, _ = s.CheckOfferings(ctx, OfferingInput{Courses: []string{"戏曲鉴赏"}, SchoolYear: "2025-2026", Semester: 1})
		before := s.requests
		r, err := s.FitCourses(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		if r.Term.valid() || r.Term.SchoolYear != in.SchoolYear || r.Term.Semester != in.Semester || !strings.Contains(strings.Join(r.Warnings, " "), "同时提供") {
			t.Fatalf("partial explicit term was overwritten: input=%+v result=%+v warnings=%v", in, r.Term, r.Warnings)
		}
		if s.requests != before || r.ScheduleComplete || r.Offerings != nil {
			t.Fatal("invalid term performed a query or reused old semester")
		}
	}
}
func TestCandidateReuseWithBothTermFieldsOmittedPreservesChoiceAndRecomputes(t *testing.T) {
	s, _ := candidateStateFixture(t)
	ctx := context.Background()
	in := candidateAliasInput()
	in.SchoolYear = "2025-2026"
	in.Semester = 2
	_, _ = s.CheckOfferings(ctx, in)
	first, err := s.FitCourses(ctx, FitCoursesInput{CourseIDs: []string{"001"}, AllowedDays: []int{2}, TimeOfDay: []string{"morning"}})
	if err != nil || !reflect.DeepEqual(first.SuggestedPlan, []string{"film-morning"}) {
		t.Fatal("initial candidate selection failed")
	}
	second, err := s.FitCourses(ctx, FitCoursesInput{TimeOfDay: []string{"evening"}})
	if err != nil || second.Term != (AcademicTerm{SchoolYear: "2025-2026", Semester: 2}) || len(second.Fits) != 3 || !reflect.DeepEqual(second.SuggestedPlan, []string{"film-evening"}) {
		t.Fatalf("valid omitted-term recomputation changed: %+v %v", second, err)
	}
	if len(second.Offerings.Queries) != 2 || second.Offerings.Queries[0].NeedsSelection || !reflect.DeepEqual(s.fitPreferences.AllowedDays, []int{2}) {
		t.Fatal("selected candidate or retained weekday lost")
	}
}
func TestCandidateReuseRespectsCompleteExplicitNewTerm(t *testing.T) {
	for _, withCourse := range []bool{false, true} {
		s, _ := candidateStateFixture(t)
		ctx := context.Background()
		_, _ = s.FitCourses(ctx, FitCoursesInput{Courses: []string{"戏曲鉴赏"}, SchoolYear: "2025-2026", Semester: 1, AllowedDays: []int{1}, TimeOfDay: []string{"evening"}})
		in := FitCoursesInput{SchoolYear: "2026-2027", Semester: 2, AllowedDays: []int{4}}
		if withCourse {
			in.Courses = []string{"书法鉴赏"}
		}
		r, err := s.FitCourses(ctx, in)
		if err != nil || r.Term != (AcademicTerm{SchoolYear: "2026-2027", Semester: 2}) || !r.ScheduleComplete || !reflect.DeepEqual(s.fitPreferences.AllowedDays, []int{4}) || s.fitPreferences.AllowedSections != nil {
			t.Fatalf("complete new term was altered or old preference retained: %+v %+v %v", r.Term, s.fitPreferences, err)
		}
		if withCourse {
			if len(r.Fits) != 1 || r.Fits[0].Status != "fits" {
				t.Fatal("complete new term course fit failed")
			}
		} else if r.Offerings != nil || len(r.Fits) != 0 {
			t.Fatal("old candidates crossed into explicit new term")
		}
	}
}
