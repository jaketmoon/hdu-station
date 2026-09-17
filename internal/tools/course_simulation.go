package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/jaketmoon/hdu-station/internal/campusauth"
)

type SimulationSlot struct {
	Week    int `json:"week"`
	Day     int `json:"weekday"`
	Section int `json:"section"`
}
type SimulationCourse struct {
	ClassID       string           `json:"classID"`
	CourseID      string           `json:"courseID"`
	CourseName    string           `json:"courseName"`
	Teacher       string           `json:"teacherName"`
	ClassTime     string           `json:"classTime"`
	State         string           `json:"state"`
	ScheduleKnown bool             `json:"scheduleKnown"`
	Slots         []SimulationSlot `json:"slots,omitempty"`
}
type SimulationItem struct {
	ClassID string `json:"classID"`
	Intent  string `json:"intent" jsonschema:"enum=ENROLL,enum=DROP,description=ENROLL模拟加入或恢复真实课程；DROP仅模拟移除真实已选课程"`
}
type SimulationData struct {
	SchemaVersion    int                `json:"schemaVersion"`
	SchoolYear       string             `json:"schoolYear"`
	Semester         int                `json:"semester"`
	Revision         int64              `json:"revision"`
	ActualCourses    []SimulationCourse `json:"actualCourses"`
	SimulationItems  []SimulationItem   `json:"simulationItems"`
	EffectiveCourses []SimulationCourse `json:"effectiveCourses"`
}
type SimulationInput struct {
	Action             string           `json:"action" jsonschema:"enum=read,enum=update,enum=replace,enum=reset,description=read查看；update局部增删改保留其他条目；replace完整替换；reset清空模拟修改恢复真实课表。后两者仅限用户明确要求"`
	SchoolYear         string           `json:"schoolYear,omitempty"`
	Semester           int              `json:"semester,omitempty"`
	ExpectedRevision   *int64           `json:"expectedRevision,omitempty" jsonschema:"description=写操作必填本轮read返回的revision；冲突不得擅自重试"`
	Items              []SimulationItem `json:"items,omitempty" jsonschema:"description=新增或修改的已核实教学班意图；replace时为完整目标方案"`
	RemoveClassIDs     []string         `json:"removeClassIDs,omitempty" jsonschema:"description=update时取消这些已有模拟操作，既可移除模拟加入，也可撤销模拟退课"`
	RequireNoConflicts bool             `json:"requireNoConflicts,omitempty" jsonschema:"description=用户要求无冲突时true，宿主按修改后的模拟课表检查全部周次节次；时间不完整则不写"`
}
type SimulationResult struct {
	Status   string          `json:"status"`
	Message  string          `json:"message"`
	NextStep string          `json:"nextStep,omitempty"`
	Data     *SimulationData `json:"data,omitempty"`
}

