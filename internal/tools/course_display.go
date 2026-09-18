package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type ShowCoursesInput struct {
	ScheduleSource  string   `json:"scheduleSource,omitempty" jsonschema:"enum=simulation,enum=actual,description=补空默认simulation；仅明确查看真实课表时actual；省略沿用本轮来源"`
	CourseIDs       []string `json:"courseIDs,omitempty" jsonschema:"description=可选：根据名称和上下文选本轮官方候选的原始课程号，可选多个；直接从已有结果选定并重算组合，不必再查一次。仅列候选时省略，不自动选择，不删除其他查询"`
	TimeOfDay       []string `json:"timeOfDay,omitempty" jsonschema:"description=用户允许的时段；仅查询开课也可过滤，不必读取本人课表；省略保留此前时段，空数组清空时段限制：morning上午1–5节、afternoon下午6–9节、evening晚上10–13节"`
	MatchSchedule   bool     `json:"matchSchedule" jsonschema:"description=用户是否要求结合本人课表找空闲位置；true时若此前只查开课，将在显示前补做课表检查"`
	AllowedDays     []int    `json:"allowedDays,omitempty" jsonschema:"description=用户允许上课的星期；只查开课时也可过滤时间，省略保留此前偏好，空数组清空该限制"`
	AllowedSections []int    `json:"allowedSections,omitempty" jsonschema:"description=用户允许的节次；只查开课时也可过滤时间，省略保留此前偏好，空数组清空该限制"`
}

// prepareCourseResults performs only explicit selection and deterministic filtering.
func (s *CampusSession) prepareCourseResults(ctx context.Context, in ShowCoursesInput) error {
	s.Calls++
	if len(in.CourseIDs) > 0 && s.HasCourseResults() {
		if _, err := lookupKeywords(nil, nil, in.CourseIDs); err != nil {
			return err
		}
		s.selectShownCourses(in.CourseIDs)
	}
	if in.TimeOfDay != nil && len(in.TimeOfDay) == 0 && in.AllowedSections == nil {
		in.AllowedSections = []int{}
	}
	if s.HasCourseResults() && (in.MatchSchedule || s.scheduleRequested) && (s.lastFit == nil || s.lastFit.Offerings != s.lastOfferings || (in.ScheduleSource != "" && in.ScheduleSource != s.lastFit.ScheduleSource) || len(in.TimeOfDay) > 0 || in.AllowedDays != nil || in.AllowedSections != nil) {
		term := s.lastOfferings.Term
		_, err := s.FitCourses(ctx, FitCoursesInput{ScheduleSource: in.ScheduleSource, TimeOfDay: in.TimeOfDay, SchoolYear: term.SchoolYear, Semester: term.Semester, AllowedDays: in.AllowedDays, AllowedSections: in.AllowedSections})
		if err != nil {
			return err
		}
	}
	if !s.scheduleRequested || !s.HasCourseResults() {
		sections := in.AllowedSections
		if len(in.TimeOfDay) > 0 {
			var err error
			sections, err = sectionsForTimeOfDay(in.TimeOfDay, sections)
			if err != nil {
				return err
			}
		}
		if !validSelection(in.AllowedDays, 7) || !validSelection(sections, 14) {
			return fmt.Errorf("允许的星期须为1–7，节次须为1–14")
		}
		preferences := &s.displayPreferences
		if s.lastFit != nil && !s.HasCourseResults() {
			preferences = &s.fitPreferences
		}
		if in.AllowedDays != nil {
			preferences.AllowedDays = in.AllowedDays
		}
		if sections != nil {
			preferences.AllowedSections = sections
		}
	}
	return nil
}

