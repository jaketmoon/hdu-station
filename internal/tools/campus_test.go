package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/jaketmoon/hdu-station/internal/campusauth"
)

type campusGrant struct{ schedule bool }

func (g campusGrant) AccessTokenFor(_ context.Context, scope string) (string, error) {
	if scope == campusauth.ScheduleScope && !g.schedule {
		return "", campusauth.ErrScope
	}
	return "pat-private-sentinel", nil
}

type campusTransport func(*http.Request) (*http.Response, error)

func (f campusTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func campusResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func campusFixture(t *testing.T, handler campusTransport) *CampusSession {
	t.Helper()
	c := NewCampusClient(campusGrant{schedule: true})
	c.http.Transport = campusTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Host != "api.hduhelp.com" || r.URL.Scheme != "https" || r.Header.Get("X-Staff-Id") != "" {
			t.Error("escaped fixed read-only campus routes")
		}
		if !strings.HasSuffix(r.URL.Path, "/config") && r.Header.Get("Authorization") != "Bearer pat-private-sentinel" {
			t.Error("missing private header")
		}
		return handler(r)
	})
	s := NewCampusSession(c, nil)
	// These legacy pagination fixtures explicitly exercise the real timetable.
	s.fitPreferences.ScheduleSource = "actual"
	return s
}

const campusConfigJSON = `{"code":0,"data":{"courseQueryDefault":{"schoolYear":"2026-2027","semester":"1"},"scheduleDefault":{"schoolYear":"2026-2027","semester":1}}}`
const campusMapJSON = `{"code":0,"data":{"campusID":{"1":"下沙"},"examinationMethod":{"2":"考查"}}}`

func catalogClass(id, course, name, slot string) map[string]any {
	return map[string]any{"classID": id, "courseID": course, "courseName": name, "classTime": slot, "teacherName": "测试老师", "credit": 2, "campusID": "1", "examinationMethod": "2", "classList": "student-private-sentinel"}
}
func catalogJSON(rows ...map[string]any) string {
	if rows == nil {
		rows = []map[string]any{}
	}
	b, _ := json.Marshal(map[string]any{"code": 0, "data": map[string]any{"classes": rows}})
	return string(b)
}

func TestLayeredLookupGroupsCourseIDsAndModelCanSelectSeveral(t *testing.T) {
	var mu sync.Mutex
	words := []string{}
	s := campusFixture(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/hduhelp-neo/academic/config":
			return campusResponse(200, campusConfigJSON), nil
		case "/hduhelp-neo/academic/class/map":
			return campusResponse(200, campusMapJSON), nil
		case "/hduhelp-neo/academic/class/search":
			q := r.URL.Query()
			if q.Get("schoolYear") != "2026-2027" || q.Get("semester") != "1" {
				t.Error("lookup changed semester")
			}
			mu.Lock()
			words = append(words, q.Get("query"))
			mu.Unlock()
			if q.Get("query") == "影视音乐鉴赏" {
				return campusResponse(200, catalogJSON()), nil
			}
			return campusResponse(200, catalogJSON(
				catalogClass("a", "001", "影视音乐赏析", "星期二第1-2节{1-17周}"),
				catalogClass("b", "001", "影视音乐赏析", "星期三第1-2节{1-17周}"),
				catalogClass("c", "002", "影视音乐欣赏", "星期四第1-2节{1-17周}"))), nil
		default:
			t.Error("unexpected route")
			return campusResponse(404, ""), nil
		}
	})
	in := OfferingInput{Courses: []string{"影视音乐鉴赏"}, CoreKeywords: []CourseKeyword{{Course: "影视音乐鉴赏", Keyword: "影视音乐"}}}
	result, err := s.CheckOfferings(context.Background(), in)
	if err != nil || len(result.Queries) != 1 {
		t.Fatal("lookup failed")
	}
	q := result.Queries[0]
	if !q.NeedsSelection {
		t.Fatal("fuzzy match did not require model selection")
	}
	if !reflect.DeepEqual(words, []string{"影视音乐鉴赏", "影视音乐"}) || len(q.Classes) != 0 || len(q.Candidates) != 2 || len(q.Candidates[0].Classes) != 2 || q.Candidates[0].CourseID != "001" {
		t.Fatal("layered lookup or course grouping lost identity")
	}
	in.CourseIDs = []string{"001", "002"}
	selected, _ := s.CheckOfferings(context.Background(), in)
	if selected.Queries[0].NeedsSelection {
		t.Fatal("confirmed IDs left a stale selection requirement")
	}
	if len(selected.Queries[0].Classes) != 3 || len(words) != 2 {
		t.Fatal("model selections did not reuse verified course IDs")
	}
	if selected.Queries[0].Classes[0].Campus != "下沙" || selected.Queries[0].Classes[0].Examination != "考查" {
		t.Fatal("dictionary not applied")
	}
	in.CourseIDs = []string{"1"}
	notFound, _ := s.CheckOfferings(context.Background(), in)
	if len(notFound.Queries[0].Classes) != 0 {
		t.Fatal("leading zero course ID inferred")
	}
	public, _ := json.Marshal(selected)
	if strings.Contains(string(public), "private-sentinel") || strings.Contains(string(public), "classList") {
		t.Fatal("private fields reached model")
	}
	// A model may continue with the official name instead of the original
	// internet name. Keep the previously observed candidate mapping visible.
	_, _ = s.CheckOfferings(context.Background(), OfferingInput{Courses: []string{"影视音乐赏析"}, CourseIDs: []string{"001"}})
	if !strings.Contains(s.Display(), "影视音乐鉴赏 → 影视音乐赏析（001，名称相近，可能对应）") {
		t.Fatal("original fuzzy name lost after official-name selection")
	}
}

