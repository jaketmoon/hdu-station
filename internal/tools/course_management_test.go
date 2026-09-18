package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

func TestCollectionCRUDAndRank(t *testing.T) {
	c := NewCampusClient(campusGrant{true})
	s := NewCampusSession(c, nil)
	stored := []string{"keep", "old"}
	writes := 0
	c.http.Transport = campusTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("X-Staff-Id") != "" {
			t.Fatal("account override")
		}
		switch r.URL.Path {
		case "/hduhelp-neo/academic/class/fav", "/hduhelp-neo/academic/class/fav/rank":
			if r.Method == "POST" {
				writes++
				var b struct {
					Classes []string `json:"classes"`
				}
				json.NewDecoder(r.Body).Decode(&b)
				if b.Classes == nil {
					t.Fatal("nil replaces list")
				}
				stored = b.Classes
				return campusResponse(200, `{"code":0}`), nil
			}
			rows := []FavoriteEntry{}
			for _, id := range stored {
				rows = append(rows, FavoriteEntry{ClassID: id, FavCount: 2})
			}
			b, _ := json.Marshal(map[string]any{"code": 0, "data": rows})
			return campusResponse(200, string(b)), nil
		case "/hduhelp-neo/academic/course":
			rows := []map[string]any{}
			for _, id := range r.URL.Query()["id"] {
				rows = append(rows, catalogClass(id, id, "课程"+id, "星期一第1-2节{1-17周}"))
			}
			return campusResponse(200, catalogJSON(rows...)), nil
		default:
			t.Fatal("unexpected endpoint")
			return nil, nil
		}
	})
	call := func(in FavoriteInput, status string) {
		t.Helper()
		r, e := s.ManageFavorites(context.Background(), in)
		if e != nil || r.Status != status {
			t.Fatalf("%s: %s %s", in.Action, r.Status, r.Message)
		}
	}
	call(FavoriteInput{Action: "remove", ClassIDs: []string{"old"}}, "not_written")
	call(FavoriteInput{Action: "rank"}, "read")
	call(FavoriteInput{Action: "remove", ClassIDs: []string{"old"}}, "not_written") // rank is not a personal list
	call(FavoriteInput{Action: "read"}, "read")
	if s.favoriteKnown["keep"].CourseName != "课程keep" {
		t.Fatal("missing course details")
	}
	s.lastOfferings = &OfferingResult{Queries: []OfferingQuery{{Classes: []Offering{{ClassID: "new", CourseName: "新课"}}}}}
	call(FavoriteInput{Action: "update", ClassIDs: []string{"new"}, RemoveClassIDs: []string{"old"}}, "confirmed")
	if !reflect.DeepEqual(stored, []string{"keep", "new"}) {
		t.Fatal("swap lost unrelated item")
	}
	call(FavoriteInput{Action: "read"}, "read")
	call(FavoriteInput{Action: "remove", ClassIDs: []string{"new"}}, "confirmed")
	call(FavoriteInput{Action: "read"}, "read")
	call(FavoriteInput{Action: "replace", ClassIDs: []string{"new"}}, "confirmed")
	call(FavoriteInput{Action: "read"}, "read")
	stored = append(stored, "external")
	call(FavoriteInput{Action: "clear"}, "not_written")
	call(FavoriteInput{Action: "read"}, "read")
	call(FavoriteInput{Action: "clear"}, "confirmed")
	if len(stored) != 0 || writes != 4 {
		t.Fatal("CRUD writes incorrect")
	}
}

type simulationFixture struct {
	data                                  SimulationData
	puts                                  int
	conflict, lost, badRead, missingAfter bool
}