// CourseResults never terminates a turn or formats the assistant's answer.
func (s *CampusSession) CourseResults(ctx context.Context, in ShowCoursesInput) (CourseResultData, error) {
	if err := s.prepareCourseResults(ctx, in); err != nil {
		return CourseResultData{}, err
	}
	result := CourseResultData{Offerings: s.lastOfferings, Fit: s.lastFit, Term: s.lastTermResult, Preferences: s.displayPreferences}
	if s.lastFit != nil {
		result.Preferences = s.fitPreferences
		copy := *s.lastFit
		copy.Availability = scheduleWindows(copy, s.fitPreferences)
		result.Fit = &copy
	}
	if !s.scheduleRequested && s.lastOfferings != nil {
		for _, q := range s.lastOfferings.Queries {
			classes := append([]Offering{}, q.Classes...)
			for _, c := range q.Candidates {
				classes = append(classes, c.Classes...)
			}
			for _, o := range classes {
				status := "unknown_time"
				if fitCourse(o, scheduleResult{}, s.displayPreferences).Status == "outside_preferences" {
					status = "outside_preferences"
				} else if o.TimeComplete {
					status = "within_preferences"
				}
				result.TimeChecks = append(result.TimeChecks, CourseTimeCheck{ClassID: o.ClassID, Status: status})
			}
		}
	}
	return result, nil
}

type CourseTimeCheck struct {
	ClassID string `json:"classID"`
	Status  string `json:"status"`
}
type CourseResultData struct {
	TimeChecks  []CourseTimeCheck   `json:"timeChecks,omitempty"`
	Offerings   *OfferingResult     `json:"offerings,omitempty"`
	Fit         *FitCoursesResult   `json:"fit,omitempty"`
	Term        *AcademicTermResult `json:"term,omitempty"`
	Preferences FitCoursesInput     `json:"preferences"`
}

func integerList(numbers []int) string {
	parts := []string{}
	for i := 0; i < len(numbers); i++ {
		start, end := numbers[i], numbers[i]
		for i+1 < len(numbers) && numbers[i+1] == end+1 {
			i++
			end = numbers[i]
		}
		if start == end {
			parts = append(parts, fmt.Sprint(start))
		} else {
			parts = append(parts, fmt.Sprintf("%d–%d", start, end))
		}
	}
	return strings.Join(parts, "、")
}

// Scoped occupancy remains a deterministic calculation, independent of prose.
type ScheduleWindow struct {
	Day       int    `json:"day"`
	Sections  []int  `json:"sections"`
	BusyWeeks []int  `json:"busyWeeks"`
	Status    string `json:"status"`
}

func scheduleWindows(f FitCoursesResult, pref FitCoursesInput) []ScheduleWindow {
	if len(pref.AllowedDays) == 0 && len(pref.AllowedSections) == 0 {
		return nil
	}
	result := []ScheduleWindow{}
	for day := 1; day <= 7; day++ {
		if !containsAll(pref.AllowedDays, []int{day}) {
			continue
		}
		for section := 1; section <= 14; section++ {
			if !containsAll(pref.AllowedSections, []int{section}) {
				continue
			}
			occupied := map[int]bool{}
			for _, slot := range f.BusyTimes {
				if slot.Day == day && len(overlap(slot.Sections, []int{section})) > 0 {
					for _, w := range slot.Weeks {
						occupied[w] = true
					}
				}
			}
			weeks := []int{}
			for w := range occupied {
				weeks = append(weeks, w)
			}
			sort.Ints(weeks)
			status := "unknown"
			if len(weeks) > 0 {
				status = "occupied"
			} else if f.ScheduleComplete {
				status = "free"
			}
			n := len(result)
			if n > 0 && result[n-1].Day == day && result[n-1].Sections[len(result[n-1].Sections)-1] == section-1 && integerList(result[n-1].BusyWeeks) == integerList(weeks) {
				result[n-1].Sections = append(result[n-1].Sections, section)
			} else {
				result = append(result, ScheduleWindow{Day: day, Sections: []int{section}, BusyWeeks: weeks, Status: status})
			}
		}
	}
	return result
}
