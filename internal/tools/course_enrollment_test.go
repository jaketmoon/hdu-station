package tools

import (
	"context"
	"net/http"
	"testing"
)

func TestScheduleClassIDCompletesCourseIdentityBeforeAllFits(t *testing.T) {
	for _, query := range []string{"影视音乐赏析", "影视音乐"} {
		t.Run(query, func(t *testing.T) {
			s := campusFixture(t, func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/hduhelp-neo/academic/config":
					return campusResponse(200, campusConfigJSON), nil
				case "/hduhelp-neo/academic/class/map":
					return campusResponse(200, campusMapJSON), nil
				case "/hduhelp-neo/academic/schedule":
					// No courseId or courseCode: identity must come from the class.
					return campusResponse(200, `{"code":0,"data":[{"schoolYear":"2026-2027","semester":1,"classId":"z-selected","classTime":"星期三第6-7节{1-9周}"}],"pagination":{"total":1,"limit":200,"offset":0,"hasMore":false}}`), nil
				case "/hduhelp-neo/academic/class/search":
					if r.URL.Query().Get("schoolYear") != "2026-2027" || r.URL.Query().Get("semester") != "1" {
						t.Error("course association crossed semesters")
					}
					if r.URL.Query().Get("query") == "书法鉴赏" {
						return campusResponse(200, catalogJSON(catalogClass("different-course", "002", "书法鉴赏", "星期一第6-7节{1-17周}"))), nil
					}
					// Keep the selected class last, after an otherwise free class.
					return campusResponse(200, catalogJSON(
						catalogClass("a-alternative", "001", "影视音乐赏析", "星期二第6-7节{1-17周}"),
						catalogClass("z-selected", "001", "影视音乐赏析", "星期三第6-7节{1-9周}"))), nil
				default:
					t.Error("unexpected request")
					return campusResponse(404, ""), nil
				}
			})
			result, err := s.FitCourses(context.Background(), FitCoursesInput{Courses: []string{query, "书法鉴赏"}})
			if err != nil || !result.ScheduleComplete {
				t.Fatalf("fixture schedule did not complete: %v", err)
			}
			allFits := append(append([]CourseFit{}, result.Fits...), result.CandidateFits...)
			seen := map[string]string{}
			for _, f := range allFits {
				seen[f.Offering.ClassID] = f.Status
			}
			if seen["a-alternative"] != "already_enrolled" || seen["z-selected"] != "already_enrolled" || seen["different-course"] != "fits" {
				t.Fatalf("class-only enrollment was not propagated to all matching course numbers: %v", seen)
			}
			if len(result.SuggestedPlan) != 1 || result.SuggestedPlan[0] != "different-course" {
				t.Fatal("another class of the selected course entered the plan")
			}
			if query == "影视音乐" && len(result.CandidateFits) != 2 {
				t.Fatal("fuzzy candidate path was not exercised")
			}
		})
	}
}