func TestExactLookupStopsBeforeCoreAndNeverTreatsEmptyAsNotOffered(t *testing.T) {
	searches := 0
	s := campusFixture(t, func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/map") {
			return campusResponse(200, campusMapJSON), nil
		}
		searches++
		if r.URL.Query().Get("query") != "戏曲鉴赏" {
			t.Error("unnecessary core fallback")
		}
		return campusResponse(200, catalogJSON(catalogClass("a", "001", "戏曲鉴赏", "星期二第1-2节{1-17周}"))), nil
	})
	r, _ := s.CheckOfferings(context.Background(), OfferingInput{Courses: []string{"戏曲鉴赏"}, SchoolYear: "2025-2026", Semester: 2})
	if searches != 1 || len(r.Queries[0].Classes) != 1 || !r.Queries[0].Partial || r.Term.SchoolYear != "2025-2026" {
		t.Fatal("exact query or semester isolation failed")
	}
}

func TestFitReadsAllPagesAndBuildsMutuallyCompatiblePlan(t *testing.T) {
	offsets := []string{}
	s := campusFixture(t, func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/hduhelp-neo/academic/config":
			return campusResponse(200, campusConfigJSON), nil
		case "/hduhelp-neo/academic/class/map":
			return campusResponse(200, campusMapJSON), nil
		case "/hduhelp-neo/academic/schedule":
			o := r.URL.Query().Get("offset")
			offsets = append(offsets, o)
			if o == "0" {
				return campusResponse(200, `{"code":0,"data":[{"schoolYear":"2026-2027","semester":1,"classId":"own-a","courseId":"own-course","classTime":"星期一第1-2节{1-17周(单)}","staffName":"private-sentinel"}],"pagination":{"total":2,"limit":200,"offset":0,"hasMore":true,"nextOffset":1}}`), nil
			}
			return campusResponse(200, `{"code":0,"data":[{"schoolYear":"2026-2027","semester":1,"classId":"own-b","classTime":"星期三第3-4节{1-17周}"}],"pagination":{"total":2,"limit":200,"offset":1,"hasMore":false}}`), nil
		case "/hduhelp-neo/academic/class/search":
			name := r.URL.Query().Get("query")
			return campusResponse(200, catalogJSON(catalogClass(name+"-free", name+"-course", name, "星期二第1-2节{1-17周}"), catalogClass(name+"-clash", name+"-course", name, "星期三第3-4节{1-17周}"))), nil
		default:
			t.Error("unexpected route")
			return campusResponse(404, ""), nil
		}
	})
	r, err := s.FitCourses(context.Background(), FitCoursesInput{Courses: []string{"戏曲鉴赏", "艺术导论"}})
	if err != nil || !r.ScheduleComplete || len(r.Fits) != 4 || len(r.SuggestedPlan) != 1 || !reflect.DeepEqual(offsets, []string{"0", "1"}) {
		t.Fatal("complete schedule or compatible plan failed")
	}
	if r.Fits[1].Status != "conflict" || r.Fits[0].Status != "fits" {
		t.Fatal("second-page conflict missed")
	}
	display := s.Display()
	for _, f := range r.Fits {
		found := false
		for _, line := range strings.Split(display, "\n") {
			if strings.HasPrefix(line, "|") && strings.Contains(line, f.Offering.CourseName) && strings.Contains(line, f.Offering.ClassTime) {
				found = true
				if !strings.Contains(line, f.Offering.ClassTime) {
					t.Fatal("official course paired with a different time")
				}
				if f.Status == "conflict" && !strings.Contains(line, "与已选课程冲突") {
					t.Fatal("computed conflict lost in display")
				}
			}
		}
		if !found {
			t.Fatal("checked class missing from display")
		}
	}
	if strings.Contains(display, "suggestedPlan") || strings.Contains(display, "private-sentinel") {
		t.Fatal("internal data leaked into final report")
	}
	public, _ := json.Marshal(r)
	if strings.Contains(string(public), "private-sentinel") || strings.Contains(string(public), "own-course") {
		t.Fatal("personal identity leaked")
	}
}

