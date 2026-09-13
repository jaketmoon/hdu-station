package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jaketmoon/hdu-station/internal/campusauth"
)

const campusRoot = "https://api.hduhelp.com/hduhelp-neo"

type CampusCredentials interface {
	AccessTokenFor(context.Context, string) (string, error)
}

// CampusClient exposes only these fixed GET routes. No CLI, arbitrary URL,
// student selector, grade endpoint or business write is available to the Agent.
type CampusClient struct {
	credentials CampusCredentials
	http        *http.Client
}

func NewCampusClient(credentials CampusCredentials) *CampusClient {
	return &CampusClient{credentials: credentials, http: sourceHTTPClient(25*time.Second, false)}
}

type campusEnvelope struct {
	Code       *int            `json:"code"`
	Data       json.RawMessage `json:"data"`
	Pagination *campusPage     `json:"pagination"`
}
type campusPage struct {
	valid      bool
	Total      int  `json:"total"`
	Limit      int  `json:"limit"`
	Offset     int  `json:"offset"`
	HasMore    bool `json:"hasMore"`
	NextOffset int  `json:"nextOffset"`
}

func (p *campusPage) UnmarshalJSON(data []byte) error {
	type page campusPage
	var value page
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*p = campusPage(value)
	for _, key := range []string{"total", "limit", "offset", "hasMore"} {
		if len(fields[key]) == 0 || string(fields[key]) == "null" {
			return nil
		}
	}
	p.valid = true
	return nil
}

func (c *CampusClient) get(ctx context.Context, path string, query url.Values, out any) (*campusPage, error) {
	scope := campusauth.CourseScope
	switch path {
	case "/academic/config":
		scope = ""
	case "/academic/class/search", "/academic/class/map":
	case "/academic/schedule":
		scope = campusauth.ScheduleScope
	default:
		return nil, errSourceResponse
	}
	headers := http.Header{"Accept": {"application/json"}}
	if scope != "" {
		if c.credentials == nil {
			return nil, campusauth.ErrLoginRequired
		}
		token, err := c.credentials.AccessTokenFor(ctx, scope)
		if err != nil {
			return nil, err
		}
		headers.Set("Authorization", "Bearer "+token)
	}
	var envelope campusEnvelope
	err := sourceJSON(ctx, c.http, http.MethodGet, campusRoot+path+"?"+query.Encode(), nil, headers, &envelope)
	if err != nil {
		return nil, err
	}
	if envelope.Code == nil {
		return nil, errSourceResponse
	}
	switch *envelope.Code {
	case 0:
	case 401:
		return nil, campusauth.ErrLoginRequired
	case 403:
		return nil, campusauth.ErrScope
	case 429:
		return nil, errSourceRateLimit
	default:
		return nil, errSourceServer
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" || json.Unmarshal(envelope.Data, out) != nil {
		return nil, errSourceResponse
	}
	return envelope.Pagination, nil
}

func campusError(err error) string {
	switch {
	case errors.Is(err, campusauth.ErrLoginRequired):
		return "校园尚未登录或授权已过期，请在助手设置的 HDU CLI 登录中完成网页授权。"
	case errors.Is(err, campusauth.ErrScope), errors.Is(err, errSourceAuth):
		return "校园授权无效或缺少读取权限，请在助手设置的 HDU CLI 登录中重新授权课程信息和本人课表。"
	case errors.Is(err, context.Canceled):
		return "校园查询已取消。"
	case errors.Is(err, context.DeadlineExceeded):
		return "校园查询超过本轮时间预算，尚未完成核实。"
	default:
		return sourceError("校园教务", err).Error()
	}
}

type AcademicTerm struct {
	SchoolYear string `json:"schoolYear"`
	Semester   int    `json:"semester"`
}

var schoolYearPattern = regexp.MustCompile(`^(20[0-9]{2})-(20[0-9]{2})$`)

func (t AcademicTerm) valid() bool {
	m := schoolYearPattern.FindStringSubmatch(t.SchoolYear)
	if m == nil || t.Semester < 1 || t.Semester > 3 {
		return false
	}
	a, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[2])
	return b == a+1
}
func (t AcademicTerm) query() url.Values {
	return url.Values{"schoolYear": {t.SchoolYear}, "semester": {strconv.Itoa(t.Semester)}}
}

