package tools

import (
	"reflect"
	"testing"
)

func TestCourseTimesParseAllMeetingsAndFailClosed(t *testing.T) {
	for _, tc := range []struct {
		raw      string
		complete bool
		count    int
	}{
		{"星期三第6-7节{3-7周};星期三第6-7节{1-2周,8-9周}", true, 2},
		{"星期一第1-2节{1-17周(单)}；星期二第3,4节{2-16周（双）}", true, 2},
		{"星期五第8-9节{1-17周};待定", false, 1},
		{"", false, 0}, {"网上自主学习", false, 0},
		{"星期一第1-2节{0-18周}", false, 0}, {"星期一第1-2节{17-3周}", false, 0},
		{"星期一第15节{1-17周}", false, 0}, {"星期一第1-2节{1-999999周}", false, 0},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			slots, complete := parseCourseTimes(tc.raw)
			if complete != tc.complete || len(slots) != tc.count {
				t.Fatalf("unexpected parse completeness: %v, count %d", complete, len(slots))
			}
		})
	}
	slots, _ := parseCourseTimes("星期三第6-7节{1-2周,8-9周}")
	if !reflect.DeepEqual(slots[0].Weeks, []int{1, 2, 8, 9}) {
		t.Fatal("non-contiguous weeks lost")
	}
}

func TestFitUsesAllWeeksPreferencesAndUnknownCoverage(t *testing.T) {
	slots, _ := parseCourseTimes("星期一第1-2节{1-17周(单)}")
	schedule := scheduleResult{times: slots, complete: true, classes: map[string]bool{"enrolled": true}, courses: map[string]bool{}}
	for _, tc := range []struct {
		raw, status    string
		complete       bool
		days, sections []int
	}{
		{"星期一第1-2节{2-16周(双)}", "fits", true, nil, nil},
		{"星期一第1-2节{1-17周}", "conflict", true, nil, nil},
		{"星期二第1-2节{1-17周}", "fits", true, nil, nil},
		{"星期二第1-2节{1-17周}", "unknown", false, nil, nil},
		{"星期二第1-2节{1-17周};待定", "unknown", true, nil, nil},
		{"星期五第6-7节{1-17周}", "outside_preferences", true, []int{1, 2, 3, 4}, nil},
		{"星期二第1-2节{1-17周}", "outside_preferences", true, nil, []int{1}},
	} {
		o := Offering{ClassID: "candidate", CourseID: "001"}
		o.Times, o.TimeComplete = parseCourseTimes(tc.raw)
		schedule.complete = tc.complete
		fit := fitCourse(o, schedule, FitCoursesInput{AllowedDays: tc.days, AllowedSections: tc.sections})
		if fit.Status != tc.status {
			t.Fatalf("%s: got %s want %s", tc.raw, fit.Status, tc.status)
		}
	}
	if fit := fitCourse(Offering{ClassID: "enrolled"}, schedule, FitCoursesInput{}); fit.Status != "already_enrolled" {
		t.Fatal("enrolled class recommended")
	}
}
