package tools

import (
	"context"
	"strings"
	"testing"
)

func TestCandidateDisplayShowsPendingChecksWithoutAddingThemToPlan(t *testing.T) {
	s, _ := candidateStateFixture(t)
	ctx := context.Background()
	in := candidateAliasInput()
	r, _ := s.FitCourses(ctx, FitCoursesInput{Courses: in.Courses, CoreKeywords: in.CoreKeywords, TimeOfDay: []string{"evening"}})
	text := s.Display()
	for _, part := range []string{"名称尚待匹配的官方候选", "影视音乐赏析 / 001", "影视音乐欣赏 / 004", "名称待匹配；不符合指定时间偏好", "名称待匹配；时间与本人课表兼容", "未计入可同时安排的组合"} {
		if !strings.Contains(text, part) {
			t.Fatalf("missing %q from %s", part, text)
		}
	}
	if len(r.SuggestedPlan) != 0 || strings.Contains(text, "一组可以同时放入") {
		t.Fatal("pending time-compatible class entered plan")
	}
	for _, id := range []string{"film-morning", "film-evening", "film-other", "opera"} {
		if strings.Contains(text, id) {
			t.Fatal("class ID exposed")
		}
	}
	// Selection promotes the chosen identity; no duplicate pending table is left.
	_, _ = s.FitCourses(ctx, FitCoursesInput{CourseIDs: []string{"001"}})
	text = s.Display()
	if strings.Contains(text, "名称尚待匹配的官方候选") || !strings.Contains(text, "一组可以同时放入") || strings.Count(text, "| 影视音乐赏析 / 001 |") != 2 {
		t.Fatal("selected identity still displayed as pending or duplicated")
	}
}
func TestCandidateDisplayUsesConflictAndUnknownEvidence(t *testing.T) {
	s, _ := candidateStateFixture(t)
	in := candidateAliasInput()
	_, _ = s.FitCourses(context.Background(), FitCoursesInput{Courses: in.Courses, CoreKeywords: in.CoreKeywords})
	s.lastFit.CandidateFits[0].Status = "conflict"
	s.lastFit.CandidateFits[0].Conflicts = []CourseTime{{Day: 2, Sections: []int{1, 2}, Weeks: []int{1, 2}}}
	s.lastFit.CandidateFits[1].Status = "unknown"
	text := s.Display()
	for _, part := range []string{"名称待匹配；与已选课程冲突：第1–2周，周二第1–2节", "名称待匹配；待确认：课表或上课时间未完整核实"} {
		if !strings.Contains(text, part) {
			t.Fatalf("lost pending evidence: %q", part)
		}
	}
}
func TestCandidateDisplayOfferingOnlyNeverInventsPersonalFit(t *testing.T) {
	s, _ := candidateStateFixture(t)
	ctx := context.Background()
	_, _ = s.CheckOfferings(ctx, candidateAliasInput())
	text, err := s.ShowCourses(ctx, ShowCoursesInput{TimeOfDay: []string{"evening"}})
	if err != nil {
		t.Fatal(err)
	}
	if s.lastFit != nil || s.scheduleRequested || strings.Contains(text, "时间与本人课表兼容") || strings.Contains(text, "可放入空闲位置") {
		t.Fatal("offering-only display invented a personal check")
	}
	if !strings.Contains(text, "名称待匹配；符合指定时间；未核对本人课表") {
		t.Fatal("offering-only time comparison lost")
	}
}
