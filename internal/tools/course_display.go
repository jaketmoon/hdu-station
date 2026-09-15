package tools

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type ShowCoursesInput struct {
	CourseIDs        []string `json:"courseIDs,omitempty" jsonschema:"description=可选：根据名称和上下文选本轮官方候选的原始课程号，可选多个；直接从已有结果选定并重算组合，不必再查一次。仅列候选时省略，不自动选择，不删除其他查询"`
	TimeOfDay        []string `json:"timeOfDay,omitempty" jsonschema:"description=用户允许的时段；仅查询开课也可过滤，不必读取本人课表；省略保留此前时段，空数组清空时段限制：morning上午1–5节、afternoon下午6–9节、evening晚上10–13节"`
	MatchSchedule    bool     `json:"matchSchedule" jsonschema:"description=用户是否要求结合本人课表找空闲位置；true时若此前只查开课，将在显示前补做课表检查"`
	AllowedDays      []int    `json:"allowedDays,omitempty" jsonschema:"description=用户允许上课的星期；只查开课时也可过滤时间，省略保留此前偏好，空数组清空该限制"`
	AllowedSections  []int    `json:"allowedSections,omitempty" jsonschema:"description=用户允许的节次；只查开课时也可过滤时间，省略保留此前偏好，空数组清空该限制"`
	CommunitySummary string   `json:"communitySummary,omitempty" jsonschema:"description=可选：仅概述本轮已读社区经验、推荐理由和原帖引用，不写校园班级号、上课时间或冲突表，这些由Go直接展示"`
}

