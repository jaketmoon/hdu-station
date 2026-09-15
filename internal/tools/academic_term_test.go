package tools

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestAcademicTermQuestionNeedsOnlyDefaultConfig(t *testing.T) {
	requests := 0
	s := campusFixture(t, func(r *http.Request) (*http.Response, error) {
		requests++
		if !strings.HasSuffix(r.URL.Path, "/config") {
			t.Error("metadata request read classes or timetable")
		}
		return campusResponse(200, campusConfigJSON), nil
	})
	result, err := s.ReadAcademicTerm(context.Background(), AcademicTermInput{})
	if err != nil || result.Term != (AcademicTerm{"2026-2027", 1}) || requests != 1 || !s.HasQueryResult() || !s.CanShowCourses() {
		t.Fatal("default term not available independently")
	}
	text, err := s.ShowCourses(context.Background(), ShowCoursesInput{})
	if err != nil || !strings.Contains(text, "2026-2027 学年第 1 学期") || strings.Contains(text, "课程 / 课程号") || strings.Contains(text, "未校验实时余量") {
		t.Fatal("term-only answer gained unrelated course flow")
	}
}
func TestAcademicTermFailureIsVisibleAndDoesNotInventCurrentTerm(t *testing.T) {
	s := NewCampusSession(nil, nil)
	result, err := s.ReadAcademicTerm(context.Background(), AcademicTermInput{})
	if err != nil || result.Warning == "" || result.Term.valid() || !strings.Contains(s.Display(), "尚未确认教务默认查询学期") {
		t.Fatal("failed default lookup was hidden or guessed")
	}
}
