package tools

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"time"
)

type FitCoursesInput struct {
	TimeOfDay       []string        `json:"timeOfDay,omitempty" jsonschema:"description=用户允许的时段：morning上午1–5节、afternoon下午6–9节、evening晚上10–13节；多项取并集，与allowedSections同传取交集"`
	CoreKeywords    []CourseKeyword `json:"coreKeywords,omitempty" jsonschema:"description=模糊课程名的核心词，规则同 check_course_offerings"`
	CourseIDs       []string        `json:"courseIDs,omitempty" jsonschema:"description=从开课工具候选中选定的课程号，可保留多个近似候选；只检查选定课程号的教学班"`
	Courses         []string        `json:"courses,omitempty" jsonschema:"description=待放入课表的完整课程名，最多12门；省略时单独读取本人课表占用时段，不要求再找课程"`
	SchoolYear      string          `json:"schoolYear,omitempty" jsonschema:"description=明确学年时传 YYYY-YYYY，与 semester 同传；本学期省略"`
	Semester        int             `json:"semester,omitempty" jsonschema:"description=1、2或3，与 schoolYear 同传"`
	AllowedDays     []int           `json:"allowedDays,omitempty" jsonschema:"description=用户允许上课的星期，周一1到周日7，省略不限制；不要自行添加偏好"`
	AllowedSections []int           `json:"allowedSections,omitempty" jsonschema:"description=用户允许的节次1到14，课程全部节次都必须在范围内，省略不限制"`
}
type CourseFit struct {
	Offering  Offering     `json:"offering"`
	Status    string       `json:"status"` // fits, conflict, outside_preferences, already_enrolled, unknown
	Conflicts []CourseTime `json:"conflicts,omitempty"`
	Reason    string       `json:"reason"`
}
type FitCoursesResult struct {
	Term             AcademicTerm    `json:"term"`
	TermSource       string          `json:"termSource"`
	CheckedAt        string          `json:"checkedAt"`
	ScheduleComplete bool            `json:"scheduleComplete"`
	ScheduleEmpty    bool            `json:"scheduleEmpty"`
	BusyTimes        []CourseTime    `json:"busyTimes"`
	Offerings        *OfferingResult `json:"offerings,omitempty"`
	Fits             []CourseFit     `json:"fits"`
	CandidateFits    []CourseFit     `json:"candidateFits,omitempty"`
	SuggestedPlan    []string        `json:"suggestedPlan,omitempty"`
	Warnings         []string        `json:"warnings,omitempty"`
}
type scheduleResult struct {
	times            []CourseTime
	complete         bool
	classes, courses map[string]bool
	warning          string
}
type scheduleRow struct {
	SchoolYear string `json:"schoolYear"`
	Semester   int    `json:"semester"`
	ClassTime  string `json:"classTime"`
	ClassID    string `json:"classId"`
	CourseID   string `json:"courseId"`
	CourseCode string `json:"courseCode"`
}

func uniqueTimes(times []CourseTime) []CourseTime {
	result := []CourseTime{}
	seen := map[string]bool{}
	for _, slot := range times {
		key, _ := json.Marshal(slot)
		if !seen[string(key)] {
			seen[string(key)] = true
			result = append(result, slot)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Day != result[j].Day {
			return result[i].Day < result[j].Day
		}
		if result[i].Sections[0] != result[j].Sections[0] {
			return result[i].Sections[0] < result[j].Sections[0]
		}
		return result[i].Weeks[0] < result[j].Weeks[0]
	})
	return result
}