type OfferingInput struct {
	Courses      []string        `json:"courses" jsonschema:"description=本次全部具体课程名（1–12门），追问从上一轮推荐提取，不编造别名"`
	CoreKeywords []CourseKeyword `json:"coreKeywords,omitempty" jsonschema:"description=可选：为模糊网名提取有辨识度的核心词。工具先查完整名称，没有精确匹配才查核心词；每门最多一个"`
	CourseIDs    []string        `json:"courseIDs,omitempty" jsonschema:"description=从工具候选中选择最接近的原始课程号；多个近似课程可同时保留。省略时不替模型选择模糊候选，不得编造课程号"`
	SchoolYear   string          `json:"schoolYear,omitempty" jsonschema:"description=明确学年时传 YYYY-YYYY，必须与 semester 同传；本学期省略由教务配置确定"`
	Semester     int             `json:"semester,omitempty" jsonschema:"description=学期1/2/3，必须与 schoolYear 同传"`
}
type CourseKeyword struct {
	Course  string `json:"course" jsonschema:"description=对应 courses 中的原始课程名"`
	Keyword string `json:"keyword" jsonschema:"description=该课程名的核心词（2–30字）"`
}
type CourseCandidate struct {
	CourseID   string     `json:"courseID"`
	CourseName string     `json:"courseName"`
	Classes    []Offering `json:"classes"`
}

type Offering struct {
	ClassID      string       `json:"classID"`
	CourseID     string       `json:"courseID"`
	CourseName   string       `json:"courseName"`
	Teacher      string       `json:"teacher"`
	Credit       string       `json:"credit"`
	Campus       string       `json:"campus"`
	Location     string       `json:"location"`
	ClassTime    string       `json:"classTime"`
	Examination  string       `json:"examination"`
	Times        []CourseTime `json:"times,omitempty"`
	TimeComplete bool         `json:"timeComplete"`
}
type OfferingQuery struct {
	Query          string            `json:"query"`
	Classes        []Offering        `json:"classes"`
	Candidates     []CourseCandidate `json:"candidates,omitempty"`
	Searched       []string          `json:"searched"`
	Partial        bool              `json:"partial"`
	Warning        string            `json:"warning,omitempty"`
	NeedsSelection bool              `json:"needsSelection"`
}
type OfferingResult struct {
	Term       AcademicTerm    `json:"term"`
	TermSource string          `json:"termSource"`
	CheckedAt  string          `json:"checkedAt"`
	Queries    []OfferingQuery `json:"queries"`
	Warnings   []string        `json:"warnings,omitempty"`
}

// State and request cache live for one answer only, never in SQLite.
type CampusSession struct {
	client            *CampusClient
	progress          func(string)
	mu                sync.Mutex
	requests          int
	Calls             int
	term              *AcademicTerm
	dictionary        map[string]map[string]string
	schedules         map[AcademicTerm]scheduleResult
	queries           map[string]OfferingQuery
	requiredTool      string
	lastInput         OfferingInput
	lastOfferings     *OfferingResult
	lastFit           *FitCoursesResult
	scheduleRequested bool
	fitPreferences    FitCoursesInput
	origins           map[string][]string
}

// While fuzzy matches remain, the next model turn must select official course
// IDs through the same read tool before it can describe them as checked.
func (s *CampusSession) RequiredTool() string { return s.requiredTool }
func (s *CampusSession) HasCourseResults() bool {
	return s.lastOfferings != nil && len(s.lastOfferings.Queries) > 0
}

func (s *CampusSession) CanShowCourses() bool {
	if !s.HasCourseResults() {
		return false
	}
	for _, q := range s.lastOfferings.Queries {
		if q.NeedsSelection {
			return false
		}
	}
	return true
}

func (s *CampusSession) CompletionGap() string {
	if s.Calls == 0 {
		return ""
	}
	if !s.HasCourseResults() {
		return "尚未完成具体候选课程核实。读取本人课表只是准备步骤；若用户已给课名或历史里已有候选，沿用它们，传 courses 查询开课并检查时间。否则先从已连接社区查找候选。不能仅输出忙闲时段、内部说明或声称推荐已经完成。"
	}
	if !s.CanShowCourses() {
		return "仍有模糊课程候选未匹配：先选择课程号，再完成核实。"
	}
	return "核实结果已就绪，请调用 show_course_results 展示具体课程和排除原因。"
}
func (s *CampusSession) requireSelection(r OfferingResult, tool string) {
	s.requiredTool = ""
	for _, q := range r.Queries {
		if q.NeedsSelection {
			s.requiredTool = tool
			return
		}
	}
	if len(r.Queries) > 0 {
		s.requiredTool = "show_course_results"
	}
}