func (s *CampusSession) ShowCourses(ctx context.Context, in ShowCoursesInput) (string, error) {
	s.Calls++
	if !s.CanShowCourses() {
		return "", nil
	}
	if len(in.CourseIDs) > 0 && s.HasCourseResults() {
		if _, err := lookupKeywords(nil, nil, in.CourseIDs); err != nil {
			return "", err
		}
		s.selectShownCourses(in.CourseIDs)
	}
	if in.TimeOfDay != nil && len(in.TimeOfDay) == 0 && in.AllowedSections == nil {
		in.AllowedSections = []int{}
	}
	if s.HasCourseResults() && (in.MatchSchedule || s.scheduleRequested) && (s.lastFit == nil || s.lastFit.Offerings != s.lastOfferings || len(in.TimeOfDay) > 0 || in.AllowedDays != nil || in.AllowedSections != nil) {
		term := s.lastOfferings.Term
		_, err := s.FitCourses(ctx, FitCoursesInput{TimeOfDay: in.TimeOfDay, SchoolYear: term.SchoolYear, Semester: term.Semester, AllowedDays: in.AllowedDays, AllowedSections: in.AllowedSections})
		if err != nil {
			return "", err
		}
	}
	if !s.scheduleRequested || !s.HasCourseResults() {
		sections := in.AllowedSections
		if len(in.TimeOfDay) > 0 {
			var err error
			sections, err = sectionsForTimeOfDay(in.TimeOfDay, sections)
			if err != nil {
				return "", err
			}
		}
		if !validSelection(in.AllowedDays, 7) || !validSelection(sections, 14) {
			return "", fmt.Errorf("允许的星期须为1–7，节次须为1–14")
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
	return s.Display(), nil
}

func campusCell(text string) string {
	text = strings.NewReplacer("\\", "\\\\", "|", "\\|", "\n", " ", "\r", " ", "`", "\\`", "[", "\\[", "]", "\\]", "*", "\\*", "_", "\\_", "<", "&lt;", ">", "&gt;").Replace(text)
	if text == "" {
		return "未提供"
	}
	return text
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
func displayTimes(times []CourseTime) string {
	parts := []string{}
	days := []string{"", "一", "二", "三", "四", "五", "六", "日"}
	for _, t := range times {
		parts = append(parts, fmt.Sprintf("第%s周，周%s第%s节", integerList(t.Weeks), days[t.Day], integerList(t.Sections)))
	}
	return strings.Join(parts, "；")
}

func (s *CampusSession) displayOfferingStatus(o Offering, fits map[string]CourseFit) string {
	status := "查到开课记录"
	if !s.scheduleRequested && (len(s.displayPreferences.AllowedDays) > 0 || len(s.displayPreferences.AllowedSections) > 0) {
		checked := fitCourse(o, scheduleResult{}, s.displayPreferences)
		if checked.Status == "outside_preferences" {
			status = "不符合指定时间偏好"
		} else if o.TimeComplete {
			status = "符合指定时间；未核对本人课表"
		} else {
			status = "上课时间不完整，待确认"
		}
	}
	if f, ok := fits[o.ClassID]; ok {
		switch f.Status {
		case "fits":
			status = "可放入空闲位置"
		case "conflict":
			status = "与已选课程冲突：" + displayTimes(f.Conflicts)
		case "already_enrolled":
			status = "课表中已有该课程"
		case "outside_preferences":
			status = "不符合指定时间偏好"
		default:
			status = "待确认：课表或上课时间未完整核实"
		}
	} else if s.lastFit != nil {
		status = "尚未核实课表兼容性"
	}
	return status
}

// Only this renderer pairs official class IDs with time and computed status.
// Model prose is not used to reconstruct authoritative campus tables.
func (s *CampusSession) Display() string {
	r := s.lastOfferings
	termDisplay := ""
	if s.lastTermResult != nil {
		termDisplay = displayAcademicTerm(*s.lastTermResult)
	}
	if r == nil {
		if s.lastFit != nil {
			return termDisplay + s.displaySchedule()
		}
		if termDisplay != "" {
			return termDisplay
		}
		return "尚未取得开课或课表查询结果。"
	}
	var b strings.Builder
	if s.lastTermResult != nil && s.lastTermResult.Term.valid() && s.lastTermResult.Term == r.Term {
		termDisplay = ""
	}
	b.WriteString(termDisplay)
	if r.Term.valid() {
		fmt.Fprintf(&b, "已查询 **%s 学年第 %d 学期**（%s）。\n\n", r.Term.SchoolYear, r.Term.Semester, campusCell(r.TermSource))
	}
	fits := map[string]CourseFit{}
	if s.lastFit != nil {
		for _, f := range s.lastFit.Fits {
			fits[f.Offering.ClassID] = f
		}
	}
	rows := 0
	for _, q := range r.Queries {
		rows += len(q.Classes)
	}
	for _, q := range r.Queries {
		names := []string{}
		seen := map[string]bool{}
		aliases := []string{}
		for _, o := range q.Classes {
			if !seen[o.CourseID] {
				seen[o.CourseID] = true
				names = append(names, campusCell(o.CourseName)+"（"+campusCell(o.CourseID)+"）")
				key := r.Term.SchoolYear + "/" + strconv.Itoa(r.Term.Semester) + "/" + o.CourseID
				for _, origin := range s.origins[key] {
					if origin != q.Query {
						aliases = append(aliases, campusCell(origin)+" → "+campusCell(o.CourseName)+"（"+campusCell(o.CourseID)+"，名称相近，可能对应）")
					}
				}
			}
		}
		if len(names) > 0 {
			if len(aliases) > 0 {
				b.WriteString(strings.Join(aliases, "。\n\n") + "。\n\n")
			}
			fmt.Fprintf(&b, "%s → %s", campusCell(q.Query), strings.Join(names, "、"))
			if len(q.Classes) > 0 && q.Classes[0].CourseName != q.Query {
				b.WriteString("（名称相近，可能对应）")
			}
			b.WriteString("。\n\n")
		}
		if len(q.Classes) == 0 {
			status := "尚未匹配到已选定的官方课程"
			if q.Warning != "" && !q.NeedsSelection {
				status = "开课情况尚未核实"
			}
			fmt.Fprintf(&b, "%s：%s", campusCell(q.Query), status)
			if len(q.Candidates) > 0 {
				parts := []string{}
				for _, c := range q.Candidates {
					parts = append(parts, campusCell(c.CourseName)+"（"+campusCell(c.CourseID)+"）")
				}
				b.WriteString("；候选有 " + strings.Join(parts, "、"))
			}
			b.WriteString("。\n\n")
		}
	}
	if rows > 0 {
		b.WriteString("| 课程 / 课程号 | 老师 · 学分 · 考核 | 上课时间 · 地点 | 核实结果 |\n| --- | --- | --- | --- |\n")
		seen := map[string]bool{}
		for _, q := range r.Queries {
			for _, o := range q.Classes {
				if seen[o.ClassID] {
					continue
				}
				seen[o.ClassID] = true
				status := s.displayOfferingStatus(o, fits)
				fmt.Fprintf(&b, "| %s / %s | %s · %s学分 · %s | %s · %s %s | %s |\n", campusCell(o.CourseName), campusCell(o.CourseID), campusCell(o.Teacher), campusCell(o.Credit), campusCell(o.Examination), campusCell(o.ClassTime), campusCell(o.Campus), campusCell(o.Location), campusCell(status))
			}
		}
		b.WriteString("\n")
	}
	pendingFits := map[string]CourseFit{}
	if s.lastFit != nil {
		for _, f := range s.lastFit.CandidateFits {
			pendingFits[f.Offering.ClassID] = f
		}
	}
	seenPending := map[string]bool{}
	for _, q := range r.Queries {
		for _, o := range q.Classes {
			seenPending[o.ClassID] = true
		}
	}
	pendingHeader := false
	for _, q := range r.Queries {
		if !q.NeedsSelection {
			continue
		}
		for _, candidate := range q.Candidates {
			for _, o := range candidate.Classes {
				if seenPending[o.ClassID] {
					continue
				}
				seenPending[o.ClassID] = true
				if !pendingHeader {
					b.WriteString("**名称尚待匹配的官方候选：** 以下时间核对仅供比较，未计入可同时安排的组合。\n\n| 课程 / 课程号 | 老师 · 学分 · 考核 | 上课时间 · 地点 | 核实结果 |\n| --- | --- | --- | --- |\n")
					pendingHeader = true
				}
				status := s.displayOfferingStatus(o, pendingFits)
				if status == "可放入空闲位置" {
					status = "时间与本人课表兼容"
				}
				if status == "查到开课记录" {
					status += "；未核对本人课表"
				}
				status = "名称待匹配；" + status
				fmt.Fprintf(&b, "| %s / %s | %s · %s学分 · %s | %s · %s %s | %s |\n", campusCell(o.CourseName), campusCell(o.CourseID), campusCell(o.Teacher), campusCell(o.Credit), campusCell(o.Examination), campusCell(o.ClassTime), campusCell(o.Campus), campusCell(o.Location), campusCell(status))
			}
		}
	}
	if pendingHeader {
		b.WriteString("\n")
	}
	if s.lastFit != nil {
		f := s.lastFit
		if f.ScheduleEmpty {
			b.WriteString("教务返回该学期空课表；请确认学期及尚未同步的选课，空闲判断仅基于此次返回。\n\n")
		}
		if !f.ScheduleComplete {
			b.WriteString("本人课表未完整核实，不能确认空闲位置；已发现的冲突仍予以保留。\n\n")
		}
		if len(f.SuggestedPlan) > 0 {
			b.WriteString("**一组可以同时放入的候选班级：**\n\n")
			for _, id := range f.SuggestedPlan {
				o := fits[id].Offering
				fmt.Fprintf(&b, "- %s（%s，%s）\n", campusCell(o.CourseName), campusCell(o.Teacher), campusCell(o.ClassTime))
			}
			b.WriteString("\n每个课程号只选一个班；以上课程彼此及与原课表均不冲突。\n\n")
		}
		if f.PlanIncomplete {
			b.WriteString("组合搜索达到本轮计算上限；已列组合仍无冲突，但尚未确认是否还能安排更多课程。\n\n")
		}
		// Preserve actionable auth/transport failures without exposing internal
		// tool protocol instructions in the product response.
		for _, warning := range f.Warnings {
			if strings.Contains(warning, "授权") || strings.Contains(warning, "超时") || strings.Contains(warning, "限流") || strings.Contains(warning, "无法连接") {
				b.WriteString(campusCell(warning) + "\n\n")
			}
		}
	}
	for _, q := range r.Queries {
		if q.CampusUnconfirmed {
			b.WriteString("部分开课记录的校区未能确认，已略过；暂不能核实这些记录是否在下沙开课。\n\n")
			break
		}
	}
	seenWarnings := map[string]bool{}
	for _, q := range r.Queries {
		if q.Warning != "" && !q.NeedsSelection {
			if !seenWarnings[q.Warning] {
				b.WriteString(campusCell(q.Warning) + "\n\n")
				seenWarnings[q.Warning] = true
			}
		}
	}
	for _, warning := range r.Warnings {
		if warning == candidateLimitWarning {
			b.WriteString(candidateLimitWarning + "\n\n")
			break
		}
	}
	if !r.Term.valid() {
		for _, warning := range r.Warnings {
			b.WriteString(campusCell(warning) + "\n\n")
		}
	}
	b.WriteString("来源：校园教务查询。仅展示已确认下沙校区的记录；开课检索覆盖未确认，未检索到不等于未开课。未校验实时余量、选课资格、学分认定、考试冲突或跨校区通勤；以上为选课建议，尚未执行选课。")
	return b.String()
}

func (s *CampusSession) displaySchedule() string {
	f := s.lastFit
	var b strings.Builder
	if f.Term.valid() {
		fmt.Fprintf(&b, "已读取 **%s 学年第 %d 学期**的本人课表。\n\n", f.Term.SchoolYear, f.Term.Semester)
	}
	scoped := s.displayFocusedSchedule(&b)
	if !scoped && len(f.BusyTimes) > 0 {
		b.WriteString("已知占用时段：\n\n")
		for _, slot := range f.BusyTimes {
			fmt.Fprintf(&b, "- %s\n", displayTimes([]CourseTime{slot}))
		}
		b.WriteString("\n")
	}
	if f.ScheduleEmpty {
		b.WriteString("教务返回空课表；请确认学期及尚未同步的选课。\n\n")
	} else if f.ScheduleComplete {
		if scoped {
			b.WriteString("课表已完整读取；上表仅展示指定的星期和节次。\n\n")
		} else {
			b.WriteString("课表已完整读取；其余时间未发现已选课程占用。\n\n")
		}
	} else {
		b.WriteString("课表未完整核实，不能确认其他时段空闲。\n\n")
	}
	for _, warning := range f.Warnings {
		if !strings.Contains(warning, "suggestedPlan") {
			b.WriteString(campusCell(warning) + "\n\n")
		}
	}
	b.WriteString("时段约定：上午第1–5节，下午第6–9节，晚上第10–13节。来源：校园教务课表。")
	return b.String()
}

func (s *CampusSession) displayFocusedSchedule(b *strings.Builder) bool {
	pref, f := s.fitPreferences, s.lastFit
	if len(pref.AllowedDays) == 0 && len(pref.AllowedSections) == 0 {
		return false
	}
	b.WriteString("| 时间 | 已知占用周次 | 空闲说明 |\n| --- | --- | --- |\n")
	dayNames := []string{"", "一", "二", "三", "四", "五", "六", "日"}
	for day := 1; day <= 7; day++ {
		if !containsAll(pref.AllowedDays, []int{day}) {
			continue
		}
		groups := []CourseTime{}
		for section := 1; section <= 14; section++ {
			if !containsAll(pref.AllowedSections, []int{section}) {
				continue
			}
			occupied := map[int]bool{}
			for _, slot := range f.BusyTimes {
				if slot.Day == day && len(overlap(slot.Sections, []int{section})) > 0 {
					for _, week := range slot.Weeks {
						occupied[week] = true
					}
				}
			}
			weeks := []int{}
			for week := range occupied {
				weeks = append(weeks, week)
			}
			sort.Ints(weeks)
			if len(groups) > 0 && integerList(groups[len(groups)-1].Weeks) == integerList(weeks) {
				groups[len(groups)-1].Sections = append(groups[len(groups)-1].Sections, section)
			} else {
				groups = append(groups, CourseTime{Day: day, Sections: []int{section}, Weeks: weeks})
			}
		}
		for _, group := range groups {
			busy, free := "未发现占用", "空闲（以本次课表为准）"
			if len(group.Weeks) > 0 {
				busy = "第" + integerList(group.Weeks) + "周有课"
				free = "其余周次未发现占用"
			}
			if !f.ScheduleComplete {
				free = "课表未完整，暂不能确认空闲"
			}
			fmt.Fprintf(b, "| 周%s第%s节 | %s | %s |\n", dayNames[day], integerList(group.Sections), busy, free)
		}
	}
	b.WriteString("\n")
	return true
}