func (s *CampusSession) schedule(ctx context.Context, term AcademicTerm) scheduleResult {
	if cached, ok := s.schedules[term]; ok {
		return cached
	}
	result := scheduleResult{times: []CourseTime{}, complete: true, classes: map[string]bool{}, courses: map[string]bool{}}
	s.progress("正在读取本人课表并核对全部分页…")
	offset, total := 0, -1
	seenPages := map[string]bool{}
	for page := 0; page < 10; page++ {
		q := term.query()
		q.Set("limit", "200")
		q.Set("offset", strconv.Itoa(offset))
		var rows []scheduleRow
		meta, err := s.get(ctx, "/academic/schedule", q, &rows)
		if err != nil {
			result.complete = false
			result.warning = campusError(err)
			break
		}
		if rows == nil || len(rows) > 200 {
			result.complete = false
			result.warning = "课表列表格式无效，无法确认空闲时间。"
			break
		}
		encoded, _ := json.Marshal(rows)
		if seenPages[string(encoded)] && len(rows) > 0 {
			result.complete = false
			result.warning = "课表分页重复，无法确认已读取全部课程。"
			break
		}
		seenPages[string(encoded)] = true
		for _, r := range rows {
			if r.SchoolYear != term.SchoolYear || r.Semester != term.Semester {
				result.complete = false
				result.warning = "课表包含不属于目标学期的记录，无法确认空闲时间。"
				continue
			}
			// classTime describes all meetings, while week/period may describe only
			// one expanded row. Never substitute a single row for an unknown schedule.
			times, ok := parseCourseTimes(r.ClassTime)
			if !ok {
				result.complete = false
				result.warning = "部分已选课程的上课时间缺失或无法完整解析，暂不能确认空闲位置。"
			}
			result.times = append(result.times, times...)
			if r.ClassID != "" {
				result.classes[r.ClassID] = true
			}
			if r.CourseID != "" {
				result.courses[r.CourseID] = true
			}
			if r.CourseCode != "" {
				result.courses[r.CourseCode] = true
			}
		}
		if meta == nil || !meta.valid || meta.Total < 0 || meta.Offset != offset || meta.Limit != 200 || (total >= 0 && meta.Total != total) {
			result.complete = false
			result.warning = "课表分页信息缺失或变化，无法确认已读取全部课程。"
			break
		}
		total = meta.Total
		if !meta.HasMore {
			if offset+len(rows) != total {
				result.complete = false
				result.warning = "课表条数与分页总数不一致，无法确认空闲时间。"
			}
			break
		}
		if len(rows) == 0 || meta.NextOffset != offset+len(rows) || meta.NextOffset >= total || page == 9 {
			result.complete = false
			result.warning = "课表分页未完成，暂不能确认空闲位置。"
			break
		}
		offset = meta.NextOffset
	}
	result.times = uniqueTimes(result.times)
	if result.complete {
		s.schedules[term] = result
	}
	return result
}

func validSelection(values []int, max int) bool {
	if len(values) > max {
		return false
	}
	for _, v := range values {
		if v < 1 || v > max {
			return false
		}
	}
	return true
}
func containsAll(allowed, values []int) bool {
	if len(allowed) == 0 {
		return true
	}
	return len(overlap(values, allowed)) == len(values)
}
func fitCourse(o Offering, schedule scheduleResult, in FitCoursesInput) CourseFit {
	fit := CourseFit{Offering: o, Status: "unknown", Reason: "上课时间或本人课表未完整核实。"}
	if schedule.classes[o.ClassID] || schedule.courses[o.CourseID] {
		fit.Status = "already_enrolled"
		fit.Reason = "本人课表中已有该教学班或同课程号课程。"
		return fit
	}
	for _, t := range o.Times {
		for _, busy := range schedule.times {
			if hit, ok := clash(t, busy); ok {
				fit.Conflicts = append(fit.Conflicts, hit)
			}
		}
	}
	if len(fit.Conflicts) > 0 {
		fit.Status = "conflict"
		fit.Reason = "与本人已选课程在相同周次、星期及节次重叠。"
		fit.Conflicts = uniqueTimes(fit.Conflicts)
		return fit
	}
	for _, t := range o.Times {
		if !containsAll(in.AllowedDays, []int{t.Day}) || !containsAll(in.AllowedSections, t.Sections) {
			fit.Status = "outside_preferences"
			fit.Reason = "上课时间不满足用户指定的星期或节次范围。"
			return fit
		}
	}
	if schedule.complete && o.TimeComplete {
		fit.Status = "fits"
		fit.Reason = "在已核实的完整课表中，全部上课周次与节次均无冲突，且符合指定时间范围。"
	}
	return fit
}

