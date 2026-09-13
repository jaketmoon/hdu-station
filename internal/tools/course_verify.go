package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// VerifyCoursesInput deliberately leaves term, campus and schedule constraints
// out of model arguments: the product supplies those from the user's request.
type VerifyCoursesInput struct {
	Courses          []string `json:"courses" jsonschema:"required,description=本轮全部候选的完整名称或课程号，1至12门；追问沿用此前课程，不传班级号"`
	CommunitySummary string   `json:"communitySummary,omitempty" jsonschema:"description=仅概述本轮已读帖子中的体验和推荐理由，用原帖引用；不写教务时间或班级号"`
}

type VerifyOptions struct {
	MatchSchedule                bool
	AllowedDays, AllowedSections []int
	MaxCourses                   int
	MinCourses                   int
}

// VerifyCourses is the only product-facing campus operation. It cannot finish
// with a timetable alone, nor silently move to a different term or campus.
func (s *CampusSession) VerifyCourses(ctx context.Context, in VerifyCoursesInput, opts VerifyOptions) (string, error) {
	for i, name := range in.Courses {
		in.Courses[i] = cleanCourseQuery(name)
	}
	if _, err := validCourses(in.Courses, false); err != nil {
		return "", err
	}
	s.XiashaOnly = true
	s.automaticMatching = true
	s.maxCourses = opts.MaxCourses
	if opts.MatchSchedule {
		_, err := s.FitCourses(ctx, FitCoursesInput{Courses: in.Courses, AllowedDays: opts.AllowedDays, AllowedSections: opts.AllowedSections})
		if err != nil {
			return "", err
		}
	} else {
		_, err := s.CheckOfferings(ctx, OfferingInput{Courses: in.Courses})
		if err != nil {
			return "", err
		}
	}
	display := s.Display()
	count := 0
	names := []string{}
	if s.lastOfferings != nil {
		seen := map[string]bool{}
		for _, q := range s.lastOfferings.Queries {
			for _, o := range q.Classes {
				if !seen[o.CourseID] {
					names = append(names, campusCell(o.CourseName))
				}
				seen[o.CourseID] = true
			}
		}
		count = len(seen)
	}
	if opts.MatchSchedule && s.lastFit != nil {
		count = len(s.lastFit.SuggestedPlan)
	}
	if count < opts.MinCourses {
		condition := "已核实开课"
		if opts.MatchSchedule {
			condition = "能同时放入课表且符合时间偏好"
		}
		display = fmt.Sprintf("本次%s的课程为 %d 门，未达到所需至少 %d 门；推荐数量尚未补足。\n\n", condition, count, opts.MinCourses) + display
	}
	if !opts.MatchSchedule && opts.MinCourses > 0 && len(names) > 0 {
		n := len(names)
		if opts.MaxCourses > 0 && n > opts.MaxCourses {
			n = opts.MaxCourses
		}
		display = fmt.Sprintf("按本次候选顺序，先列 %d 门已核实开课的候选：%s。其余查到的课程列作备选；开课不代表已满足口碑偏好。\n\n", n, strings.Join(names[:n], "、")) + display
	}
	return display, nil
}

var trailingCourseCode = regexp.MustCompile(`\s*[/（(\s]\s*[A-Za-z][0-9]{7,11}[）)]?\s*$`)

func cleanCourseQuery(name string) string {
	// Models sometimes copy the visible “name (course code)” label wholesale.
	// Query the name, so a mismatched copied code cannot silently change courses.
	name = strings.TrimSpace(name)
	cleaned := strings.TrimSpace(trailingCourseCode.ReplaceAllString(name, ""))
	if cleaned != "" {
		return cleaned
	}
	return name
}

// Community posts are leads, not verified current teaching requirements.
// A name appearing somewhere in prose cannot establish course/teacher identity.
func (s *CampusSession) CommunityCaveat(_ string) string {
	return "**口碑核实范围：**社区摘要仅是往届线索，尚未逐课程、逐教师核实到本学期；不能据此保证作业少、给分高、无需绘画基础或免考试。下列官方结果仅核实开课和时间，不代表这些体验条件已经满足。\n\n"
}

func normalizedCourseName(name string) string {
	name = strings.TrimSpace(strings.Trim(name, "《》「」"))
	if len([]rune(name)) >= 4 {
		name = strings.TrimSuffix(name, "课")
	}
	for _, suffix := range []string{"鉴赏", "赏析", "欣赏"} {
		name = strings.TrimSuffix(name, suffix)
	}
	return name
}

// Only equivalent names are promoted automatically. Broader search hits remain
// visible candidates and require a full name from the user, not a model guess.
func resolveCourseNames(q OfferingQuery) OfferingQuery {
	if len(q.Classes) != 0 {
		return q
	}
	key := normalizedCourseName(q.Query)
	for _, c := range q.Candidates {
		if len([]rune(key)) >= 2 && normalizedCourseName(c.CourseName) == key {
			q.Classes = append(q.Classes, c.Classes...)
		}
	}
	q.NeedsSelection = len(q.Classes) == 0 && len(q.Candidates) > 0
	return q
}