func newSimulationFixture(t *testing.T) (*CampusSession, *simulationFixture) {
	t.Helper()
	c := NewCampusClient(campusGrant{true})
	s := NewCampusSession(c, nil)
	f := &simulationFixture{data: SimulationData{SchemaVersion: 1, SchoolYear: "2026-2027", Semester: 1, Revision: 1, ActualCourses: []SimulationCourse{{ClassID: "actual", CourseName: "原课程", ScheduleKnown: true, Slots: []SimulationSlot{{1, 1, 1}}}}, SimulationItems: []SimulationItem{}, EffectiveCourses: []SimulationCourse{{ClassID: "actual", CourseName: "原课程", ScheduleKnown: true, Slots: []SimulationSlot{{1, 1, 1}}}}}}
	c.http.Transport = campusTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/hduhelp-neo/academic/config" {
			return campusResponse(200, campusConfigJSON), nil
		}
		if r.URL.Path != "/hduhelp-neo/academic/course-simulation" || r.Header.Get("X-Staff-Id") != "" {
			t.Fatal("escaped simulation endpoint")
		}
		if r.Method == "PUT" {
			f.puts++
			var b struct {
				SchoolYear       string           `json:"schoolYear"`
				Semester         int              `json:"semester"`
				ExpectedRevision int64            `json:"expectedRevision"`
				Items            []SimulationItem `json:"items"`
			}
			json.NewDecoder(r.Body).Decode(&b)
			if f.conflict {
				return campusResponse(409, `{"code":409}`), nil
			}
			if b.ExpectedRevision != f.data.Revision || b.Items == nil || b.SchoolYear != f.data.SchoolYear || b.Semester != f.data.Semester {
				t.Error("invalid revision, term or full items")
			}
			f.data.Revision++
			f.data.SimulationItems = b.Items
			f.data.EffectiveCourses = []SimulationCourse{}
			dropped := map[string]bool{}
			for _, i := range b.Items {
				if i.Intent == "DROP" {
					dropped[i.ClassID] = true
				} else {
					f.data.EffectiveCourses = append(f.data.EffectiveCourses, SimulationCourse{ClassID: i.ClassID, CourseName: "模拟课程", ScheduleKnown: true, Slots: []SimulationSlot{{1, 2, 2}}})
				}
			}
			for _, o := range f.data.ActualCourses {
				if !dropped[o.ClassID] {
					f.data.EffectiveCourses = append(f.data.EffectiveCourses, o)
				}
			}
			if f.lost {
				return campusResponse(500, `{}`), nil
			}
		}
		if f.badRead || f.missingAfter && f.puts > 0 {
			return campusResponse(500, `{}`), nil
		}
		b, _ := json.Marshal(map[string]any{"code": 0, "data": f.data})
		return campusResponse(200, string(b)), nil
	})
	s.lastOfferings = &OfferingResult{Term: AcademicTerm{"2026-2027", 1}, Queries: []OfferingQuery{{Classes: []Offering{{ClassID: "new", CourseName: "新增课", TimeComplete: true, Times: []CourseTime{{Weeks: []int{1}, Day: 2, Sections: []int{2}}}}, {ClassID: "clash", CourseName: "撞课", TimeComplete: true, Times: []CourseTime{{Weeks: []int{1}, Day: 1, Sections: []int{1}}}}}}}}
	return s, f
}
func TestSimulationCRUDPreservesOtherItems(t *testing.T) {
	s, f := newSimulationFixture(t)
	call := func(in SimulationInput, status string) {
		t.Helper()
		r, e := s.ManageSimulation(context.Background(), in)
		if e != nil || r.Status != status {
			t.Fatalf("%s: %s %s", in.Action, r.Status, r.Message)
		}
		if r.Data != nil {
			for _, o := range r.Data.ActualCourses {
				if len(o.Slots) > 0 {
					t.Fatal("expanded slots exposed to model")
				}
			}
		}
	}
	call(SimulationInput{Action: "update", Items: []SimulationItem{{"new", "ENROLL"}}}, "not_written")
	call(SimulationInput{Action: "read"}, "read")
	rev := f.data.Revision
	call(SimulationInput{Action: "update", ExpectedRevision: &rev, Items: []SimulationItem{{"new", "ENROLL"}}, RequireNoConflicts: true}, "confirmed")
	rev = f.data.Revision
	call(SimulationInput{Action: "update", ExpectedRevision: &rev, Items: []SimulationItem{{"actual", "DROP"}}}, "confirmed")
	if len(f.data.SimulationItems) != 2 {
		t.Fatal("patch erased enrollment")
	}
	rev = f.data.Revision
	call(SimulationInput{Action: "update", ExpectedRevision: &rev, RemoveClassIDs: []string{"new"}}, "confirmed")
	if !sameSimulation(f.data.SimulationItems, []SimulationItem{{"actual", "DROP"}}) {
		t.Fatal("remove erased other changes")
	}
	rev = f.data.Revision
	call(SimulationInput{Action: "update", ExpectedRevision: &rev, Items: []SimulationItem{{"actual", "ENROLL"}}}, "confirmed")
	if len(f.data.SimulationItems) != 0 {
		t.Fatal("restore left drop")
	}
	rev = f.data.Revision
	call(SimulationInput{Action: "replace", ExpectedRevision: &rev, Items: []SimulationItem{{"new", "ENROLL"}}}, "confirmed")
	rev = f.data.Revision
	call(SimulationInput{Action: "reset", ExpectedRevision: &rev}, "confirmed")
	if len(f.data.SimulationItems) != 0 || len(f.data.ActualCourses) != 1 {
		t.Fatal("reset changed actual courses")
	}
}
func TestSimulationWriteBoundaries(t *testing.T) {
	for _, mode := range []string{"stale", "race", "unknown_id", "drop_simulated", "time_conflict", "missing_time", "lost_response", "unconfirmed", "read_failed"} {
		t.Run(mode, func(t *testing.T) {
			s, f := newSimulationFixture(t)
			s.ManageSimulation(context.Background(), SimulationInput{Action: "read"})
			rev := f.data.Revision
			in := SimulationInput{Action: "update", ExpectedRevision: &rev, Items: []SimulationItem{{"new", "ENROLL"}}, RequireNoConflicts: true}
			want := "not_written"
			switch mode {
			case "stale":
				f.data.Revision++
				want = "conflict"
			case "race":
				f.conflict = true
				want = "conflict"
			case "unknown_id":
				in.Items[0].ClassID = "invented"
			case "drop_simulated":
				in.Items[0].Intent = "DROP"
			case "time_conflict":
				in.Items[0].ClassID = "clash"
			case "missing_time":
				s.lastOfferings.Queries[0].Classes[0].TimeComplete = false
			case "lost_response":
				f.lost = true
				want = "confirmed"
			case "unconfirmed":
				f.missingAfter = true
				want = "unknown"
			case "read_failed":
				f.badRead = true
			}
			r, e := s.ManageSimulation(context.Background(), in)
			if e != nil || r.Status != want {
				t.Fatalf("got %s %s", r.Status, r.Message)
			}
			if want == "not_written" && f.puts != 0 {
				t.Fatal("invalid mutation sent")
			}
			if want == "unknown" || want == "conflict" {
				puts := f.puts
				s.ManageSimulation(context.Background(), in)
				if f.puts != puts {
					t.Fatal("automatic retry")
				}
			}
		})
	}
}

