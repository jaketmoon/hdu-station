package tools

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestScheduleAloneCanFinishAndLaterFitKeepsCandidates(t *testing.T) {
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
	if !strings.Contains(text, "空课表") || !s.CanShowCourses() {
		t.Fatal("schedule-only request was forced into recommendations")
	}
	if strings.Contains(s.Display(), "suggestedPlan") {
		t.Fatal("internal instructions leaked")
	}
	_, _ = s.CheckOfferings(ctx, OfferingInput{Courses: []string{"戏曲鉴赏"}})
	_, _ = s.FitCourses(ctx, FitCoursesInput{})
	text, _ = s.ShowCourses(ctx, ShowCoursesInput{})
	if !strings.Contains(text, "戏曲鉴赏") || !strings.Contains(text, "不符合指定时间偏好") || strings.Contains(text, "一组可以同时放入") {
		t.Fatal("schedule reread lost candidates or explicit Friday preference")
	}
}

func TestUnresolvedNamesCanBeShownWithoutPretendingTheyAreVerified(t *testing.T) {
	s := NewCampusSession(nil, nil)
	s.lastOfferings = &OfferingResult{Queries: []OfferingQuery{{Query: "影视音乐", NeedsSelection: true}}}
	text, _ := s.ShowCourses(context.Background(), ShowCoursesInput{})
	if !strings.Contains(text, "尚未匹配") || !s.CanShowCourses() {
		t.Fatal("candidate-only answer was blocked or falsely confirmed")
	}
}

func TestQueryStateDistinguishesRejectedShowFromFailedQuery(t *testing.T) {
	s := NewCampusSession(nil, nil)
	_, _ = s.ShowCourses(context.Background(), ShowCoursesInput{})
	if s.HasQueryResult() {
		t.Fatal("display-only call invented query state")
	}
	_, _ = s.CheckOfferings(context.Background(), OfferingInput{})
	if !s.HasQueryResult() || !strings.Contains(s.Display(), "1–12") {
		t.Fatal("failed query response lost its diagnostic display")
	}
}

func TestFocusedTimetableMergesWeeksAndShowsBothBusyAndFree(t *testing.T) {
	s := NewCampusSession(nil, nil)
	times, _ := parseCourseTimes("星期一第6-9节{1-17周};星期二第8-9节{1-7周,11-17周};星期二第8-9节{8-10周}")
	s.lastFit = &FitCoursesResult{Term: AcademicTerm{"2026-2027", 1}, ScheduleComplete: true, BusyTimes: times}
	s.scheduleRequested = true
	text, err := s.ShowCourses(context.Background(), ShowCoursesInput{AllowedDays: []int{2}, TimeOfDay: []string{"afternoon"}})
	if err != nil || !strings.Contains(text, "周二第6–7节 | 未发现占用 | 空闲") || !strings.Contains(text, "周二第8–9节 | 第1–17周有课") || strings.Contains(text, "周一") || strings.Contains(text, "其余时间未发现已选课程占用") {
		t.Fatalf("focused schedule lost busy/free scope: %s", text)
	}
	if s.requests != 0 {
		t.Fatal("displaying a focus reread campus or discovered courses")
	}
	s.lastFit.ScheduleComplete = false
	text = s.Display()
	if strings.Contains(text, "空闲（以本次课表为准）") || !strings.Contains(text, "暂不能确认空闲") {
		t.Fatal("incomplete timetable approved free slots")
	}
	s.lastFit.ScheduleComplete = true
	text, err = s.ShowCourses(context.Background(), ShowCoursesInput{TimeOfDay: []string{"evening"}})
	if err != nil || !strings.Contains(text, "周二第10–13节") || strings.Contains(text, "| 周二第6") {
		t.Fatal("new display focus did not replace old periods")
	}
}

func TestUnfilteredTimetableStillDisplaysWholeWeek(t *testing.T) {
	s := NewCampusSession(nil, nil)
	times, _ := parseCourseTimes("星期一第6-9节{1-17周};星期二第8-9节{1-17周}")
	s.lastFit = &FitCoursesResult{ScheduleComplete: true, BusyTimes: times}
	text := s.Display()
	if !strings.Contains(text, "周一") || !strings.Contains(text, "周二") || strings.Contains(text, "| 时间 |") {
		t.Fatal("unrequested timetable focus imposed")
	}
}
