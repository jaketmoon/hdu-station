package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

type contextGrant string

func (g contextGrant) AccessTokenFor(context.Context, string) (string, error) { return string(g), nil }
func TestCourseContextIsolationExpiryAndProvenance(t *testing.T) {
	ctx := context.Background()
	var store CourseContexts
	client := NewCampusClient(contextGrant("account-a"))
	original := NewCampusSession(client, nil)
	original.lastOfferings = &OfferingResult{Term: AcademicTerm{"2026-2027", 1}, Queries: []OfferingQuery{{Query: "影视音乐鉴赏", NeedsSelection: true, Candidates: []CourseCandidate{{CourseID: "001", CourseName: "影视音乐赏析", Classes: []Offering{{ClassID: "a", CourseID: "001", CourseName: "影视音乐赏析"}, {ClassID: "b", CourseID: "001", CourseName: "影视音乐赏析"}}}}}}}
	store.Save(ctx, "one", original)
	restored := NewCampusSession(client, nil)
	data := store.Restore(ctx, "one", restored)
	if !strings.Contains(data, "影视音乐") || strings.Contains(data, "account-a") || restored.lastOfferings == original.lastOfferings {
		t.Fatal("missing or unsafe evidence")
	}
	if store.Restore(ctx, "two", NewCampusSession(client, nil)) != "" {
		t.Fatal("cross-conversation leak")
	}
	if restored.lastFit != nil || restored.simulationSnapshot != nil || restored.favoriteSnapshot != nil {
		t.Fatal("restored stale execution state")
	}
	stored := []string{"existing"}
	posts := 0
	client.http.Transport = campusTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/hduhelp-neo/academic/class/fav" {
			t.Fatal("unexpected requery")
		}
		if r.Method == "POST" {
			posts++
			var b struct {
				Classes []string `json:"classes"`
			}
			json.NewDecoder(r.Body).Decode(&b)
			stored = b.Classes
			return campusResponse(200, `{"code":0}`), nil
		}
		entries := []FavoriteEntry{}
		for _, id := range stored {
			entries = append(entries, FavoriteEntry{ClassID: id})
		}
		b, _ := json.Marshal(map[string]any{"code": 0, "data": entries})
		return campusResponse(200, string(b)), nil
	})
	result, _ := restored.ManageFavorites(ctx, FavoriteInput{Action: "add", ClassIDs: []string{"a", "b", "a"}})
	if result.Status != "confirmed" || posts != 1 || len(stored) != 3 {
		t.Fatal("cached similar multi-class add lost IDs or original")
	}
	bad, _ := restored.ManageFavorites(ctx, FavoriteInput{Action: "add", ClassIDs: []string{"invented"}})
	if bad.Status != "not_written" || posts != 1 {
		t.Fatal("untrusted ID written")
	}
	if store.Restore(ctx, "one", NewCampusSession(NewCampusClient(contextGrant("account-b")), nil)) != "" {
		t.Fatal("account changed without invalidation")
	}
	store.Save(ctx, "one", original)
	entry := store.entries["one"]
	entry.expires = time.Now().Add(-time.Second)
	store.entries["one"] = entry
	if store.Restore(ctx, "one", NewCampusSession(client, nil)) != "" {
		t.Fatal("expired evidence restored")
	}
	store.Save(ctx, "one", original)
	store.Delete("one")
	if store.Restore(ctx, "one", NewCampusSession(client, nil)) != "" {
		t.Fatal("deleted evidence restored")
	}
}
func TestKeywordSearchUsesOnlyModelProvidedWords(t *testing.T) {
	words, err := lookupKeywords([]string{"人工智能中的法律问题", "影视音乐鉴赏"}, nil, nil)
	if err != nil || words["人工智能中的法律问题"] != "人工智能中的法律问题" || words["影视音乐鉴赏"] != "影视音乐鉴赏" {
		t.Fatal("host rewrote search semantics")
	}
	words, err = lookupKeywords([]string{"影视音乐鉴赏"}, []CourseKeyword{{Course: "影视音乐鉴赏", Keyword: "影视音乐"}}, nil)
	if err != nil || words["影视音乐鉴赏"] != "影视音乐" {
		t.Fatal("model keyword ignored")
	}
}

func TestCourseContextKeepsOnlyLatestOperationAndDoesNotRenewEvidence(t *testing.T) {
	ctx := context.Background()
	var contexts CourseContexts
	s := NewCampusSession(NewCampusClient(contextGrant("account")), nil)
	s.lastOfferings = &OfferingResult{Term: AcademicTerm{"2026-2027", 1}, Queries: []OfferingQuery{{Query: "测试课"}}}
	s.favoriteReceipts = []FavoriteResult{{Action: "add", Status: "not_written", Message: "intermediate-failure"}, {Action: "add", Status: "confirmed", Message: "done", Courses: []Offering{{ClassID: "verified", Times: []CourseTime{{Day: 1, Weeks: []int{1}, Sections: []int{1}}}}}}}
	contexts.Save(ctx, "one", s)
	next := NewCampusSession(s.client, nil)
	data := contexts.Restore(ctx, "one", next)
	if strings.Contains(data, "intermediate-failure") || strings.Contains(data, `"times"`) || !strings.Contains(data, `"confirmed"`) {
		t.Fatal("not a compact final outcome")
	}
	expiry := contexts.entries["one"].expires
	contexts.Save(ctx, "one", next)
	if !contexts.entries["one"].expires.Equal(expiry) {
		t.Fatal("following up renewed old evidence")
	}
}

func TestStructuredScheduleAvailabilityNeverConfirmsIncompleteFreeTime(t *testing.T) {
	fit := FitCoursesResult{BusyTimes: []CourseTime{{Day: 2, Sections: []int{6}, Weeks: []int{1, 3}}}}
	pref := FitCoursesInput{AllowedDays: []int{2}, AllowedSections: []int{6, 7}}
	rows := scheduleWindows(fit, pref)
	if len(rows) != 2 || rows[0].Status != "occupied" || rows[1].Status != "unknown" {
		t.Fatal("incomplete occupancy was upgraded")
	}
	fit.ScheduleComplete = true
	rows = scheduleWindows(fit, pref)
	if rows[1].Status != "free" || len(rows[0].BusyWeeks) != 2 {
		t.Fatal("scoped availability lost known evidence")
	}
}
