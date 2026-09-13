package agent

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jaketmoon/hdu-station/internal/storage"
)

type courseRequest struct {
	Boundary            string
	MatchSchedule       bool
	AllowedDays         []int
	AllowedSections     []int
	MaxCourses          int
	MinCourses          int
	CommunityOnly       bool
	OfficialOnly        bool
	RequireVerification bool
	EnrolledCourse      string
}

// This intentionally recognizes a small set of explicit constraints. Timetable
// occupancy still comes from the official tool, never from these text matches.
func parseCourseRequest(history []storage.Message) courseRequest {
	r := courseRequest{MaxCourses: 12}
	latest := ""
	for _, message := range history {
		if message.Role != "user" || (message.State != "" && message.State != "complete") {
			continue
		}
		latest = message.Content
	}
	if r.Boundary = requestBoundary(latest); r.Boundary != "" {
		return r
	}
	r.CommunityOnly = matches(`只查小红书|只看QQ|只查QQ|只看社区|只查社区|仅查社区|只看评价|只问评价|经验|体验|评价|口碑|怎么考核|如何考核`, latest) && !matches(`推荐|开课|开不开|有没有开|这学期|本学期|课表|冲突|能不能选|能选吗|筛选|插空|能放|塞`, latest)
	for _, message := range history {
		if message.Role != "user" || (message.State != "" && message.State != "complete") {
			continue
		}
		q := message.Content
		if matches(`课表|插空|插进|撞|冲突|塞|空档|能放|能不能上|已选|再加.*班|有课.*还能`, q) {
			r.MatchSchedule = true
		}
		if matches(`一个班|二选一`, q) {
			r.MinCourses, r.MaxCourses = 1, 1
		} else if count := regexp.MustCompile(`([一二两三四五六七八九十0-9]+)门`).FindStringSubmatch(q); len(count) > 0 {
			n, _ := strconv.Atoi(count[1])
			if n == 0 {
				n = map[string]int{"一": 1, "二": 2, "两": 2, "两三": 3, "二三": 3, "三": 3, "四": 4, "五": 5, "六": 6, "七": 7, "八": 8, "九": 9, "十": 10, "十一": 11, "十二": 12}[count[1]]
			}
			if n > 0 && n <= 12 {
				r.MinCourses, r.MaxCourses = n, n
				if count[1] == "两三" || count[1] == "二三" {
					r.MinCourses = 2
				}
			}
		}
		days, sections, noDays, noSections := []int{}, []int{}, []int{}, []int{}
		ambiguous := false
		for _, clause := range regexp.MustCompile(`[，,。？！!?；;]`).Split(q, -1) {
			// "Only Wednesday afternoon has class" states existing occupancy.
			if matches(`有课|已选|正在上|下午忙|上午忙|晚上忙`, clause) {
				continue
			}
			d := requestDays(clause)
			s := requestSections(clause)
			negative := matches(`不要|不排|别排|留空|空出来|全天空|整天.*空|不想去|不想上`, clause)
			if negative {
				if len(d) > 0 && len(s) > 0 {
					ambiguous = true // A day-specific exclusion cannot be a Cartesian filter.
				} else if len(d) > 0 {
					noDays = append(noDays, d...)
				} else {
					noSections = append(noSections, s...)
				}
				continue
			}
			if !matches(`只|想|空|考虑|找|安排|改成|能放|能加`, clause) {
				continue
			}
			if len(d) > 1 && len(regexp.MustCompile(`上午|早上|下午|晚上|晚课|第?[0-9]+(?:到|至|[-–—])[0-9]+节|第?[0-9]+节`).FindAllString(clause, -1)) > 1 {
				ambiguous = true
			}
			days, sections = append(days, d...), append(sections, s...)
		}
		if len(days) > 0 {
			r.AllowedDays = uniqueRequestNumbers(days)
		}
		if len(sections) > 0 {
			r.AllowedSections = uniqueRequestNumbers(sections)
		}
		if len(noDays) > 0 {
			r.AllowedDays = subtractRequestNumbers(r.AllowedDays, noDays, 7)
			ambiguous = ambiguous || len(r.AllowedDays) == 0
		}
		if len(noSections) > 0 {
			r.AllowedSections = subtractRequestNumbers(r.AllowedSections, noSections, 14)
			ambiguous = ambiguous || len(r.AllowedSections) == 0
		}
		if len(days)+len(sections)+len(noDays)+len(noSections) > 0 {
			r.MatchSchedule = true
		}
		if message.Content == latest && ambiguous {
			r.Boundary = "时间要求存在矛盾，或包含不同星期各自的时段。请给出一组明确的可选星期和共同节次，例如“只考虑周二、周四第6–9节”；我会再核对本人课表。"
		}
	}
	if matches(`(?:不用|不要|不需要|别)(?:再)?(?:查|读|看|结合|参考)(?:我的|本人)?课表`, latest) {
		r.MatchSchedule = false
	}
	// A narrow shortcut for explicit official-only requests and named-course
	// timetable queries; uncertain or experiential requests retain all sources.
	explicitOfficial := matches(`只(?:查|看|用|核对|核实)(?:教务|官方|开课|时间)|(?:不|不用|不要|别)(?:再)?(?:搜|搜索|查|看)(?:社区|网友)?(?:帖子|社区|评价)`, latest)
	enrolled := regexp.MustCompile(`已选的([^，。？！；]{4,})[，。？！；]`).FindStringSubmatch(latest)
	namedCourse := matches(`[A-Z][0-9]{7,11}|\p{Han}{2,}(?:鉴赏|赏析|导论|实践课)`, latest) || len(enrolled) > 1
	communityIntent := matches(`推荐|评价|体验|口碑|经验|怎么样|有趣|轻松|太累|水课|作业|给分|绩点|满绩|好过|考核|考试|论文|展示|上台|签到|难|基础`, latest)
	officialIntent := r.MatchSchedule || matches(`开课|哪些班|哪个班|开不开|开吗|上课时间|哪个学期`, latest)
	r.OfficialOnly = explicitOfficial || (officialIntent && !r.CommunityOnly && namedCourse && !communityIntent)
	r.RequireVerification = r.OfficialOnly && namedCourse
	if r.OfficialOnly && len(enrolled) > 1 {
		r.EnrolledCourse = strings.TrimSpace(enrolled[1])
	}
	return r
}

