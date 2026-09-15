package tools

import (
	"fmt"
	"reflect"
	"testing"
)

func planFit(course, class, time string) CourseFit {
	times, complete := parseCourseTimes(time)
	return CourseFit{Status: "fits", Offering: Offering{CourseID: course, ClassID: class, Times: times, TimeComplete: complete}}
}

func TestBestCoursePlanChangesEarlierClassToFitAllThree(t *testing.T) {
	fits := []CourseFit{
		planFit("film", "film-first", "星期二第8-9节{1-17周}"),
		planFit("film", "film-later", "星期二第6-7节{1-17周}"),
		planFit("game", "game", "星期二第8-9节{1-17周}"),
		planFit("writing", "writing", "星期四第6-7节{1-17周}"),
	}
	got, complete := bestCoursePlan(fits, 12)
	if !complete || !reflect.DeepEqual(got, []string{"film-later", "game", "writing"}) {
		t.Fatalf("got %v complete=%v; want all three courses", got, complete)
	}
	// A caller asking for only two courses should retain the original order.
	got, complete = bestCoursePlan(fits, 2)
	if !complete || !reflect.DeepEqual(got, []string{"film-first", "writing"}) {
		t.Fatalf("unstable tie: %v complete=%v", got, complete)
	}
}

func TestBestCoursePlanUsesEveryMeetingAndWeek(t *testing.T) {
	fits := []CourseFit{
		planFit("a", "a", "星期三第6-7节{1-9周};星期四第6-7节{1-17周}"),
		planFit("b", "b", "星期三第6-7节{10-17周}"),
		planFit("c", "c", "星期四第7-8节{3-5周}"),
	}
	got, complete := bestCoursePlan(fits, 12)
	if !complete || !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("got %v complete=%v", got, complete)
	}
}

func TestBestCoursePlanCanOmitAnEarlierCourseToFitMore(t *testing.T) {
	fits := []CourseFit{
		planFit("a", "a", "星期一第1-2节{1-17周};星期二第1-2节{1-17周}"),
		planFit("b", "b", "星期一第1-2节{1-17周}"),
		planFit("c", "c", "星期二第1-2节{1-17周}"),
		planFit("d", "d", "星期三第1-2节{1-17周}"),
	}
	got, complete := bestCoursePlan(fits, 12)
	if !complete || !reflect.DeepEqual(got, []string{"b", "c", "d"}) {
		t.Fatalf("got %v complete=%v; want three courses after skipping a", got, complete)
	}
}

func TestBestCoursePlanExcludesUnverifiedAndDuplicateClasses(t *testing.T) {
	a := planFit("a", "a", "星期一第1-2节{1-17周}")
	b := planFit("b", "b", "星期二第1-2节{1-17周}")
	b.Status = "unknown"
	c := planFit("c", "c", "星期三第1-2节{1-17周};待定")
	got, complete := bestCoursePlan([]CourseFit{a, a, b, c}, 12)
	if !complete || !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("got %v complete=%v", got, complete)
	}
}

func TestBestCoursePlanReportsBoundAndKeepsFeasiblePartialResult(t *testing.T) {
	fits := []CourseFit{
		planFit("a", "a", "星期一第1-2节{1-17周}"),
		planFit("b", "b", "星期二第1-2节{1-17周}"),
	}
	got, complete := coursePlanWithBudget(fits, 12, 1)
	if complete || !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("search limit not surfaced: %v complete=%v", got, complete)
	}
	// A thirteenth eligible course must not silently become a proven optimum.
	fits = nil
	for i := 1; i <= 13; i++ {
		fits = append(fits, planFit(fmt.Sprint(i), fmt.Sprint(i), fmt.Sprintf("星期一第1节{%d周}", i)))
	}
	got, complete = bestCoursePlan(fits, 12)
	if complete || len(got) != 12 {
		t.Fatalf("course limit not surfaced: %v complete=%v", got, complete)
	}
}

func TestBestCoursePlanEmpty(t *testing.T) {
	got, complete := bestCoursePlan(nil, 12)
	if !complete || len(got) != 0 {
		t.Fatalf("got %v complete=%v", got, complete)
	}
}
