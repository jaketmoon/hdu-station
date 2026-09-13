package tools

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestVerifyCurrentXiashaAndPrivateClassIDs(t *testing.T) {
	s := campusFixture(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/config"):
			return campusResponse(200, campusConfigJSON), nil
		case strings.HasSuffix(r.URL.Path, "/map"):
			return campusResponse(200, campusMapJSON), nil
		default:
			if r.URL.Query().Get("schoolYear") != "2026-2027" {
				t.Error("wrong term")
			}
			x := catalogClass("private-xiasha", "001", "影视音乐赏析", "星期二第6-7节{1-17周}")
			y := catalogClass("private-other", "001", "影视音乐赏析", "星期三第6-7节{1-17周}")
			y["campusID"] = "unknown-campus"
			return campusResponse(200, catalogJSON(x, y)), nil
		}
	})
	got, err := s.VerifyCourses(context.Background(), VerifyCoursesInput{Courses: []string{"影视音乐鉴赏", "001"}}, VerifyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "影视音乐赏析") || !strings.Contains(got, "星期二第6-7节") || strings.Contains(got, "星期三") || strings.Contains(got, "private-") || strings.Contains(got, "班级号") {
		t.Fatalf("scope/display violated: %s", got)
	}
}

func TestAmbiguousCourseNamesRemainCandidates(t *testing.T) {
	q := resolveCourseNames(OfferingQuery{Query: "艺术", Candidates: []CourseCandidate{{CourseID: "1", CourseName: "书法艺术", Classes: []Offering{{ClassID: "a"}}}, {CourseID: "2", CourseName: "影视艺术", Classes: []Offering{{ClassID: "b"}}}}})
	if len(q.Classes) != 0 || !q.NeedsSelection || len(q.Candidates) != 2 {
		t.Fatal("ambiguous name guessed")
	}
}

func TestCourseLabelsAreNotTreatedAsLiteralNames(t *testing.T) {
	for _, label := range []string{"影视音乐赏析 C5692013", "影视音乐赏析（C5692013）", "影视音乐赏析 / C5692013"} {
		if got := cleanCourseQuery(label); got != "影视音乐赏析" {
			t.Fatalf("%s => %s", label, got)
		}
	}
	if cleanCourseQuery("C5692013") != "C5692013" || cleanCourseQuery("形势与政策1") != "形势与政策1" {
		t.Fatal("course identity damaged")
	}
}

func TestCommunityProseCannotConfirmCurrentTeachingRequirements(t *testing.T) {
	s := NewCampusSession(nil, nil)
	for _, summary := range []string{"刘娟娟老师作业少", "徐艳只在另一门课的线索中出现", "柳丽娟、刘恩茂给分高", ""} {
		got := s.CommunityCaveat(summary)
		if !strings.Contains(got, "尚未逐课程、逐教师核实") || !strings.Contains(got, "不代表这些体验条件已经满足") {
			t.Fatal("prose treated as verified teaching requirements")
		}
	}
}

func TestTermFailureKeepsActionableReasonInFinalDisplay(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		status     int
	}{
		{"network", "校园教务暂时无法连接", 0},
		{"server", "服务处理请求失败", 500},
		{"invalid response", "数据格式无效", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := campusFixture(t, func(*http.Request) (*http.Response, error) {
				if tc.status == 0 {
					return nil, errors.Join(context.DeadlineExceeded, errors.New("private-upstream-sentinel"))
				}
				return campusResponse(tc.status, "private-upstream-sentinel"), nil
			})
			got, err := s.VerifyCourses(context.Background(), VerifyCoursesInput{Courses: []string{"戏曲鉴赏"}}, VerifyOptions{MatchSchedule: true})
			if err != nil || !strings.Contains(got, tc.want) || strings.Contains(got, "private-") || strings.Contains(got, "suggestedPlan") || strings.Contains(got, "fits") {
				t.Fatalf("lost safe error: %s %v", got, err)
			}
		})
	}
}
