package tools

import (
	"context"
	"testing"
)

func replanCourse(id, course string, week, day, section int) SimulationCourse {
	return SimulationCourse{ClassID: id, CourseID: course, CourseName: course, ScheduleKnown: true, Slots: []SimulationSlot{{week, day, section}}}
}

func replanFixture(t *testing.T) (*CampusSession, *simulationFixture) {
	s, f := newSimulationFixture(t)
	real := replanCourse("actual", "real", 1, 1, 1)
	keep := replanCourse("keep", "fixed", 1, 2, 1)
	a1 := replanCourse("a1", "a", 1, 1, 1) // real conflict
	a2 := replanCourse("a2", "a", 1, 3, 1)
	a3 := replanCourse("a3", "a", 1, 4, 1)
	b := replanCourse("b", "b", 1, 3, 1) // forces alternate a3
	c := replanCourse("c", "c", 1, 2, 1) // unrelated simulated conflict
	unknown := replanCourse("unknown", "u", 1, 5, 1)
	unknown.ScheduleKnown = false
	f.data.ActualCourses = []SimulationCourse{real}
	f.data.EffectiveCourses = []SimulationCourse{keep, a1, a2, a3, b, c, unknown}
	// Real course hidden by a prior DROP must still occupy time for replanning.
	f.data.SimulationItems = []SimulationItem{{"actual", "DROP"}, {"keep", "ENROLL"}, {"a1", "ENROLL"}, {"a2", "ENROLL"}, {"a3", "ENROLL"}, {"b", "ENROLL"}, {"c", "ENROLL"}, {"unknown", "ENROLL"}}
	s.lastOfferings = nil // Existing enrollments require no re-verification.
	return s, f
}
func TestSimulationReplanPreservesFixedCoursesAndSavesOnce(t *testing.T) {
	s, f := replanFixture(t)
	r, err := s.PlanSimulation(context.Background(), SimulationReplanInput{ClassIDs: []string{"a1", "a2", "a3", "b", "c", "unknown"}})
	if err != nil || r.Status != "planned" || !sameIDs(r.SuggestedPlan, []string{"a3", "b"}) || f.puts != 0 {
		t.Fatalf("plan: %+v err=%v writes=%d", r, err, f.puts)
	}
	if !sameIDs(r.Update.RemoveClassIDs, []string{"a1", "a2", "c", "unknown"}) || !r.Update.RequireNoConflicts || r.PlanIncomplete {
		t.Fatalf("patch: %+v", r)
	}
	saved, err := s.ManageSimulation(context.Background(), *r.Update)
	if err != nil || saved.Status != "confirmed" || f.puts != 1 {
		t.Fatalf("save: %+v %v writes=%d", saved, err, f.puts)
	}
	if !sameSimulation(f.data.SimulationItems, []SimulationItem{{"actual", "DROP"}, {"keep", "ENROLL"}, {"a3", "ENROLL"}, {"b", "ENROLL"}}) {
		t.Fatalf("lost unrelated state: %+v", f.data.SimulationItems)
	}
}
func TestSimulationReplanRejectsInvalidScopeAndIncompleteFixedCourses(t *testing.T) {
	for _, ids := range [][]string{nil, {"actual"}, {"missing"}, {"a1", "a1"}} {
		s, f := replanFixture(t)
		r, err := s.PlanSimulation(context.Background(), SimulationReplanInput{ClassIDs: ids})
		if err != nil || r.Status != "blocked" || r.Update != nil || f.puts != 0 {
			t.Fatalf("invalid scope: %+v %v", r, err)
		}
	}
	s, f := replanFixture(t)
	f.data.ActualCourses[0].ScheduleKnown = false
	r, _ := s.PlanSimulation(context.Background(), SimulationReplanInput{ClassIDs: []string{"a1", "a2", "a3", "b", "c", "unknown"}})
	if r.Status != "blocked" || r.Update != nil {
		t.Fatalf("unknown fixed schedule accepted: %+v", r)
	}
}
func TestSimulationReplanRevisionChangeStopsSave(t *testing.T) {
	s, f := replanFixture(t)
	r, _ := s.PlanSimulation(context.Background(), SimulationReplanInput{ClassIDs: []string{"a1", "a2", "a3", "b", "c", "unknown"}})
	if r.Update == nil {
		t.Fatalf("missing plan: %+v", r)
	}
	f.data.Revision++
	saved, _ := s.ManageSimulation(context.Background(), *r.Update)
	if saved.Status != "conflict" || f.puts != 0 {
		t.Fatalf("stale plan saved: %+v", saved)
	}
}
func TestSimulationReplanDistinguishesWeeks(t *testing.T) {
	s, f := newSimulationFixture(t)
	a := replanCourse("a", "a", 2, 1, 1) // Same weekday/section as real, different week.
	f.data.EffectiveCourses = append(f.data.EffectiveCourses, a)
	f.data.SimulationItems = []SimulationItem{{"a", "ENROLL"}}
	r, _ := s.PlanSimulation(context.Background(), SimulationReplanInput{ClassIDs: []string{"a"}})
	if r.Status != "planned" || !sameIDs(r.SuggestedPlan, []string{"a"}) {
		t.Fatalf("disjoint weeks rejected: %+v", r)
	}
}

func TestSimulationReplanRejectsInvalidFixedWeek(t *testing.T) {
	s, f := replanFixture(t)
	f.data.ActualCourses[0].Slots[0].Week = 61
	r, _ := s.PlanSimulation(context.Background(), SimulationReplanInput{ClassIDs: []string{"a1", "a2", "a3", "b", "c", "unknown"}})
	if r.Status != "blocked" || r.Update != nil || f.puts != 0 {
		t.Fatalf("invalid fixed week accepted: %+v", r)
	}
}