func (s *CampusSession) FitCourses(ctx context.Context, in FitCoursesInput) (result FitCoursesResult, err error) {
	s.Calls++
	s.scheduleRequested = true
	if len(in.TimeOfDay) > 0 {
		sections, timeErr := sectionsForTimeOfDay(in.TimeOfDay, in.AllowedSections)
		if timeErr != nil {
			return result, timeErr
		}
		in.AllowedSections = sections
	}
	if in.AllowedDays == nil {
		in.AllowedDays = s.fitPreferences.AllowedDays
	}
	if in.AllowedSections == nil {
		in.AllowedSections = s.fitPreferences.AllowedSections
	}
	s.fitPreferences = in
	// A later schedule-only call must not erase candidates already checked in
	// this answer. Reuse them only within the same semester.
	if len(in.Courses) == 0 && s.HasCourseResults() && (in.SchoolYear == "" || (in.SchoolYear == s.lastOfferings.Term.SchoolYear && in.Semester == s.lastOfferings.Term.Semester)) {
		in.Courses, in.CoreKeywords, in.CourseIDs = s.lastInput.Courses, s.lastInput.CoreKeywords, s.lastInput.CourseIDs
		in.SchoolYear, in.Semester = s.lastOfferings.Term.SchoolYear, s.lastOfferings.Term.Semester
	}
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	result = FitCoursesResult{CheckedAt: time.Now().Format(time.RFC3339), BusyTimes: []CourseTime{}, Fits: []CourseFit{}, Warnings: []string{"仅按教务课表核对整学期的周次、星期、节次；未校验选课资格、余量、考试冲突或跨校区通勤。fits表示单门与原课表兼容；suggestedPlan是一组相互无冲突且每个课程号最多一个班的候选组合，不是已选课，也不是最优方案。"}}
	defer func() {
		s.lastFit = &result
		s.lastOfferings = result.Offerings
		s.lastInput = OfferingInput{Courses: in.Courses, CoreKeywords: in.CoreKeywords, CourseIDs: in.CourseIDs, SchoolYear: in.SchoolYear, Semester: in.Semester}
	}()
	if !validSelection(in.AllowedDays, 7) || !validSelection(in.AllowedSections, 14) {
		result.Warnings = append(result.Warnings, "允许的星期须为1–7，节次须为1–14。")
		return result, nil
	}
	names, err := validCourses(in.Courses, true)
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		return result, nil
	}
	if _, err := lookupKeywords(names, in.CoreKeywords, in.CourseIDs); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		return result, nil
	}
	input := OfferingInput{Courses: names, SchoolYear: in.SchoolYear, Semester: in.Semester, CoreKeywords: in.CoreKeywords, CourseIDs: in.CourseIDs}
	result.Term, result.TermSource, err = s.resolveTerm(ctx, input)
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		return result, nil
	}
	schedule := s.schedule(ctx, result.Term)
	if errors.Is(ctx.Err(), context.Canceled) {
		return result, ctx.Err()
	}
	result.ScheduleComplete = schedule.complete
	result.ScheduleEmpty = schedule.complete && len(schedule.classes) == 0 && len(schedule.times) == 0
	if len(names) == 0 {
		result.BusyTimes = schedule.times
	}
	if schedule.warning != "" {
		result.Warnings = append(result.Warnings, schedule.warning)
	}
	if result.ScheduleEmpty {
		result.Warnings = append(result.Warnings, "教务返回该学期空课表，请确认学期及尚未同步的选课；空闲判断仅基于此次返回。")
	}
	if len(names) == 0 {
		return result, nil
	}
	// Both reads use exactly the resolved term, even if defaults change mid-turn.
	input.SchoolYear, input.Semester = result.Term.SchoolYear, result.Term.Semester
	offerings, err := s.offerings(ctx, input)
	offerings.TermSource = result.TermSource
	result.Offerings = &offerings
	if err != nil {
		return result, err
	}
	s.progress("正在按周次、星期和节次检查可放入课表的班级…")
	chosen := []Offering{}
	chosenCourses := map[string]bool{}
	for _, q := range offerings.Queries {
		if q.NeedsSelection {
			for _, candidate := range q.Candidates {
				for _, o := range candidate.Classes {
					result.CandidateFits = append(result.CandidateFits, fitCourse(o, schedule, in))
				}
			}
			result.Warnings = append(result.Warnings, "有候选名称尚未选择。candidateFits已核对候选班级与课表的兼容性，但尚未纳入suggestedPlan；先选择最接近的courseIDs再次调用本工具，再给出最终组合。不能把未列入fits解读为时间冲突。")
		}
		for _, o := range q.Classes {
			fit := fitCourse(o, schedule, in)
			result.Fits = append(result.Fits, fit)
			if fit.Status != "fits" || chosenCourses[o.CourseID] {
				continue
			}
			compatible := true
			for _, old := range chosen {
				for _, a := range old.Times {
					for _, b := range o.Times {
						if _, ok := clash(a, b); ok {
							compatible = false
						}
					}
				}
			}
			if compatible {
				chosen = append(chosen, o)
				chosenCourses[o.CourseID] = true
				result.SuggestedPlan = append(result.SuggestedPlan, o.ClassID)
			}
		}
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return result, ctx.Err()
	}
	return result, nil
}

// Convert the product's named time blocks once, rather than asking the model
// to repeatedly enumerate periods. Explicit sections further narrow the range.
func sectionsForTimeOfDay(blocks []string, explicit []int) ([]int, error) {
	selected := map[int]bool{}
	for _, block := range blocks {
		bounds, ok := map[string][2]int{"morning": {1, 5}, "afternoon": {6, 9}, "evening": {10, 13}}[block]
		if !ok {
			return nil, errors.New("时段须为 morning、afternoon 或 evening")
		}
		for n := bounds[0]; n <= bounds[1]; n++ {
			selected[n] = true
		}
	}
	sections := []int{}
	for n := 1; n <= 13; n++ {
		if selected[n] && containsAll(explicit, []int{n}) {
			sections = append(sections, n)
		}
	}
	if len(sections) == 0 {
		return nil, errors.New("指定时段与节次没有交集，请确认时间条件")
	}
	return sections, nil
}