func NewCampusSession(client *CampusClient, progress func(string)) *CampusSession {
	if progress == nil {
		progress = func(string) {}
	}
	return &CampusSession{client: client, progress: progress, schedules: map[AcademicTerm]scheduleResult{}, queries: map[string]OfferingQuery{}, origins: map[string][]string{}}
}

func (s *CampusSession) get(ctx context.Context, path string, q url.Values, out any) (*campusPage, error) {
	s.mu.Lock()
	if s.requests >= 40 {
		s.mu.Unlock()
		return nil, context.DeadlineExceeded
	}
	s.requests++
	s.mu.Unlock()
	if s.client == nil {
		return nil, campusauth.ErrLoginRequired
	}
	return s.client.get(ctx, path, q, out)
}

func (s *CampusSession) resolveTerm(ctx context.Context, in OfferingInput) (AcademicTerm, string, error) {
	t := AcademicTerm{in.SchoolYear, in.Semester}
	if t.SchoolYear != "" || t.Semester != 0 {
		if !t.valid() {
			return t, "", errors.New("请同时提供有效学年（如2026-2027）和学期（1、2或3）。")
		}
		return t, "本次指定的查询学期", nil
	}
	if s.term != nil {
		return *s.term, "教务配置的当前默认学期", nil
	}
	var cfg struct {
		Course struct {
			SchoolYear string     `json:"schoolYear"`
			Semester   scalarText `json:"semester"`
		} `json:"courseQueryDefault"`
		Schedule struct {
			SchoolYear string     `json:"schoolYear"`
			Semester   scalarText `json:"semester"`
		} `json:"scheduleDefault"`
	}
	if _, err := s.get(ctx, "/academic/config", nil, &cfg); err != nil {
		return t, "", errors.New(campusError(err))
	}
	n, _ := strconv.Atoi(string(cfg.Schedule.Semester))
	t = AcademicTerm{cfg.Schedule.SchoolYear, n}
	if !t.valid() || cfg.Course.SchoolYear != t.SchoolYear || string(cfg.Course.Semester) != strconv.Itoa(t.Semester) {
		return AcademicTerm{}, "", errors.New("教务开课与课表默认学期缺失或不一致，请明确要查询的学年和学期。")
	}
	s.term = &t
	return t, "教务配置的当前默认学期", nil
}

type scalarText string

func (v *scalarText) UnmarshalJSON(data []byte) error {
	var str string
	if json.Unmarshal(data, &str) == nil {
		*v = scalarText(str)
		return nil
	}
	var n json.Number
	if string(data) == "null" || json.Unmarshal(data, &n) != nil {
		return errSourceResponse
	}
	*v = scalarText(n.String())
	return nil
}

func cleanCampusText(v string, max int) string {
	v = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' {
			return -1
		}
		return r
	}, v)
	r := []rune(strings.TrimSpace(v))
	if len(r) > max {
		r = r[:max]
	}
	return string(r)
}

func validCourses(names []string, allowEmpty bool) ([]string, error) {
	if len(names) > 12 || (!allowEmpty && len(names) == 0) {
		return nil, errors.New("请一次传入1–12门具体课程；更多课程需分批核实。")
	}
	result := []string{}
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if len([]rune(name)) < 2 || len([]rune(name)) > 80 || strings.ContainsAny(name, "\n\r\x00") {
			return nil, errors.New("课程名须为2–80字的单行完整名称。")
		}
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	return result, nil
}

