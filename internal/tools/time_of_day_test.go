package tools

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestTimeOfDayBoundariesAndIntersection(t *testing.T) {
	for _, tc := range []struct {
		blocks         []string
		explicit, want []int
	}{
		{[]string{"morning"}, nil, []int{1, 2, 3, 4, 5}},
		{[]string{"afternoon"}, nil, []int{6, 7, 8, 9}},
		{[]string{"evening"}, nil, []int{10, 11, 12, 13}},
		{[]string{"morning", "evening"}, []int{5, 6, 9, 10, 13, 14}, []int{5, 10, 13}},
	} {
		got, err := sectionsForTimeOfDay(tc.blocks, tc.explicit)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("unexpected periods: %v %v", got, err)
		}
	}
	for _, tc := range []struct {
		blocks   []string
		explicit []int
	}{{[]string{"afternoon"}, []int{10}}, {[]string{"night"}, nil}} {
		if _, err := sectionsForTimeOfDay(tc.blocks, tc.explicit); err == nil {
			t.Fatal("invalid or contradictory time silently broadened")
		}
	}
}

func TestOfferingTimeFilterDoesNotReadTimetable(t *testing.T) {
	s := campusFixture(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/config"):
			return campusResponse(200, campusConfigJSON), nil
		case strings.HasSuffix(r.URL.Path, "/map"):
			return campusResponse(200, campusMapJSON), nil
		case strings.HasSuffix(r.URL.Path, "/schedule"):
			t.Fatal("offering-only request read private timetable")
		}
		return campusResponse(200, catalogJSON(catalogClass("private-class-a", "001", "影视音乐赏析", "星期二第8-9节{1-17周}"), catalogClass("private-class-b", "001", "影视音乐赏析", "星期二第10-11节{1-17周}"))), nil
	})
	_, _ = s.CheckOfferings(context.Background(), OfferingInput{Courses: []string{"影视音乐赏析"}})
	text, err := s.ShowCourses(context.Background(), ShowCoursesInput{TimeOfDay: []string{"evening"}})
	if err != nil || !strings.Contains(text, "符合指定时间；未核对本人课表") || strings.Contains(text, "private-class") {
		t.Fatal("offering filter failed or exposed class IDs")
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "第8-9节") && !strings.Contains(line, "不符合指定时间偏好") {
			t.Fatal("afternoon admitted into evening")
		}
	}
}

func TestShowCanReplacePreviouslyAppliedTimeBlock(t *testing.T) {
	s := campusFixture(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/config"):
			return campusResponse(200, campusConfigJSON), nil
		case strings.HasSuffix(r.URL.Path, "/map"):
			return campusResponse(200, campusMapJSON), nil
		case strings.HasSuffix(r.URL.Path, "/schedule"):
			return campusResponse(200, `{"code":0,"data":[],"pagination":{"total":0,"limit":200,"offset":0,"hasMore":false}}`), nil
		}
		return campusResponse(200, catalogJSON(catalogClass("afternoon", "001", "影视音乐赏析", "星期二第8-9节{1-17周}"), catalogClass("evening", "001", "影视音乐赏析", "星期二第10-11节{1-17周}"))), nil
	})
	_, _ = s.FitCourses(context.Background(), FitCoursesInput{Courses: []string{"影视音乐赏析"}, TimeOfDay: []string{"afternoon"}})
	text, err := s.ShowCourses(context.Background(), ShowCoursesInput{MatchSchedule: true, TimeOfDay: []string{"evening"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "|") && strings.Contains(line, "第8-9节") && !strings.Contains(line, "不符合指定时间偏好") {
			t.Fatal("show retained an outdated afternoon constraint")
		}
		if strings.HasPrefix(line, "|") && strings.Contains(line, "第10-11节") && !strings.Contains(line, "可放入空闲位置") {
			t.Fatal("show failed to apply the evening constraint")
		}
	}
}
