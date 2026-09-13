package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

type ShowCoursesInput struct {
	MatchSchedule    bool   `json:"matchSchedule" jsonschema:"description=用户是否要求结合本人课表找空闲位置；true时若此前只查开课，将在显示前补做课表检查"`
	AllowedDays      []int  `json:"allowedDays,omitempty" jsonschema:"description=此前尚未做课表检查时，传用户允许上课的星期；已筛选时保留此前偏好"`
	AllowedSections  []int  `json:"allowedSections,omitempty" jsonschema:"description=此前尚未做课表检查时，传用户允许的节次；已筛选时保留此前偏好"`
	CommunitySummary string `json:"communitySummary,omitempty" jsonschema:"description=可选：仅概述本轮已读社区经验、推荐理由和原帖引用，不写校园班级号、上课时间或冲突表，这些由Go直接展示"`
}

func (s *CampusSession) ShowCourses(ctx context.Context, in ShowCoursesInput) (string, error) {
	s.Calls++
	if !s.CanShowCourses() {
		return "", nil
	}
	if (in.MatchSchedule || s.scheduleRequested) && (s.lastFit == nil || s.lastFit.Offerings != s.lastOfferings) {
		x := s.lastInput
		_, err := s.FitCourses(ctx, FitCoursesInput{Courses: x.Courses, CoreKeywords: x.CoreKeywords, CourseIDs: x.CourseIDs, SchoolYear: x.SchoolYear, Semester: x.Semester, AllowedDays: in.AllowedDays, AllowedSections: in.AllowedSections})
		if err != nil {
			return "", err
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

// Only this renderer pairs official class IDs with time and computed status.
// Model prose is not used to reconstruct authoritative campus tables.
func (s *CampusSession) Display() string {
	r := s.lastOfferings
	if r == nil {
		if s.lastFit != nil {
			text := "本次尚未完成具体课程的开课与时间核实，暂时无法给出可放入课表的班级。"
			for _, warning := range s.lastFit.Warnings {
				if actionableCampusWarning(warning) {
					text += "\n\n" + campusCell(warning)
				}
			}
			return text
		}
		return "尚未取得开课或课表查询结果。"
	}
	var b strings.Builder
	if r.Term.valid() {
		fmt.Fprintf(&b, "已查询 **%s 学年第 %d 学期**（%s），仅展示下沙校区。\n\n", r.Term.SchoolYear, r.Term.Semester, campusCell(r.TermSource))
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
					if !s.automaticMatching && origin != q.Query {
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
			if len(q.Classes) > 0 && q.Classes[0].CourseName != q.Query && q.Classes[0].CourseID != q.Query {
				b.WriteString("（名称相近，可能对应）")
			}
			b.WriteString("。\n\n")
			if s.lastFit != nil && len(q.Classes) > 0 {
				counts := map[string]int{}
				for _, o := range q.Classes {
					counts[fits[o.ClassID].Status]++
				}
				switch len(q.Classes) {
				case counts["outside_preferences"]:
					fmt.Fprintf(&b, "%s：查到本学期开课，但已查教学班均不符合指定时间偏好，并非确认未开课。\n\n", campusCell(q.Classes[0].CourseName))
				case counts["already_enrolled"]:
					fmt.Fprintf(&b, "%s：本人课表中已有该课程，不重复推荐其他教学班。\n\n", campusCell(q.Classes[0].CourseName))
				}
			}
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
				b.WriteString("；候选有 " + strings.Join(parts, "、") + "。请补充完整课程名或课程号后再核实，暂不猜测对应关系")
			} else if q.Warning == "" {
				b.WriteString("；请检查课名，提供完整官方课程名或课程号后再核实")
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
				status := "查到开课记录"
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
				fmt.Fprintf(&b, "| %s / %s | %s · %s学分 · %s | %s · %s %s | %s |\n", campusCell(o.CourseName), campusCell(o.CourseID), campusCell(o.Teacher), campusCell(o.Credit), campusCell(o.Examination), campusCell(o.ClassTime), campusCell(o.Campus), campusCell(o.Location), campusCell(status))
			}
		}
		b.WriteString("\n")
	}
	if s.lastFit != nil {
		f := s.lastFit
		b.WriteString("冲突按周次、星期、节次同时重叠判断；同一天同节次但周次完全错开，可以相容。\n\n")
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
				fmt.Fprintf(&b, "- %s（%s）：%s，%s\n", campusCell(o.CourseName), campusCell(o.Teacher), campusCell(o.ClassTime), campusCell(o.Location))
			}
			b.WriteString("\n每个课程号只选一个班；组合仅在本次已核实候选中选择。\n\n")
			if f.PlanLimited {
				b.WriteString("候选组合较多，本次已到计算上限；以上组合无冲突，但尚未确认最大门数。\n\n")
			}
		} else {
			b.WriteString("本次已核实候选中没有可确认同时放入的组合；排除或待确认原因见上文。\n\n")
		}
		// Preserve actionable auth/transport failures without exposing internal
		// tool protocol instructions in the product response.
		for _, warning := range f.Warnings {
			if actionableCampusWarning(warning) {
				b.WriteString(campusCell(warning) + "\n\n")
			}
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
	if !r.Term.valid() {
		for _, warning := range r.Warnings {
			b.WriteString(campusCell(warning) + "\n\n")
		}
	}
	b.WriteString("来源：校园教务查询。仅限本学期下沙；未检索到不等于未开课。未核实余量、选课资格、学分认定或考试安排。考查不代表免考；以上建议尚未执行选课。")
	return b.String()
}

func actionableCampusWarning(warning string) bool {
	for _, keyword := range []string{"授权", "超时", "限流", "无法连接", "服务处理请求失败", "数据格式无效", "时间预算", "默认学期"} {
		if strings.Contains(warning, keyword) {
			return true
		}
	}
	return false
}