func TestIncompleteOrMismatchedScheduleNeverApprovesFreeSlot(t *testing.T) {
	for _, body := range []string{
		`{"code":0,"data":[]}`,
		`{"code":0,"data":[],"pagination":{"limit":200,"offset":0}}`,
		`{"code":0,"data":[],"pagination":{"total":1,"offset":0,"limit":200,"hasMore":false}}`,
		`{"code":0,"data":[{"schoolYear":"2026-2027","semester":1,"classTime":"待定"}],"pagination":{"total":1,"offset":0,"limit":200,"hasMore":false}}`,
		`{"code":0,"data":[{"schoolYear":"2025-2026","semester":1,"classTime":"星期一第1-2节{1-17周}"}],"pagination":{"total":1,"offset":0,"limit":200,"hasMore":false}}`,
	} {
		s := campusFixture(t, func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "/schedule") {
				return campusResponse(200, body), nil
			}
			if strings.HasSuffix(r.URL.Path, "/map") {
				return campusResponse(200, campusMapJSON), nil
			}
			return campusResponse(200, catalogJSON(catalogClass("a", "001", "戏曲鉴赏", "星期二第1-2节{1-17周}"))), nil
		})
		r, _ := s.FitCourses(context.Background(), FitCoursesInput{Courses: []string{"戏曲鉴赏"}, SchoolYear: "2026-2027", Semester: 1})
		if r.ScheduleComplete || len(r.Fits) != 1 || r.Fits[0].Status != "unknown" || len(r.SuggestedPlan) != 0 {
			t.Fatal("incomplete schedule approved free slot")
		}
	}
}

func TestCampusFailuresStayBoundedAndDoNotReflectSecrets(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500, 302} {
		s := campusFixture(t, func(*http.Request) (*http.Response, error) {
			r := campusResponse(status, `{"msg":"private-sentinel"}`)
			r.Header.Set("Location", "https://example.test")
			return r, nil
		})
		r, _ := s.CheckOfferings(context.Background(), OfferingInput{Courses: []string{"测试课程"}, SchoolYear: "2026-2027", Semester: 1})
		b, _ := json.Marshal(r)
		if strings.Contains(string(b), "private-sentinel") || len(r.Queries) != 1 || r.Queries[0].Warning == "" {
			t.Fatal("unsafe or missing campus error")
		}
		if s.requests > 2 {
			t.Fatal("retried auth/server failure using fuzzy fallback")
		}
	}
	s := campusFixture(t, func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.CheckOfferings(ctx, OfferingInput{Courses: []string{"测试课程"}, SchoolYear: "2026-2027", Semester: 1}); err != context.Canceled {
		t.Fatal("cancellation lost")
	}
	if err := s.client.http.CheckRedirect(nil, nil); err != http.ErrUseLastResponse {
		t.Fatal("redirect policy changed")
	}
	if _, err := s.client.get(context.Background(), "/academic/grade", nil, &struct{}{}); err == nil {
		t.Fatal("unregistered route accepted")
	}
}

