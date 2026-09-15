package tools

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestShowSelectsCachedCandidateWithoutDiscardingOtherQueries(t *testing.T) {
	s, _ := candidateStateFixture(t)
	in := candidateAliasInput()
	_, err := s.FitCourses(context.Background(), FitCoursesInput{Courses: in.Courses, CoreKeywords: in.CoreKeywords, TimeOfDay: []string{"evening"}})
	if err != nil {
		t.Fatal(err)
	}
	before := s.requests
	for _, ids := range [][]string{{"001"}, {"001", "001"}, {"004"}, {"001", "004"}} {
		shown, err := s.ShowCourses(context.Background(), ShowCoursesInput{CourseIDs: ids})
		if err != nil || s.requests != before {
			t.Fatalf("selection issued HTTP: %d -> %d (%v)", before, s.requests, err)
		}
		if len(s.lastOfferings.Queries) != 2 || s.lastOfferings.Queries[0].NeedsSelection || !strings.Contains(shown, "戏曲鉴赏 / 002") || strings.Contains(shown, "有候选名称尚未选择") {
			t.Fatal("selection lost other queries or stayed pending")
		}
		if reflect.DeepEqual(ids, []string{"004"}) {
			if len(s.lastFit.SuggestedPlan) != 0 {
				t.Fatal("switching identity ignored retained evening preference")
			}
		} else if !reflect.DeepEqual(s.lastFit.SuggestedPlan, []string{"film-evening"}) {
			t.Fatalf("candidate did not enter correct plan: %v", s.lastFit.SuggestedPlan)
		}
	}
}

func TestShowSelectionIsOptionalAndNeverInventsOrReadsSchedule(t *testing.T) {
	for _, ids := range [][]string{nil, {}, {"does-not-exist"}, {"1"}, {"001"}} {
		s, _ := candidateStateFixture(t)
		_, _ = s.CheckOfferings(context.Background(), candidateAliasInput())
		before := s.requests
		shown, err := s.ShowCourses(context.Background(), ShowCoursesInput{CourseIDs: ids, TimeOfDay: []string{"evening"}})
		if err != nil || s.requests != before || s.lastFit != nil || s.scheduleRequested || strings.Contains(shown, "可放入空闲位置") {
			t.Fatal("offering selection read/invented schedule")
		}
		chosen := len(ids) > 0 && ids[0] == "001"
		if s.lastOfferings.Queries[0].NeedsSelection == chosen {
			t.Fatal("unknown/absent ID promoted, or valid ID not selected")
		}
		if chosen && (!strings.Contains(shown, "符合指定时间；未核对本人课表") || strings.Contains(shown, "名称尚待匹配的官方候选")) {
			t.Fatal("selection display lost known offering-only time check")
		}
	}
	s, _ := candidateStateFixture(t)
	_, _ = s.CheckOfferings(context.Background(), candidateAliasInput())
	old := s.lastOfferings
	_, err := s.ShowCourses(context.Background(), ShowCoursesInput{CourseIDs: []string{"bad id"}})
	if err == nil || s.lastOfferings != old {
		t.Fatal("malformed ID mutated state")
	}
}

func TestShowSelectionPreservesIncompleteAndConflictingEvidenceWithoutHTTP(t *testing.T) {
	s, _ := candidateStateFixture(t)
	in := candidateAliasInput()
	_, _ = s.FitCourses(context.Background(), FitCoursesInput{Courses: in.Courses, CoreKeywords: in.CoreKeywords})
	// This is the existing result shape after a missing timetable page: retain
	// a known conflict, but never approve compatible-looking unknown classes.
	s.schedules = map[AcademicTerm]scheduleResult{}
	s.lastFit.ScheduleComplete = false
	s.lastFit.CandidateFits[0].Status = "conflict"
	s.lastFit.CandidateFits[0].Conflicts = []CourseTime{{Day: 2, Sections: []int{1}, Weeks: []int{1}}}
	s.lastFit.CandidateFits[1].Status = "unknown"
	s.lastFit.Fits[0].Status = "unknown"
	before := s.requests
	shown, err := s.ShowCourses(context.Background(), ShowCoursesInput{CourseIDs: []string{"001"}})
	if err != nil || s.requests != before || s.lastFit.ScheduleComplete || len(s.lastFit.SuggestedPlan) != 0 || !strings.Contains(shown, "与已选课程冲突") || !strings.Contains(shown, "课表或上课时间未完整核实") {
		t.Fatal("cached incomplete evidence was reread or upgraded")
	}
}

func TestShowSelectionHonorsExplicitScheduleRequest(t *testing.T) {
	s, _ := candidateStateFixture(t)
	_, _ = s.CheckOfferings(context.Background(), candidateAliasInput())
	before := s.requests
	_, err := s.ShowCourses(context.Background(), ShowCoursesInput{CourseIDs: []string{"001"}, MatchSchedule: true})
	if err != nil || s.requests != before+1 || s.lastFit == nil || len(s.lastFit.SuggestedPlan) != 2 {
		t.Fatalf("explicit new schedule request changed: %d -> %d %v", before, s.requests, err)
	}
}
