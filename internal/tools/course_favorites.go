package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jaketmoon/hdu-station/internal/campusauth"
)

// The upstream endpoint replaces the whole list and has no revision/CAS.
// Serialize read/merge/write/verify across all Station conversations.
var favoriteGate = make(chan struct{}, 1)

type FavoriteInput struct {
	Action         string   `json:"action,omitempty" jsonschema:"enum=read,enum=rank,enum=add,enum=remove,enum=update,enum=replace,enum=clear,description=read查看收藏；rank排行；add添加；remove删除classIDs；update删除removeClassIDs同时添加classIDs；replace仅保留classIDs；clear清空，后两者必须用户明确要求"`
	RemoveClassIDs []string `json:"removeClassIDs,omitempty" jsonschema:"description=update时明确要移除的本轮收藏列表classID；其余收藏保留"`
	ClassIDs       []string `json:"classIDs,omitempty" jsonschema:"description=可信课程上下文或本轮工具返回的教学班classID，最多100个；同课允许多个班，全部收藏时传所有符合条件的班级。近似名称由模型判断对应后选择ID。"`
	RequireFit     bool     `json:"requireFit,omitempty" jsonschema:"description=用户要求无冲突或插空后收藏时必须true；班级必须来自最新suggestedPlan。只核实后收藏时false，不额外读课表。"`
}
type FavoriteEntry struct {
	ClassID  string `json:"classID"`
	FavCount int    `json:"favCount,omitempty"`
}
type FavoriteResult struct {
	CourseCount int             `json:"courseCount,omitempty"`
	ClassCount  int             `json:"classCount,omitempty"`
	AddedCount  int             `json:"addedCount,omitempty"`
	TotalCount  int             `json:"totalCount,omitempty"`
	Action      string          `json:"action,omitempty"`
	Entries     []FavoriteEntry `json:"entries,omitempty"`
	Status      string          `json:"status"`
	Message     string          `json:"message"`
	NextStep    string          `json:"nextStep,omitempty"`
	Courses     []Offering      `json:"courses,omitempty"`
}

func (s *CampusSession) AddFavorites(ctx context.Context, in FavoriteInput) (FavoriteResult, error) {
	in.Action = "add"
	return s.ManageFavorites(ctx, in)
}