func requestBoundary(q string) string {
	for _, rule := range []struct{ pattern, answer string }{
		{`下学期|下个学期|下一学期|上学期|上个学期|往年|去年|明年|20[0-9]{2}\s*[-–—/]\s*20[0-9]{2}|20[0-9]{2}学年`, "目前只核实教务默认的本学期、下沙校区开课，不预测下学期，也不查询指定或历史学期。请改问本学期的课程。"},
		{`青山湖|文一校区|东岳校区`, "目前只筛选下沙校区课程，不能核实其他校区是否适合。请在教务系统按你的校区查询。"},
		{`(?:课表|什么时候有课).*(?:不用|不要|不需要)推荐|(?:不用|不要|不需要)推荐.*课表|只(?:看|查|显示)(?:一下|我的|本人|本学期|完整)*课表`, "目前课表读取用于筛选候选课程，暂不提供独立课表展示。请在教务系统查看完整课表；需要选课时可给我课程名称。"},
		{`名额|余量|捡漏|剩余人数`, "目前不能查询实时选课余量或捡漏，只能核实本学期下沙的开课记录和课表冲突。名额请以教务选课页面为准。"},
		{`考试.*(?:撞|冲突|时间|哪天)|(?:撞|冲突).*考试`, "上课不冲突不代表一定能选。我目前不查询考试安排，也不核验选课资格；考试冲突和资格请在教务系统确认。"},
		{`退掉|退课|代选|抢课|直接.*选上|帮我.*选上`, "我只能读取开课信息和课表，不能代选、抢课或退课。请在教务系统操作；我可以先核实本学期下沙课程的时间。"},
		{`学分.*(?:认定|缺口)|补.*学分|培养方案|算人文经典|课程类别|什么类别|课程性质|(?:专业|资格).*(?:能不能选|可选|能选吗)`, "目前不核验课程类别、培养方案学分认定或专业选课资格。开课和时间相容不能代替这些条件，请向学院或教务系统确认。"},
		{`考查.*(?:是不是|等于|不用|免考)`, "“考查”不等于免考试、免作业，也不能据此认定只交论文。当前工具不能核实本学期具体考核要求，应以任课教师通知或课程大纲为准；往届帖子只能作为经验参考。"},
		{`(?:不用|不要|不想|免|不做|不考)[^，。！？]{0,6}(?:闭卷|小组展示|上台|考试)|交论文就行`, "目前不能核实本学期是否免闭卷、免小组展示、免上台或只交论文，因此暂不按这些条件推荐或筛选。往届的签到、论文、给分经验不能推出本学期没有展示或考试。请以任课教师或课程大纲为准；我仍可核实本学期下沙开课与课表冲突。"},
		{`[零二两三四五六七八九十0-9]+点|一点(?:钟|半|以后|之后|前|后)|(?:下午|上午|晚上|中午|凌晨)一点|[0-9]{1,2}[:：][0-9]{2}`, "目前按教务节次筛选，不能可靠地把钟点换算为节次。请先确认校历中的作息表，再告诉我可用的星期和节次，例如“周二第8–9节”。"},
		{`忘了.*(?:全名|名字|课名)|(?:说|叫)[「“]?[^，。]{1,3}[」”]?(?:这课|很水|这门课).*正式课名`, "这个描述不足以可靠定位课程。请补充较完整的课程名、课程号，或至少一个明确的课程关键词；过于模糊的昵称暂不自动猜测。"},
		{`第[一二三四五六七八九十0-9]+个班`, "为避免把班级顺序对应错，请提供课程名称，以及老师或上课时间，例如“影视音乐赏析，周二第8–9节”。我会据此核实课表冲突。"},
		{`(?:三个班|多个班|几个班).*凑.*学分`, "同一课程的不同教学班不能当作多门课累加学分；选课建议中每个课程号最多保留一个班。具体学分认定请以培养方案和教务规则为准。"},
	} {
		if matches(rule.pattern, q) {
			return rule.answer
		}
	}
	return ""
}