func TestSimulationChecksModifiedScheduleRatherThanActualOnly(t *testing.T) {
	s, f := newSimulationFixture(t)
	target := map[string]string{"actual": "DROP", "clash": "ENROLL"}
	known := map[string]Offering{}
	for _, o := range s.lastOfferings.Queries[0].Classes {
		known[o.ClassID] = o
	}
	if !simulationFits(&f.data, target, known) {
		t.Fatal("simulated drop still occupies time")
	}
	target["actual"] = "ENROLL"
	if simulationFits(&f.data, target, known) {
		t.Fatal("restored actual collision ignored")
	}
}

func TestSimulationRejectsCrossTermAndDuplicateOperations(t *testing.T) {
	s, f := newSimulationFixture(t)
	s.ManageSimulation(context.Background(), SimulationInput{Action: "read"})
	rev := f.data.Revision
	s.lastOfferings.Term = AcademicTerm{"2025-2026", 1}
	r, _ := s.ManageSimulation(context.Background(), SimulationInput{Action: "update", ExpectedRevision: &rev, Items: []SimulationItem{{"new", "ENROLL"}}})
	if r.Status != "not_written" || f.puts != 0 {
		t.Fatal("cross-term enrollment accepted")
	}
	r, _ = s.ManageSimulation(context.Background(), SimulationInput{Action: "update", ExpectedRevision: &rev, Items: []SimulationItem{{"actual", "DROP"}, {"actual", "ENROLL"}}})
	if r.Status != "not_written" || f.puts != 0 {
		t.Fatal("ambiguous duplicate accepted")
	}
}
