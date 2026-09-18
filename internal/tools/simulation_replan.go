package tools

import "context"

type SimulationReplanInput struct {
	SchoolYear string   `json:"schoolYear,omitempty"`
	Semester   int      `json:"semester,omitempty"`
	ClassIDs   []string `json:"classIDs" jsonschema:"description=本轮模拟课表read中用户指定重排集合的全部ENROLL教学班ID，1–100个；其他课程保持固定，不接受真实课程或DROP"`
}

type SimulationReplanResult struct {
	Status         string           `json:"status"`
	Message        string           `json:"message"`
	Fits           []CourseFit      `json:"fits,omitempty"`
	SuggestedPlan  []string         `json:"suggestedPlan,omitempty"`
	PlanIncomplete bool             `json:"planIncomplete"`
	Update         *SimulationInput `json:"update,omitempty"`
}

// Replanning is a read-only calculation over existing enrollments. The returned
// patch removes only unselected members; it never clears the live timetable.
func (s *CampusSession) PlanSimulation(ctx context.Context, in SimulationReplanInput) (SimulationReplanResult, error) {
	result := SimulationReplanResult{Status: "blocked"}
	read, err := s.ManageSimulation(ctx, SimulationInput{Action: "read", SchoolYear: in.SchoolYear, Semester: in.Semester})
	if err != nil || read.Status != "read" {
		result.Message = read.Message
		return result, err
	}
	d := s.simulationSnapshot // Full host-only slots, not the redacted read result.
	if len(in.ClassIDs) == 0 || len(in.ClassIDs) > 100 {
		result.Message = "请指定1–100个已有模拟加入班级作为待选集合。"
		return result, nil
	}
	actual := map[string]bool{}
	for _, o := range d.ActualCourses {
		actual[o.ClassID] = true
	}
	target := map[string]string{}
	for _, item := range d.SimulationItems {
		target[item.ClassID] = item.Intent
	}
	// This workflow treats every real enrollment as fixed, even when an earlier
	// simulation DROP hides it. Preserve that action on save, but never use it
	// to create room in the proposed combination.
	for id := range actual {
		delete(target, id)
	}
	selected := map[string]bool{}
	for _, id := range in.ClassIDs {
		if selected[id] || actual[id] || target[id] != "ENROLL" {
			result.Message = "待选集合包含重复项或非当前模拟加入班级；请按最新read结果定位，不修改真实课程或其他模拟动作。"
			return result, nil
		}
		selected[id] = true
		delete(target, id)
	}
	// Fixed courses must themselves be conflict-free and fully known.
	if !simulationFits(d, target, nil) {
		result.Message = "保留课程已有冲突或时间不完整，无法通过仅重排指定集合得到已确认无冲突的课表。"
		return result, nil
	}
	base := scheduleResult{complete: true, classes: map[string]bool{}, courses: map[string]bool{}}
	candidates := map[string]Offering{}
	fixedAndCandidates := append([]SimulationCourse{}, d.ActualCourses...)
	for _, c := range d.EffectiveCourses {
		if !actual[c.ClassID] {
			fixedAndCandidates = append(fixedAndCandidates, c)
		}
	}
	for _, c := range fixedAndCandidates {
		o := Offering{ClassID: c.ClassID, CourseID: c.CourseID, CourseName: c.CourseName, Teacher: c.Teacher, ClassTime: c.ClassTime, TimeComplete: c.ScheduleKnown && len(c.Slots) > 0}
		for _, slot := range c.Slots {
			if slot.Week < 1 || slot.Week > 60 || slot.Day < 1 || slot.Day > 7 || slot.Section < 1 || slot.Section > 14 {
				o.TimeComplete = false
				continue
			}
			o.Times = append(o.Times, CourseTime{Weeks: []int{slot.Week}, Day: slot.Day, Sections: []int{slot.Section}})
		}
		if selected[c.ClassID] {
			candidates[c.ClassID] = o
		} else {
			base.classes[c.ClassID] = true
			if c.CourseID != "" {
				base.courses[c.CourseID] = true
			}
			base.times = append(base.times, o.Times...)
			base.complete = base.complete && o.TimeComplete
		}
	}
	if !base.complete {
		result.Message = "固定课程时间无效或不完整，未生成修改。"
		return result, nil
	}
	for _, id := range in.ClassIDs {
		o, ok := candidates[id]
		if !ok || o.CourseID == "" {
			result.Message = "待选班级缺少有效课程记录或课程号，无法确认每门最多一班；未生成修改。"
			return result, nil
		}
		result.Fits = append(result.Fits, fitCourse(o, base, FitCoursesInput{ScheduleSource: "simulation"}))
	}
	var complete bool
	result.SuggestedPlan, complete = bestCoursePlan(result.Fits, 12)
	result.PlanIncomplete = !complete
	keep := map[string]bool{}
	for _, id := range result.SuggestedPlan {
		keep[id] = true
		target[id] = "ENROLL"
	}
	if !simulationFits(d, target, nil) {
		result.Message = "目标方案未通过完整时间校验，未生成修改。"
		return result, nil
	}
	remove := []string{}
	for _, id := range in.ClassIDs {
		if !keep[id] {
			remove = append(remove, id)
		}
	}
	result.Update = &SimulationInput{Action: "update", SchoolYear: d.SchoolYear, Semester: d.Semester, ExpectedRevision: &d.Revision, RemoveClassIDs: remove, RequireNoConflicts: true}
	result.Status = "planned"
	result.Message = "已计算每门最多一班的无冲突组合，尚未保存；用update一次性保存，保留集合外所有模拟动作。"
	if !complete {
		result.Message += "达到12门课程或搜索步数上限，未证明门数最多。"
	}
	return result, nil
}
