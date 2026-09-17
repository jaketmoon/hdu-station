package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/jaketmoon/hdu-station/internal/campusauth"
)

func sameIDs(a, b []string) bool {
	set := map[string]bool{}
	for _, id := range a {
		set[id] = true
	}
	other := map[string]bool{}
	for _, id := range b {
		other[id] = true
	}
	if len(set) != len(other) {
		return false
	}
	for id := range set {
		if !other[id] {
			return false
		}
	}
	return true
}
func (s *CampusSession) managementHeaders(ctx context.Context, read, write string) (http.Header, error) {
	if s.client == nil || s.client.credentials == nil {
		return nil, campusauth.ErrLoginRequired
	}
	token, err := s.client.credentials.AccessTokenFor(ctx, read)
	if err != nil {
		return nil, err
	}
	if write != "" {
		next, e := s.client.credentials.AccessTokenFor(ctx, write)
		if e != nil {
			return nil, e
		}
		if token != next {
			return nil, campusauth.ErrLoginRequired
		}
	}
	return http.Header{"Authorization": {"Bearer " + token}, "Accept": {"application/json"}, "Content-Type": {"application/json"}}, nil
}
func (s *CampusSession) readFavorites(ctx context.Context, action string, finish func(string) (FavoriteResult, error), result *FavoriteResult) (FavoriteResult, error) {
	headers, err := s.managementHeaders(ctx, campusauth.FavoriteReadScope, "")
	if err != nil {
		return finish("无法读取收藏：请在设置中检查收藏读取授权。")
	}
	path := "/academic/class/fav"
	if action == "rank" {
		path += "/rank"
	}
	var e struct {
		Code *int            `json:"code"`
		Data []FavoriteEntry `json:"data"`
	}
	err = sourceJSON(ctx, s.client.http, http.MethodGet, campusRoot+path, nil, headers, &e)
	if err != nil || e.Code == nil || *e.Code != 0 || e.Data == nil {
		return finish("无法读取收藏：服务未返回完整有效列表。")
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, v := range e.Data {
		if v.ClassID == "" || seen[v.ClassID] {
			return finish("无法读取收藏：列表包含无效或重复项。")
		}
		seen[v.ClassID] = true
		ids = append(ids, v.ClassID)
	}
	if action == "read" {
		s.favoriteSnapshot = append([]string{}, ids...)
		s.favoriteSelection = seen
	}
	result.Status = "read"
	result.Entries = e.Data
	details := s.favoriteDetails(ctx, ids)
	if action == "read" {
		s.favoriteKnown = details
	}
	var b strings.Builder
	title := "当前课程收藏"
	if action == "rank" {
		title = "课程收藏排行"
	}
	fmt.Fprintf(&b, "%s：共%d个教学班。", title, len(ids))
	for i, v := range e.Data {
		o, ok := details[v.ClassID]
		if !ok {
			o = Offering{ClassID: v.ClassID, CourseName: fmt.Sprintf("收藏项%d（详情未取得）", i+1)}
		}
		result.Courses = append(result.Courses, o)
		if action == "rank" {
			fmt.Fprintf(&b, "\n\n%d. %s（%s，%s），%d人收藏。", i+1, campusCell(o.CourseName), campusCell(o.Teacher), campusCell(o.ClassTime), v.FavCount)
		}
	}
	return finish(b.String())
}

// Decode only public course fields, never classList or other upstream metadata.
func (s *CampusSession) favoriteDetails(ctx context.Context, ids []string) map[string]Offering {
	out := map[string]Offering{}
	for start := 0; start < len(ids); start += 20 {
		end := start + 20
		if end > len(ids) {
			end = len(ids)
		}
		q := url.Values{}
		for _, id := range ids[start:end] {
			q.Add("id", id)
		}
		var data struct {
			Classes []struct {
				ClassID    string `json:"classID"`
				CourseID   string `json:"courseID"`
				CourseName string `json:"courseName"`
				Teacher    string `json:"teacherName"`
				ClassTime  string `json:"classTime"`
			} `json:"classes"`
		}
		if _, err := s.get(ctx, "/academic/course", q, &data); err != nil {
			break
		}
		for _, o := range data.Classes {
			out[o.ClassID] = Offering{ClassID: o.ClassID, CourseID: cleanCampusText(o.CourseID, 128), CourseName: cleanCampusText(o.CourseName, 120), Teacher: cleanCampusText(o.Teacher, 100), ClassTime: cleanCampusText(o.ClassTime, 600)}
		}
	}
	return out
}
func (s *CampusSession) ManagementDisplay() string {
	return strings.TrimSpace(s.FavoriteDisplay() + "\n\n" + s.managementDisplay)
}