func (s *CampusSession) readSimulation(ctx context.Context, term AcademicTerm, headers http.Header) (*SimulationData, error) {
	var e struct {
		Code *int            `json:"code"`
		Data *SimulationData `json:"data"`
	}
	err := sourceJSON(ctx, s.client.http, http.MethodGet, campusRoot+"/academic/course-simulation?"+term.query().Encode(), nil, headers, &e)
	if err != nil {
		return nil, err
	}
	d := e.Data
	if e.Code == nil || *e.Code != 0 || d == nil || d.SchemaVersion != 1 || d.Revision < 1 || d.SchoolYear != term.SchoolYear || d.Semester != term.Semester || d.ActualCourses == nil || d.EffectiveCourses == nil || d.SimulationItems == nil {
		return nil, errSourceResponse
	}
	for _, courses := range [][]SimulationCourse{d.ActualCourses, d.EffectiveCourses} {
		seen := map[string]bool{}
		for _, o := range courses {
			if o.ClassID == "" || seen[o.ClassID] {
				return nil, errSourceResponse
			}
			seen[o.ClassID] = true
		}
	}
	seen := map[string]bool{}
	for _, i := range d.SimulationItems {
		if i.ClassID == "" || seen[i.ClassID] || (i.Intent != "ENROLL" && i.Intent != "DROP") {
			return nil, errSourceResponse
		}
		seen[i.ClassID] = true
	}
	return d, nil
}
func (s *CampusSession) ManageSimulation(ctx context.Context, in SimulationInput) (SimulationResult, error) {
	s.Calls++
	result := SimulationResult{Status: "not_written"}
	finish := func(message string, data *SimulationData) (SimulationResult, error) {
		if s.lastFit != nil && s.lastFit.ScheduleSource == "simulation" && (in.Action != "read" || (data != nil && data.Revision != s.lastFit.SimulationRevision)) {
			s.lastFit = nil
		}
		result.Message = message
		s.managementDisplay = message
		if data != nil {
			s.managementDisplay += "\n\n" + displaySimulation(data)
			copy := *data
			copy.ActualCourses = append([]SimulationCourse{}, data.ActualCourses...)
			copy.EffectiveCourses = append([]SimulationCourse{}, data.EffectiveCourses...)
			for i := range copy.ActualCourses {
				copy.ActualCourses[i].Slots = nil
			}
			for i := range copy.EffectiveCourses {
				copy.EffectiveCourses[i].Slots = nil
			}
			result.Data = &copy
		}
		return result, nil
	}
	if in.Action != "read" && in.Action != "update" && in.Action != "replace" && in.Action != "reset" {
		return finish("未修改模拟课表：操作无效。", nil)
	}
	term, _, err := s.resolveTerm(ctx, OfferingInput{SchoolYear: in.SchoolYear, Semester: in.Semester})
	if err != nil {
		return finish("未读取或修改模拟课表："+err.Error(), nil)
	}
	writeScope := ""
	if in.Action != "read" {
		writeScope = campusauth.SimulationWriteScope
	}
	headers, err := s.managementHeaders(ctx, campusauth.SimulationReadScope, writeScope)
	if err != nil {
		return finish("模拟课表授权不可用，请在设置中授权模拟方案读写。", nil)
	}
	if in.Action != "read" && s.simulationBlocked {
		return finish("本轮模拟写入结果未确认或有版本冲突，已停止继续写入；请核对最新方案。", nil)
	}
	current, err := s.readSimulation(ctx, term, headers)
	if err != nil {
		return finish("无法完整读取模拟课表，未执行写入。", nil)
	}
	if in.Action == "read" {
		s.simulationSnapshot = current
		result.Status = "read"
		return finish("已读取 Neo 模拟课表，学校真实课表未修改。", current)
	}
	prior := s.simulationSnapshot
	if prior == nil || prior.SchoolYear != term.SchoolYear || prior.Semester != term.Semester || in.ExpectedRevision == nil {
		result.NextStep = "先调用read并使用返回的revision，再执行用户已授权的修改；无需重复询问。"
		return finish("未修改模拟课表：请先读取本轮目标学期的方案。", current)
	}
	if *in.ExpectedRevision != prior.Revision || current.Revision != prior.Revision {
		s.simulationBlocked = true
		result.Status = "conflict"
		return finish("模拟课表版本已变化，本次未覆盖；请核对最新方案后再修改。", current)
	}
	if len(in.Items) > 100 || len(in.RemoveClassIDs) > 100 || (in.Action != "update" && len(in.RemoveClassIDs) > 0) || (in.Action == "reset" && len(in.Items) > 0) {
		return finish("未修改模拟课表：参数不一致或超过100项。", current)
	}
	target := map[string]string{}
	if in.Action == "update" {
		for _, i := range current.SimulationItems {
			target[i.ClassID] = i.Intent
		}
	}
	for _, id := range in.RemoveClassIDs {
		if _, ok := target[id]; !ok {
			return finish("未修改模拟课表：取消目标不在当前模拟操作中。", current)
		}
		delete(target, id)
	}
	actual := map[string]bool{}
	for _, o := range current.ActualCourses {
		actual[o.ClassID] = true
	}
	known := map[string]Offering{}
	if s.lastOfferings != nil && s.lastOfferings.Term == term {
		for _, q := range s.lastOfferings.Queries {
			if !q.NeedsSelection {
				for _, o := range q.Classes {
					known[o.ClassID] = o
				}
			}
		}
	}
	currentEnroll := map[string]bool{}
	for _, i := range current.SimulationItems {
		if i.Intent == "ENROLL" {
			currentEnroll[i.ClassID] = true
		}
	}
	seen := map[string]bool{}
	for _, i := range in.Items {
		if i.ClassID == "" || seen[i.ClassID] {
			return finish("未修改模拟课表：存在重复或无效教学班。", current)
		}
		seen[i.ClassID] = true
		switch i.Intent {
		case "DROP":
			if !actual[i.ClassID] {
				return finish("未修改模拟课表：模拟退课只能针对当前真实课表中的课程。", current)
			}
		case "ENROLL":
			if _, ok := known[i.ClassID]; !ok && !actual[i.ClassID] && !currentEnroll[i.ClassID] {
				return finish("未修改模拟课表：新班级须先按目标学期核实开课。", current)
			}
		default:
			return finish("未修改模拟课表：只支持 ENROLL 或 DROP。", current)
		}
		target[i.ClassID] = i.Intent
		if i.Intent == "ENROLL" && actual[i.ClassID] {
			delete(target, i.ClassID)
		}
	}
	if len(target) > 100 {
		return finish("未修改模拟课表：完整方案超过100项。", current)
	}
	if in.RequireNoConflicts && !simulationFits(current, target, known) {
		return finish("未修改模拟课表：目标方案存在时间冲突或时间信息不完整。", current)
	}
	items := []SimulationItem{}
	for id, intent := range target {
		items = append(items, SimulationItem{id, intent})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ClassID < items[j].ClassID })
	if sameSimulation(items, current.SimulationItems) {
		result.Status = "unchanged"
		return finish("已核实：模拟课表已符合要求，无需重复写入。", current)
	}
	body, _ := json.Marshal(map[string]any{"schoolYear": term.SchoolYear, "semester": term.Semester, "expectedRevision": current.Revision, "items": items})
	var response struct {
		Code *int `json:"code"`
	}
	s.progress("正在更新 Neo 模拟方案并复查，学校真实课表保持不变…")
	writeErr := sourceJSON(ctx, s.client.http, http.MethodPut, campusRoot+"/academic/course-simulation", bytes.NewReader(body), headers, &response)
	after, readErr := s.readSimulation(ctx, term, headers)
	if writeErr == errSourceConflict {
		s.simulationBlocked = true
		result.Status = "conflict"
		return finish("模拟课表发生版本冲突，本次未覆盖且未自动重试。", after)
	}
	if readErr == nil && sameSimulation(items, after.SimulationItems) {
		s.simulationSnapshot = after
		result.Status = "confirmed"
		return finish("已复查：Neo 模拟课表已按要求更新，学校真实选课未改变。", after)
	}
	s.simulationBlocked = true
	result.Status = "unknown"
	return finish("模拟课表写入结果尚未确认，可能已生效；已停止自动重试，请核对最新方案。", after)
}
func sameSimulation(a, b []SimulationItem) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]string{}
	for _, i := range a {
		m[i.ClassID] = i.Intent
	}
	for _, i := range b {
		if m[i.ClassID] != i.Intent {
			return false
		}
	}
	return true
}
func simulationFits(d *SimulationData, target map[string]string, known map[string]Offering) bool {
	courses := map[string]SimulationCourse{}
	for _, o := range d.EffectiveCourses {
		if target[o.ClassID] == "ENROLL" {
			courses[o.ClassID] = o
		}
	}
	for _, o := range d.ActualCourses {
		if target[o.ClassID] != "DROP" {
			courses[o.ClassID] = o
		}
	}
	for id, intent := range target {
		if intent != "ENROLL" {
			continue
		}
		if _, ok := courses[id]; ok {
			continue
		}
		o, ok := known[id]
		if !ok {
			return false
		}
		c := SimulationCourse{ClassID: id, ScheduleKnown: o.TimeComplete}
		for _, t := range o.Times {
			for _, w := range t.Weeks {
				for _, sec := range t.Sections {
					c.Slots = append(c.Slots, SimulationSlot{w, t.Day, sec})
				}
			}
		}
		courses[id] = c
	}
	occupied := map[SimulationSlot]string{}
	for id, o := range courses {
		if !o.ScheduleKnown || len(o.Slots) == 0 {
			return false
		}
		for _, slot := range o.Slots {
			if slot.Week < 1 || slot.Day < 1 || slot.Day > 7 || slot.Section < 1 || slot.Section > 14 {
				return false
			}
			if other := occupied[slot]; other != "" && other != id {
				return false
			}
			occupied[slot] = id
		}
	}
	return true
}
func displaySimulation(d *SimulationData) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s 学年第%d学期 · 模拟课表**\n\n", campusCell(d.SchoolYear), d.Semester)
	b.WriteString("| 课程 | 老师 | 上课时间 | 状态 |\n| --- | --- | --- | --- |\n")
	actual := map[string]bool{}
	drops := map[string]bool{}
	for _, i := range d.SimulationItems {
		if i.Intent == "DROP" {
			drops[i.ClassID] = true
		}
	}
	for _, o := range d.ActualCourses {
		actual[o.ClassID] = true
		state := "真实已选，保留"
		if drops[o.ClassID] {
			state = "模拟移除，真实仍已选"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", campusCell(o.CourseName), campusCell(o.Teacher), campusCell(o.ClassTime), state)
	}
	for _, o := range d.EffectiveCourses {
		if actual[o.ClassID] {
			continue
		}
		fmt.Fprintf(&b, "| %s | %s | %s | 模拟加入 |\n", campusCell(o.CourseName), campusCell(o.Teacher), campusCell(o.ClassTime))
	}
	if len(d.ActualCourses) == 0 && len(d.EffectiveCourses) == 0 {
		b.WriteString("\n该学期模拟课表为空。\n")
	}
	target := map[string]string{}
	for _, i := range d.SimulationItems {
		target[i.ClassID] = i.Intent
	}
	if !simulationFits(d, target, nil) {
		b.WriteString("\n当前模拟课表存在时间冲突或时间信息不完整，不能确认无冲突。\n")
	}
	b.WriteString("\n仅为 Neo 模拟方案，不改变学校真实选课。")
	return b.String()
}
