package tools

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func candidateStateFixture(t *testing.T) (*CampusSession, *atomic.Int32) {
	t.Helper()
	searches := &atomic.Int32{}
	s := campusFixture(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/config"):
			return campusResponse(200, campusConfigJSON), nil
		case strings.HasSuffix(r.URL.Path, "/map"):
			return campusResponse(200, campusMapJSON), nil
		case strings.HasSuffix(r.URL.Path, "/schedule"):
			return campusResponse(200, `{"code":0,"data":[],"pagination":{"total":0,"limit":200,"offset":0,"hasMore":false}}`), nil
		case strings.HasSuffix(r.URL.Path, "/search"):
			searches.Add(1)
			name := r.URL.Query().Get("query")
			switch name {
			case "影视音乐鉴赏":
				return campusResponse(200, catalogJSON()), nil
			case "影视音乐":
				return campusResponse(200, catalogJSON(
					catalogClass("film-morning", "001", "影视音乐赏析", "星期二第1-2节{1-17周}"),
					catalogClass("film-evening", "001", "影视音乐赏析", "星期二第10-11节{1-17周}"),
					catalogClass("film-other", "004", "影视音乐欣赏", "星期三第1-2节{1-17周}"))), nil
			case "戏曲鉴赏":
				return campusResponse(200, catalogJSON(catalogClass("opera", "002", name, "星期五第6-7节{1-17周}"))), nil
			default:
				return campusResponse(200, catalogJSON(catalogClass(name+"-class", name+"-course", name, "星期四第10-11节{1-17周}"))), nil
			}
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			return campusResponse(404, ""), nil
		}
	})
	return s, searches
}
func candidateQueryNames(r *OfferingResult) []string {
	if r == nil {
		return nil
	}
	names := []string{}
	for _, q := range r.Queries {
		names = append(names, q.Query)
	}
	return names
}
func candidateAliasInput() OfferingInput {
	return OfferingInput{Courses: []string{"影视音乐鉴赏", "戏曲鉴赏"}, CoreKeywords: []CourseKeyword{{Course: "影视音乐鉴赏", Keyword: "影视音乐"}}}
}
func TestCandidateStateRetainsIndependentQueriesAndRecomputesAllFits(t *testing.T) {
	s, _ := candidateStateFixture(t)
	ctx := context.Background()
	if _, err := s.CheckOfferings(ctx, candidateAliasInput()); err != nil {
		t.Fatal(err)
	}
	selected, err := s.FitCourses(ctx, FitCoursesInput{Courses: []string{"影视音乐鉴赏"}, CoreKeywords: []CourseKeyword{{Course: "影视音乐鉴赏", Keyword: "影视音乐"}}, CourseIDs: []string{"001"}})
	if err != nil || len(selected.Fits) != 3 || len(selected.SuggestedPlan) != 2 {
		t.Fatalf("selection discarded exact query: fits=%d plan=%v err=%v", len(selected.Fits), selected.SuggestedPlan, err)
	}
	if got := candidateQueryNames(selected.Offerings); !reflect.DeepEqual(got, []string{"影视音乐鉴赏", "戏曲鉴赏"}) {
		t.Fatalf("queries %v", got)
	}
	_, err = s.CheckOfferings(ctx, OfferingInput{Courses: []string{"书法鉴赏"}})
	if err != nil {
		t.Fatal(err)
	}
	display, err := s.ShowCourses(ctx, ShowCoursesInput{AllowedDays: []int{2, 4}, TimeOfDay: []string{"evening"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.lastFit.Fits) != 4 || len(s.lastFit.SuggestedPlan) != 2 {
		t.Fatalf("not all accumulated classes recomputed: %+v", s.lastFit)
	}
	statuses := map[string]string{}
	for _, f := range s.lastFit.Fits {
		statuses[f.Offering.ClassID] = f.Status
	}
	want := map[string]string{"film-morning": "outside_preferences", "film-evening": "fits", "opera": "outside_preferences", "书法鉴赏-class": "fits"}
	if !reflect.DeepEqual(statuses, want) {
		t.Fatalf("statuses %v", statuses)
	}
	if s.lastOfferings.Queries[0].NeedsSelection || !strings.Contains(display, "戏曲鉴赏 / 002") {
		t.Fatal("show undid selection or lost exact result")
	}
	// Re-querying an alias without new IDs must preserve its already chosen course.
	r, _ := s.CheckOfferings(ctx, OfferingInput{Courses: []string{"影视音乐鉴赏"}, CoreKeywords: []CourseKeyword{{Course: "影视音乐鉴赏", Keyword: "影视音乐"}}})
	if len(r.Queries) != 3 || r.Queries[0].NeedsSelection || len(r.Queries[0].Classes) != 2 {
		t.Fatalf("same query was duplicated or selection reset: %+v", r.Queries)
	}
}
func TestCandidateStateExplicitReplacementAndSemesterIsolation(t *testing.T) {
	s, _ := candidateStateFixture(t)
	ctx := context.Background()
	_, _ = s.CheckOfferings(ctx, candidateAliasInput())
	r, _ := s.CheckOfferings(ctx, OfferingInput{Courses: []string{"书法鉴赏"}, ReplaceCandidates: true})
	if got := candidateQueryNames(&r); !reflect.DeepEqual(got, []string{"书法鉴赏"}) {
		t.Fatalf("replacement %v", got)
	}
	_, _ = s.FitCourses(ctx, FitCoursesInput{AllowedDays: []int{1}})
	r, _ = s.CheckOfferings(ctx, OfferingInput{Courses: []string{"戏曲鉴赏"}, SchoolYear: "2025-2026", Semester: 2})
	if len(r.Queries) != 1 || s.scheduleRequested || s.lastFit != nil || len(s.fitPreferences.AllowedDays) > 0 {
		t.Fatal("different semester retained queries or fit preferences")
	}
	fit, _ := s.FitCourses(ctx, FitCoursesInput{})
	if fit.Term != r.Term || len(fit.Fits) != 1 || fit.Fits[0].Status != "fits" {
		t.Fatalf("semester reuse %v %+v", fit.Term, fit.Fits)
	}
	cleared, _ := s.FitCourses(ctx, FitCoursesInput{ReplaceCandidates: true})
	if cleared.Offerings != nil || len(cleared.Fits) > 0 || s.HasCourseResults() {
		t.Fatal("explicit empty replacement retained candidates")
	}
	fresh, _ := candidateStateFixture(t)
	if fresh.HasCourseResults() || fresh.lastFit != nil {
		t.Fatal("new session reused another conversation")
	}
}
func TestCandidateStateLimitRejectsNewQueriesWithoutDiscardingExisting(t *testing.T) {
	for _, viaFit := range []bool{false, true} {
		t.Run(fmt.Sprintf("fit-%v", viaFit), func(t *testing.T) {
			s, searches := candidateStateFixture(t)
			ctx := context.Background()
			names := []string{}
			for i := 0; i < 12; i++ {
				names = append(names, fmt.Sprintf("测试课程%02d", i))
			}
			_, _ = s.CheckOfferings(ctx, OfferingInput{Courses: names})
			before := searches.Load()
			if viaFit {
				_, _ = s.FitCourses(ctx, FitCoursesInput{Courses: []string{"第十三门课"}})
			} else {
				_, _ = s.CheckOfferings(ctx, OfferingInput{Courses: []string{"第十三门课"}})
			}
			if searches.Load() != before || len(s.lastOfferings.Queries) != 12 || !strings.Contains(s.Display(), candidateLimitWarning) {
				t.Fatalf("capacity silently lost data or issued query: %d -> %d", before, searches.Load())
			}
			_, _ = s.CheckOfferings(ctx, OfferingInput{Courses: []string{names[0]}})
			if len(s.lastOfferings.Queries) != 12 {
				t.Fatal("existing query was not allowed at capacity")
			}
			_, _ = s.CheckOfferings(ctx, OfferingInput{Courses: []string{"第十三门课"}, ReplaceCandidates: true})
			if len(s.lastOfferings.Queries) != 1 || searches.Load() != before+1 {
				t.Fatal("explicit narrowing did not release capacity")
			}
		})
	}
}
func TestCandidateStatePendingTimeChecksNeverEnterPlan(t *testing.T) {
	s, _ := candidateStateFixture(t)
	ctx := context.Background()
	in := candidateAliasInput()
	r, _ := s.FitCourses(ctx, FitCoursesInput{Courses: in.Courses, CoreKeywords: in.CoreKeywords})
	if len(r.CandidateFits) != 3 || len(r.Fits) != 1 || !reflect.DeepEqual(r.SuggestedPlan, []string{"opera"}) {
		t.Fatalf("pending candidate entered plan: %+v", r)
	}
}

func TestCandidateStateDefaultTermDoesNotInheritExplicitOldTermPreferences(t *testing.T) {
	cases := []struct {
		name           string
		in             FitCoursesInput
		days, sections []int
		status         string
	}{
		{name: "no-new-preferences", in: FitCoursesInput{}, status: "fits"},
		{name: "new-weekday", in: FitCoursesInput{AllowedDays: []int{4}}, days: []int{4}, status: "fits"},
		{name: "new-afternoon", in: FitCoursesInput{TimeOfDay: []string{"afternoon"}}, sections: []int{6, 7, 8, 9}, status: "outside_preferences"},
		{name: "new-explicit-sections", in: FitCoursesInput{AllowedSections: []int{10, 11}}, sections: []int{10, 11}, status: "fits"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := candidateStateFixture(t)
			ctx := context.Background()
			_, _ = s.FitCourses(ctx, FitCoursesInput{Courses: []string{"戏曲鉴赏"}, SchoolYear: "2025-2026", Semester: 1, AllowedDays: []int{1}, TimeOfDay: []string{"evening"}})
			in := tc.in
			in.Courses = []string{"书法鉴赏"}
			r, err := s.FitCourses(ctx, in)
			if err != nil || r.Term != (AcademicTerm{SchoolYear: "2026-2027", Semester: 1}) || len(r.Fits) != 1 {
				t.Fatalf("default term not resolved: term=%+v fits=%d err=%v", r.Term, len(r.Fits), err)
			}
			if r.Fits[0].Status != tc.status || !reflect.DeepEqual(s.fitPreferences.AllowedDays, tc.days) || !reflect.DeepEqual(s.fitPreferences.AllowedSections, tc.sections) {
				t.Fatalf("old preferences leaked or new input lost: status=%s days=%v sections=%v", r.Fits[0].Status, s.fitPreferences.AllowedDays, s.fitPreferences.AllowedSections)
			}
			if !s.scheduleRequested {
				t.Fatal("term reset erased current fit request")
			}
		})
	}
}
func TestCandidateStateResolvedSameTermStillInheritsPreferences(t *testing.T) {
	s, _ := candidateStateFixture(t)
	ctx := context.Background()
	_, _ = s.FitCourses(ctx, FitCoursesInput{Courses: []string{"戏曲鉴赏"}, SchoolYear: "2026-2027", Semester: 1, AllowedDays: []int{1}, TimeOfDay: []string{"evening"}})
	r, err := s.FitCourses(ctx, FitCoursesInput{Courses: []string{"书法鉴赏"}})
	if err != nil || len(r.Fits) != 2 || !reflect.DeepEqual(s.fitPreferences.AllowedDays, []int{1}) || !reflect.DeepEqual(s.fitPreferences.AllowedSections, []int{10, 11, 12, 13}) {
		t.Fatal("resolved same-term preferences not inherited")
	}
	for _, f := range r.Fits {
		if f.Status != "outside_preferences" {
			t.Fatal("same-term constraint was silently cleared")
		}
	}
}
