package tools

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestScheduleAloneCannotFinishAndLaterReadKeepsCandidates(t *testing.T) {
	s := campusFixture(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/config"):
			return campusResponse(200, campusConfigJSON), nil
		case strings.HasSuffix(r.URL.Path, "/map"):
			return campusResponse(200, campusMapJSON), nil
		case strings.HasSuffix(r.URL.Path, "/schedule"):
			return campusResponse(200, `{"code":0,"data":[],"pagination":{"total":0,"limit":200,"offset":0,"hasMore":false}}`), nil
		default:
			return campusResponse(200, catalogJSON(catalogClass("friday", "001", "戏曲鉴赏", "星期五第6-7节{1-16周}"))), nil
		}
	})
	ctx := context.Background()
	_, _ = s.FitCourses(ctx, FitCoursesInput{AllowedDays: []int{1, 2, 3, 4, 6, 7}})
	text, _ := s.ShowCourses(ctx, ShowCoursesInput{MatchSchedule: true})
	if text != "" || s.CompletionGap() == "" || s.CanShowCourses() {
		t.Fatal("schedule-only read ended recommendation")
	}
	if strings.Contains(s.Display(), "suggestedPlan") {
		t.Fatal("internal instructions leaked")
	}
	_, _ = s.CheckOfferings(ctx, OfferingInput{Courses: []string{"戏曲鉴赏"}})
	_, _ = s.FitCourses(ctx, FitCoursesInput{})
	text, _ = s.ShowCourses(ctx, ShowCoursesInput{})
	if !strings.Contains(text, "戏曲鉴赏") || strings.Contains(text, "friday") || strings.Contains(text, "班级号") || !strings.Contains(text, "不符合指定时间偏好") || strings.Contains(text, "一组可以同时放入") {
		t.Fatal("schedule reread lost candidates or explicit Friday preference")
	}
}

func TestUnresolvedNamesCannotFinish(t *testing.T) {
	s := NewCampusSession(nil, nil)
	s.lastOfferings = &OfferingResult{Queries: []OfferingQuery{{Query: "影视音乐", NeedsSelection: true}}}
	text, _ := s.ShowCourses(context.Background(), ShowCoursesInput{})
	if text != "" || s.CanShowCourses() {
		t.Fatal("unresolved name was allowed to finish")
	}
}