func (s *CampusSession) CheckOfferings(ctx context.Context, in OfferingInput) (OfferingResult, error) {
	s.Calls++
	s.requiredTool = ""
	ctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	r, err := s.offerings(ctx, in)
	s.lastInput = in
	s.lastOfferings = &r
	s.lastFit = nil
	s.requireSelection(r, "check_course_offerings")
	return r, err
}
func (s *CampusSession) offerings(ctx context.Context, in OfferingInput) (OfferingResult, error) {
	result := OfferingResult{CheckedAt: time.Now().Format(time.RFC3339), Queries: []OfferingQuery{}, Warnings: []string{"仅核实该学期检索到的开课记录，不代表仍有余量、符合选课资格或已完成选课。检索分页覆盖未确认，未检索到不等于未开课。"}}
	names, err := validCourses(in.Courses, false)
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		return result, nil
	}
	keywords, err := lookupKeywords(names, in.CoreKeywords, in.CourseIDs)
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		return result, nil
	}
	result.Term, result.TermSource, err = s.resolveTerm(ctx, in)
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		return result, nil
	}
	if s.dictionary == nil {
		var dictionary map[string]map[string]string
		if _, err := s.get(ctx, "/academic/class/map", nil, &dictionary); err == nil {
			s.dictionary = dictionary
		}
	}
	s.progress("正在核实目标学期的开课班级…")
	// At most three requests in flight. Each name keeps its own result and error.
	result.Queries = make([]OfferingQuery, len(names))
	var wg sync.WaitGroup
	gate := make(chan struct{}, 3)
	for i, name := range names {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			select {
			case gate <- struct{}{}:
				defer func() { <-gate }()
			case <-ctx.Done():
				result.Queries[i] = OfferingQuery{Query: name, Classes: []Offering{}, Partial: true, Warning: campusError(ctx.Err())}
				return
			}
			result.Queries[i] = selectCourseIDs(s.searchOffering(ctx, result.Term, name, keywords[name]), in.CourseIDs)
		}(i, name)
	}
	wg.Wait()
	if errors.Is(ctx.Err(), context.Canceled) {
		return result, ctx.Err()
	}
	return result, nil
}

func lookupKeywords(names []string, hints []CourseKeyword, ids []string) (map[string]string, error) {
	if len(hints) > 12 || len(ids) > 24 {
		return nil, errors.New("核心词最多12个，选定课程号最多24个。")
	}
	result := map[string]string{}
	for _, name := range names {
		core := strings.Trim(name, "《》「」")
		for _, suffix := range []string{"导论", "概论", "鉴赏", "赏析", "入门", "基础"} {
			if strings.HasSuffix(core, suffix) && len([]rune(strings.TrimSuffix(core, suffix))) >= 2 {
				core = strings.TrimSuffix(core, suffix)
				break
			}
		}
		if core == name && len([]rune(core)) > 4 {
			core = string([]rune(core)[:4])
		}
		result[name] = core
	}
	seen := map[string]bool{}
	for _, hint := range hints {
		_, ok := result[hint.Course]
		word := strings.TrimSpace(hint.Keyword)
		if !ok || seen[hint.Course] || len([]rune(word)) < 2 || len([]rune(word)) > 30 || strings.ContainsAny(word, "\n\r\x00") {
			return nil, errors.New("每门课程的核心词须唯一、为2–30字，且对应本次课程名。")
		}
		result[hint.Course] = word
		seen[hint.Course] = true
	}
	for _, id := range ids {
		if id == "" || len(id) > 128 || strings.ContainsAny(id, " \t\n\r\x00") {
			return nil, errors.New("课程号须使用工具提供的完整原始值。")
		}
	}
	return result, nil
}

// Selection changes no identity: it only promotes classes already returned by
// the official query under an explicitly selected course ID.
func selectCourseIDs(query OfferingQuery, ids []string) OfferingQuery {
	if len(ids) == 0 {
		query.NeedsSelection = len(query.Classes) == 0 && len(query.Candidates) > 0
		if query.NeedsSelection {
			query.Warning = "名称尚未选定：请根据原名和上下文选最接近的候选课程号（相似者可选多个），将 courseIDs 传入同一工具继续核实；尚未筛选不表示冲突或不可放入。"
		}
		return query
	}
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	matches := []Offering{}
	for _, o := range query.Classes {
		if selected[o.CourseID] {
			matches = append(matches, o)
		}
	}
	for _, candidate := range query.Candidates {
		if selected[candidate.CourseID] {
			matches = append(matches, candidate.Classes...)
		}
	}
	query.Classes = matches
	if len(matches) == 0 {
		query.Warning = "选定课程号未在本次检索结果中找到；不能按名称猜测替代。"
	}
	return query
}

