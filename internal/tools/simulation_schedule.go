package tools

import (
	"context"
	"fmt"
	"github.com/jaketmoon/hdu-station/internal/campusauth"
	"sort"
)

// Always fetch the current effective snapshot. Never fall back to the real
// timetable: that would reintroduce dropped courses and omit simulated courses.
func (s *CampusSession) simulationSchedule(ctx context.Context, term AcademicTerm) (scheduleResult, int64) {
	r := scheduleResult{classes: map[string]bool{}, courses: map[string]bool{}, times: []CourseTime{}}
	s.progress("正在读取模拟课表并核对全部有效课程…")
	headers, err := s.managementHeaders(ctx, campusauth.SimulationReadScope, "")
	if err != nil {
		r.warning = "模拟课表读取授权不可用，请在设置中重新授权；未改用真实课表。"
		return r, 0
	}
	data, err := s.readSimulation(ctx, term, headers)
	if err != nil {
		r.warning = "模拟课表未能完整读取，无法确认空闲位置；未改用真实课表。"
		return r, 0
	}
	r.complete = true
	for _, o := range data.EffectiveCourses {
		r.classes[o.ClassID] = true
		if o.CourseID != "" {
			r.courses[o.CourseID] = true
		}
		if !o.ScheduleKnown || len(o.Slots) == 0 {
			r.complete = false
			r.warning = "部分模拟课表课程时间不完整，不能确认空闲位置。"
		}
		for _, slot := range o.Slots {
			if slot.Week < 1 || slot.Week > 60 || slot.Day < 1 || slot.Day > 7 || slot.Section < 1 || slot.Section > 14 {
				r.complete = false
				r.warning = "模拟课表含无效时间，不能确认空闲位置。"
				continue
			}
			r.times = append(r.times, CourseTime{Weeks: []int{slot.Week}, Day: slot.Day, Sections: []int{slot.Section}})
		}
	}
	// Group the expanded API slots before exposing busy times to the model/UI.
	byPeriod := map[[2]int]map[int]bool{}
	for _, t := range r.times {
		key := [2]int{t.Day, t.Sections[0]}
		if byPeriod[key] == nil {
			byPeriod[key] = map[int]bool{}
		}
		byPeriod[key][t.Weeks[0]] = true
	}
	r.times = nil
	groups := map[string]int{}
	for day := 1; day <= 7; day++ {
		for section := 1; section <= 14; section++ {
			weeks := []int{}
			for week := range byPeriod[[2]int{day, section}] {
				weeks = append(weeks, week)
			}
			if len(weeks) == 0 {
				continue
			}
			sort.Ints(weeks)
			key := fmt.Sprint(day) + "/" + integerList(weeks)
			if index, ok := groups[key]; ok {
				r.times[index].Sections = append(r.times[index].Sections, section)
			} else {
				groups[key] = len(r.times)
				r.times = append(r.times, CourseTime{Day: day, Weeks: weeks, Sections: []int{section}})
			}
		}
	}
	r.times = uniqueTimes(r.times)
	return r, data.Revision
}
