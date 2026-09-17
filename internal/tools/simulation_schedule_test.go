package tools

import (
	"context"
	"strings"
	"testing"
)

func TestFillGapsUsesEffectiveSimulationCourses(t *testing.T) {
	s, f := newSimulationFixture(t)
	// The real course is dropped; a simulated course instead occupies Tuesday.
	f.data.SimulationItems = []SimulationItem{{"actual", "DROP"}, {"sim", "ENROLL"}}
	f.data.EffectiveCourses = []SimulationCourse{{ClassID: "sim", CourseID: "sim-course", ScheduleKnown: true, Slots: []SimulationSlot{{1, 2, 2}}}}
	r, err := s.FitCourses(context.Background(), FitCoursesInput{})
	if err != nil || !r.ScheduleComplete || r.ScheduleSource != "simulation" || r.SimulationRevision != 1 {
		t.Fatalf("wrong source: %+v %v", r, err)
	}
	if len(r.Fits) != 2 || r.Fits[0].Status != "conflict" || r.Fits[1].Status != "fits" {
		t.Fatalf("simulation add/drop ignored: %+v", r.Fits)
	}
	text := s.Display()
	if !strings.Contains(text, "排课依据：Neo 模拟课表") || strings.Contains(text, "与已选课程冲突") {
		t.Fatal("wrong source in visible results")
	}
	// A later read must see external changes, not a same-turn cached timetable.
	f.data.EffectiveCourses = []SimulationCourse{}
	f.data.SimulationItems = []SimulationItem{{"actual", "DROP"}}
	f.data.Revision++
	r, err = s.FitCourses(context.Background(), FitCoursesInput{})
	if err != nil || !r.ScheduleEmpty || r.Fits[0].Status != "fits" || r.SimulationRevision != 2 {
		t.Fatal("stale simulated occupancy reused")
	}
}
func TestIncompleteSimulationDoesNotFallBackToActual(t *testing.T) {
	for _, mode := range []string{"read_failure", "unknown_time", "bad_slot"} {
		t.Run(mode, func(t *testing.T) {
			s, f := newSimulationFixture(t)
			switch mode {
			case "read_failure":
				f.badRead = true
			case "unknown_time":
				f.data.EffectiveCourses[0].ScheduleKnown = false
			case "bad_slot":
				f.data.EffectiveCourses[0].Slots = []SimulationSlot{{1, 8, 2}}
			}
			r, err := s.FitCourses(context.Background(), FitCoursesInput{})
			if err != nil || r.ScheduleComplete || len(r.SuggestedPlan) != 0 || r.Fits[0].Status != "unknown" {
				t.Fatalf("unverified simulation treated as free: %+v", r)
			}
		})
	}
}
func TestSimulationManagementInvalidatesFitBeforeDisplay(t *testing.T) {
	s, f := newSimulationFixture(t)
	r, _ := s.FitCourses(context.Background(), FitCoursesInput{})
	if r.Fits[1].Status != "conflict" {
		t.Fatal("fixture missing real occupancy")
	}
	f.data.EffectiveCourses = []SimulationCourse{}
	f.data.SimulationItems = []SimulationItem{{"actual", "DROP"}}
	f.data.Revision++
	_, _ = s.ManageSimulation(context.Background(), SimulationInput{Action: "read"})
	if s.lastFit != nil {
		t.Fatal("old fit not invalidated")
	}
	_, err := s.ShowCourses(context.Background(), ShowCoursesInput{})
	if err != nil || s.lastFit == nil || s.lastFit.Fits[1].Status != "fits" {
		t.Fatal("display did not recalculate updated simulation")
	}
}