func TestFitChoosesThreeCoursesAroundExistingTimetable(t *testing.T) {
	s := campusFixture(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/config"):
			return campusResponse(200, campusConfigJSON), nil
		case strings.HasSuffix(r.URL.Path, "/map"):
			return campusResponse(200, campusMapJSON), nil
		case strings.HasSuffix(r.URL.Path, "/schedule"):
			return campusResponse(200, `{"code":0,"data":[{"schoolYear":"2026-2027","semester":1,"classTime":"星期二第8-9节{1-17周}"}],"pagination":{"total":1,"limit":200,"offset":0,"hasMore":false}}`), nil
		default:
			name := r.URL.Query().Get("query")
			switch name {
			case "影视音乐赏析":
				return campusResponse(200, catalogJSON(catalogClass("film-busy", "film", name, "星期二第8-9节{1-17周}"), catalogClass("film-night", "film", name, "星期二第10-11节{1-17周}"), catalogClass("film-afternoon", "film", name, "星期二第6-7节{1-17周}"))), nil
			case "数字游戏设计与艺术赏析":
				return campusResponse(200, catalogJSON(catalogClass("game-tue", "game", name, "星期二第10-11节{1-17周}"), catalogClass("game-wed", "game", name, "星期三第10-11节{1-17周}"))), nil
			default:
				return campusResponse(200, catalogJSON(catalogClass("writing-tue", "writing", name, "星期二第10-11节{1-17周}"))), nil
			}
		}
	})
	r, err := s.FitCourses(context.Background(), FitCoursesInput{Courses: []string{"影视音乐赏析", "数字游戏设计与艺术赏析", "书法鉴赏"}})
	if err != nil || !r.ScheduleComplete || r.PlanIncomplete || !reflect.DeepEqual(r.SuggestedPlan, []string{"film-afternoon", "game-wed", "writing-tue"}) {
		t.Fatalf("missed compatible three-course plan: %v %v", r.SuggestedPlan, err)
	}
	if r.Fits[0].Status != "conflict" {
		t.Fatal("new plan changed existing-course conflict")
	}
	s.lastFit.PlanIncomplete = true
	text := s.Display()
	if !strings.Contains(text, "尚未确认是否还能安排更多") || strings.Contains(text, "film-afternoon") {
		t.Fatal("search bound not visible or internal class ID exposed")
	}
}

func TestOnlyConfirmedXiashaRowsEnterCandidatesAndPlans(t *testing.T) {
	s := campusFixture(t, func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/config"):
			return campusResponse(200, campusConfigJSON), nil
		case strings.HasSuffix(r.URL.Path, "/map"):
			return campusResponse(200, `{"code":0,"data":{"campusID":{"1":"下沙","2":"青山湖"},"examinationMethod":{"2":"考查"}}}`), nil
		case strings.HasSuffix(r.URL.Path, "/schedule"):
			return campusResponse(200, `{"code":0,"data":[],"pagination":{"total":0,"limit":200,"offset":0,"hasMore":false}}`), nil
		default:
			known := catalogClass("known", "001", "影视音乐赏析", "星期二第10-11节{1-17周}")
			other := catalogClass("other", "002", "影视音乐赏析", "星期三第10-11节{1-17周}")
			other["campusID"] = "2"
			unknown := catalogClass("unknown", "003", "影视音乐赏析", "星期四第10-11节{1-17周}")
			unknown["campusID"] = "campus-private-internal-code"
			return campusResponse(200, catalogJSON(known, other, unknown)), nil
		}
	})
	result, err := s.FitCourses(context.Background(), FitCoursesInput{Courses: []string{"影视音乐赏析"}})
	if err != nil || len(result.Fits) != 1 || len(result.SuggestedPlan) != 1 || result.Fits[0].Offering.Campus != "下沙" || !result.Offerings.Queries[0].CampusUnconfirmed {
		t.Fatal("non-Xiasha or unknown campus entered fits/plan")
	}
	text := s.Display()
	if strings.Contains(text, "campus-private-internal-code") || strings.Contains(text, "青山湖") || !strings.Contains(text, "校区未能确认") {
		t.Fatal("campus identifier leaked or missing-campus limitation hidden")
	}
}