func (s *CampusSession) searchOffering(ctx context.Context, term AcademicTerm, name, core string) OfferingQuery {
	key := term.SchoolYear + "/" + strconv.Itoa(term.Semester) + "/" + name + "/" + core
	s.mu.Lock()
	cached, ok := s.queries[key]
	s.mu.Unlock()
	if ok {
		return cached
	}
	result := OfferingQuery{Query: name, Classes: []Offering{}, Partial: true, Searched: []string{}}
	seen := map[string]bool{}
	candidateIndex := map[string]int{}
	// Full-name pages first. Only a missing exact match enables the one-page
	// core-word fallback in the same term. Never search historical semesters.
	for stage, word := range []string{name, core} {
		if stage == 1 && (len(result.Classes) > 0 || core == name || result.Warning != "") {
			break
		}
		result.Searched = append(result.Searched, word)
		pages := 2
		if stage == 1 {
			pages = 1
		}
		for page := 0; page < pages; page++ {
			q := term.query()
			q.Set("query", word)
			q.Set("size", "20")
			q.Set("from", strconv.Itoa(page*20))
			var data struct {
				Classes []struct {
					ClassID     string     `json:"classID"`
					CourseID    string     `json:"courseID"`
					CourseName  string     `json:"courseName"`
					Teacher     string     `json:"teacherName"`
					Credit      scalarText `json:"credit"`
					CampusID    string     `json:"campusID"`
					Location    string     `json:"location"`
					ClassTime   string     `json:"classTime"`
					Examination string     `json:"examinationMethod"`
				} `json:"classes"`
			}
			if _, err := s.get(ctx, "/academic/class/search", q, &data); err != nil {
				result.Warning = campusError(err)
				break
			}
			if data.Classes == nil || len(data.Classes) > 20 {
				result.Warning = "开课查询返回的班级列表格式无效。"
				break
			}
			for _, r := range data.Classes {
				if r.ClassID == "" || r.CourseID == "" || r.CourseName == "" || len(r.ClassID) > 128 || len(r.CourseID) > 128 {
					result.Warning = "部分班级缺少课程号或班级号，已跳过。"
					continue
				}
				id := r.ClassID + "/" + r.CourseID
				if seen[id] {
					continue
				}
				seen[id] = true
				o := Offering{ClassID: r.ClassID, CourseID: r.CourseID, CourseName: cleanCampusText(r.CourseName, 100), Teacher: cleanCampusText(r.Teacher, 100), Credit: cleanCampusText(string(r.Credit), 20), Campus: cleanCampusText(r.CampusID, 30), Location: cleanCampusText(r.Location, 120), ClassTime: cleanCampusText(r.ClassTime, 1000), Examination: cleanCampusText(r.Examination, 60)}
				if value := s.dictionary["campusID"][r.CampusID]; value != "" {
					o.Campus = cleanCampusText(value, 50)
				}
				if value := s.dictionary["examinationMethod"][r.Examination]; value != "" {
					o.Examination = cleanCampusText(value, 60)
				}
				o.Times, o.TimeComplete = parseCourseTimes(r.ClassTime)
				if strings.TrimSpace(r.CourseName) == name {
					result.Classes = append(result.Classes, o)
				} else {
					originKey := term.SchoolYear + "/" + strconv.Itoa(term.Semester) + "/" + r.CourseID
					s.mu.Lock()
					found := false
					for _, old := range s.origins[originKey] {
						if old == name {
							found = true
						}
					}
					if !found {
						s.origins[originKey] = append(s.origins[originKey], name)
					}
					s.mu.Unlock()
					index, ok := candidateIndex[r.CourseID]
					if !ok {
						index = len(result.Candidates)
						candidateIndex[r.CourseID] = index
						result.Candidates = append(result.Candidates, CourseCandidate{CourseID: r.CourseID, CourseName: o.CourseName, Classes: []Offering{}})
					}
					result.Candidates[index].Classes = append(result.Candidates[index].Classes, o)
				}
			}
			if len(data.Classes) < 20 {
				break
			}
		}
	}
	if result.Warning == "" {
		s.mu.Lock()
		s.queries[key] = result
		s.mu.Unlock()
	}
	return result
}