func (s *CampusSession) ManageFavorites(ctx context.Context, in FavoriteInput) (FavoriteResult, error) {
	s.Calls++
	result := FavoriteResult{Status: "not_written", Action: in.Action}
	finish := func(message string) (FavoriteResult, error) {
		result.Message = message
		if result.Status == "confirmed" || result.Status == "already_present" {
			courses := map[string]bool{}
			for _, o := range result.Courses {
				courses[o.CourseID] = true
			}
			result.CourseCount, result.ClassCount = len(courses), len(result.Courses)
		}
		s.favoriteReceipts = append(s.favoriteReceipts, result)
		return result, nil
	}
	if in.Action == "read" || in.Action == "rank" {
		return s.readFavorites(ctx, in.Action, finish, &result)
	}
	if in.Action != "add" && in.Action != "remove" && in.Action != "update" && in.Action != "replace" && in.Action != "clear" {
		return finish("未写入收藏：请选择有效操作。")
	}
	if len(in.RemoveClassIDs) > 100 || len(in.ClassIDs) > 100 || (in.Action != "update" && len(in.RemoveClassIDs) > 0) || (in.Action == "clear" && len(in.ClassIDs) > 0) {
		return finish("未写入收藏：操作参数不一致或超过100项。")
	}
	if (in.Action == "add" || in.Action == "remove") && len(in.ClassIDs) == 0 {
		return finish("未写入收藏：请明确至少一个教学班。")
	}
	adding := in.Action == "add" || in.Action == "update" || in.Action == "replace"
	for _, receipt := range s.favoriteReceipts {
		if receipt.Status == "unknown" {
			return finish("收藏结果尚未确认：本轮有未确认的写入，不再自动重试；请稍后核对收藏。")
		}
	}
	if adding && len(in.ClassIDs) > 0 && s.lastOfferings == nil && len(s.favoriteSelection) == 0 {
		result.NextStep = "请立即按上下文课程、老师和时间调用 check_course_offerings 重新定位，再调用收藏；用户已授权，无需再询问是否重查。"
		return finish("未写入收藏：尚无可信的教学班记录。")
	}
	known := map[string]Offering{}
	for id := range s.favoriteSelection {
		o, ok := s.favoriteKnown[id]
		if !ok {
			o = Offering{ClassID: id, CourseName: "原有收藏（详情未取得）"}
		}
		known[id] = o
	}
	if s.lastOfferings != nil {
		for _, q := range s.lastOfferings.Queries {
			for _, o := range q.Classes {
				known[o.ClassID] = o
			}
			// Selecting a returned class ID is the model's name-match decision.
			for _, candidate := range q.Candidates {
				for _, o := range candidate.Classes {
					known[o.ClassID] = o
				}
			}
		}
	}
	plan := map[string]bool{}
	if s.lastFit != nil && s.lastFit.Offerings == s.lastOfferings {
		for _, id := range s.lastFit.SuggestedPlan {
			plan[id] = true
		}
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, id := range in.ClassIDs {
		o, ok := known[id]
		if adding && (!ok || id == "") {
			return finish("未写入收藏：包含未经工具核实的教学班，请先核实。")
		}
		if adding && in.RequireFit && !plan[id] {
			return finish("未写入收藏：所选班级不在最新核实的无冲突组合中，请先完成课表适配。")
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
			if adding {
				result.Courses = append(result.Courses, o)
			}
		}
	}
	if s.client == nil || s.client.credentials == nil {
		return finish("未写入收藏：请在设置中登录 HDU CLI 并授权课程收藏读写。")
	}
	select {
	case favoriteGate <- struct{}{}:
		defer func() { <-favoriteGate }()
	case <-ctx.Done():
		return finish("未写入收藏：等待已取消。")
	}
	// Resolve both scopes before reading; use one account credential throughout.
	token, err := s.client.credentials.AccessTokenFor(ctx, campusauth.FavoriteWriteScope)
	if err != nil {
		return finish("未写入收藏：请在设置中重新授权课程收藏读写。")
	}
	readToken, err := s.client.credentials.AccessTokenFor(ctx, campusauth.FavoriteReadScope)
	if err != nil || token != readToken {
		return finish("未写入收藏：收藏读取授权不可用或账号已变化，请重新尝试。")
	}
	headers := http.Header{"Authorization": {"Bearer " + token}, "Accept": {"application/json"}, "Content-Type": {"application/json"}}
	read := func() ([]string, error) {
		var e struct {
			Code *int `json:"code"`
			Data []struct {
				ClassID string `json:"classID"`
			} `json:"data"`
		}
		if err := sourceJSON(ctx, s.client.http, http.MethodGet, campusRoot+"/academic/class/fav", nil, headers, &e); err != nil {
			return nil, err
		}
		if e.Code == nil || *e.Code != 0 || e.Data == nil {
			return nil, errSourceResponse
		}
		out := []string{}
		for _, v := range e.Data {
			if v.ClassID == "" {
				return nil, errSourceResponse
			}
			out = append(out, v.ClassID)
		}
		return out, nil
	}
	s.progress("正在读取收藏并执行指定修改…")
	existing, err := read()
	if err != nil {
		return finish("未写入收藏：无法读取原收藏，已停止，避免覆盖已有内容。")
	}
	present := map[string]bool{}
	for _, id := range existing {
		present[id] = true
	}
	remove := map[string]bool{}
	removeIDs := in.RemoveClassIDs
	if in.Action == "remove" {
		removeIDs = ids
	}
	for _, id := range removeIDs {
		if !present[id] || !s.favoriteSelection[id] {
			return finish("未写入收藏：删除目标不在本轮已读取的收藏中，请先查看收藏再选择。")
		}
		remove[id] = true
	}
	if (in.Action == "clear" || in.Action == "replace") && !sameIDs(existing, s.favoriteSnapshot) {
		return finish("未写入收藏：请先读取当前完整收藏；列表已变化时需重新选择，不覆盖未查看的内容。")
	}
	if (in.Action == "clear" || in.Action == "replace") && s.favoriteSnapshot == nil {
		return finish("未写入收藏：请先查看收藏，再执行清空或替换。")
	}
	union := []string{}
	if in.Action != "clear" && in.Action != "replace" {
		for _, id := range existing {
			if !remove[id] {
				union = append(union, id)
			}
		}
	}
	if adding {
		kept := map[string]bool{}
		for _, id := range union {
			kept[id] = true
		}
		for _, id := range ids {
			if !kept[id] {
				union = append(union, id)
				kept[id] = true
			}
		}
	}
	result.TotalCount = len(union)
	if sameIDs(union, existing) {
		result.Status = "already_present"
		return finish("已在收藏中。")
	}
	body, _ := json.Marshal(map[string]any{"classes": union})
	var response struct {
		Code *int `json:"code"`
	}
	writeErr := sourceJSON(ctx, s.client.http, http.MethodPost, campusRoot+"/academic/class/fav", bytes.NewReader(body), headers, &response)
	// A lost response may still have committed. Always read back; never retry POST.
	after, err := read()
	verified := map[string]bool{}
	for _, id := range after {
		verified[id] = true
	}
	complete := err == nil
	if in.Action != "add" {
		complete = complete && sameIDs(after, union)
	}
	for _, id := range union {
		complete = complete && verified[id]
	}
	if complete {
		result.Status = "confirmed"
		existingSet := map[string]bool{}
		for _, id := range existing {
			existingSet[id] = true
		}
		for _, id := range ids {
			if adding && !existingSet[id] {
				result.AddedCount++
			}
		}
		result.TotalCount = len(after)
		if in.Action == "add" {
			return finish("收藏已确认。")
		}
		return finish(fmt.Sprintf("收藏已更新，共%d个教学班。", len(after)))
	}
	result.Status = "unknown"
	if writeErr == nil && response.Code != nil && *response.Code != 0 {
		result.Status = "not_written"
		return finish("未写入收藏：服务拒绝了请求，请检查收藏授权后再试。")
	}
	return finish("收藏结果尚未确认：写入后未能核实完整列表，可能已经生效；未自动重试，请稍后核对收藏。")
}