func requestDays(q string) []int {
	result := []int{}
	for _, m := range regexp.MustCompile(`(?:周|星期|礼拜)([一二三四五六日天1-7])`).FindAllStringSubmatch(q, -1) {
		d := strings.Index("一二三四五六日", m[1])
		if m[1] == "天" {
			result = append(result, 7)
		} else if d >= 0 {
			result = append(result, d/3+1)
		} else if n, err := strconv.Atoi(m[1]); err == nil {
			result = append(result, n)
		}
	}
	if matches(`周末|周六日|星期六日`, q) {
		result = append(result, 6, 7)
	}
	return uniqueRequestNumbers(result)
}

func requestSections(q string) []int {
	result := []int{}
	for _, m := range regexp.MustCompile(`第?([0-9]{1,2}(?:、[0-9]{1,2})*)(?:(?:到|至|[-–—~～])第?([0-9]{1,2}))?节`).FindAllStringSubmatch(q, -1) {
		if m[2] != "" {
			a, _ := strconv.Atoi(m[1])
			b, _ := strconv.Atoi(m[2])
			if a >= 1 && b <= 14 && a <= b {
				result = append(result, requestRange(a, b)...)
			}
		} else {
			for _, text := range strings.Split(m[1], "、") {
				n, _ := strconv.Atoi(text)
				if n >= 1 && n <= 14 {
					result = append(result, n)
				}
			}
		}
	}
	if len(result) > 0 {
		return uniqueRequestNumbers(result)
	}
	for _, part := range []struct {
		pattern     string
		first, last int
	}{{`早上|上午|早课`, 1, 5}, {`下午`, 6, 9}, {`晚上|晚课|晚间`, 10, 14}} {
		if matches(part.pattern, q) {
			result = append(result, requestRange(part.first, part.last)...)
		}
	}
	return uniqueRequestNumbers(result)
}

func matches(pattern, text string) bool { return regexp.MustCompile(pattern).MatchString(text) }
func requestRange(first, last int) []int {
	r := []int{}
	for n := first; n <= last; n++ {
		r = append(r, n)
	}
	return r
}
func uniqueRequestNumbers(numbers []int) []int {
	seen, result := map[int]bool{}, []int{}
	for _, n := range numbers {
		if !seen[n] {
			seen[n] = true
			result = append(result, n)
		}
	}
	sort.Ints(result)
	return result
}
func subtractRequestNumbers(allowed, excluded []int, max int) []int {
	if len(allowed) == 0 {
		allowed = requestRange(1, max)
	}
	result := []int{}
	for _, n := range allowed {
		found := false
		for _, x := range excluded {
			found = found || n == x
		}
		if !found {
			result = append(result, n)
		}
	}
	return result
}
