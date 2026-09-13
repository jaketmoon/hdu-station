package agent

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/jaketmoon/hdu-station/internal/storage"
)

func TestCourseRequestDataset(t *testing.T) {
	data, err := os.ReadFile("testdata/student_questions.json")
	if err != nil {
		t.Fatal(err)
	}
	var dataset struct {
		Cases []struct {
			ID    string   `json:"id"`
			Turns []string `json:"turns"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &dataset); err != nil {
		t.Fatal(err)
	}
	boundaries := map[string]bool{}
	for _, id := range []string{"E03", "E04", "E10", "E16", "E18", "E31", "E32", "E33", "E36", "E37", "E38", "E39", "E40", "E41", "E42", "E43", "E44", "E49", "E54", "E55"} {
		boundaries[id] = true
	}
	preferences := map[string]struct{ days, sections []int }{
		"R03": {[]int{3}, requestRange(10, 14)}, "R04": {[]int{1, 2, 3, 4, 6, 7}, nil}, "R05": {[]int{2}, requestRange(6, 9)},
		"E19": {nil, requestRange(1, 5)}, "E20": {nil, requestRange(6, 9)}, "E21": {[]int{2}, requestRange(10, 14)},
		"E22": {[]int{6, 7}, nil}, "E23": {[]int{1, 3, 4, 5, 6, 7}, nil}, "E24": {[]int{2, 4}, nil},
		"E25": {nil, []int{8}}, "E26": {[]int{2}, []int{6, 7}}, "E50": {[]int{2}, requestRange(10, 14)}, "E51": {[]int{1, 2, 3, 4, 6, 7}, nil},
	}
	turns := 0
	for _, c := range dataset.Cases {
		t.Run(c.ID, func(t *testing.T) {
			history := []storage.Message{}
			var got courseRequest
			for i, q := range c.Turns {
				history = append(history, storage.Message{Role: "user", State: "complete", Content: q})
				got = parseCourseRequest(history)
				wantBoundary := boundaries[c.ID] || (c.ID == "E52" && i == 1)
				if (got.Boundary != "") != wantBoundary {
					t.Fatalf("turn %d boundary mismatch: %+v", i+1, got)
				}
				turns++
			}
			if want, ok := preferences[c.ID]; ok && (!reflect.DeepEqual(got.AllowedDays, want.days) || !reflect.DeepEqual(got.AllowedSections, want.sections) || !got.MatchSchedule) {
				t.Fatalf("wrong constraint: %+v", got)
			}
			if c.ID == "R02" || c.ID == "E28" || c.ID == "E53" {
				if len(got.AllowedDays)+len(got.AllowedSections) > 0 {
					t.Fatal("occupied time became allowed preference")
				}
				if !got.MatchSchedule {
					t.Fatal("schedule or conflict request was not recognized")
				}
			}
			if (c.ID == "E28" || c.ID == "E29") && !got.MatchSchedule {
				t.Fatal("conflict/enrollment question skipped timetable")
			}
			if (c.ID == "E45" || c.ID == "E47") && !got.CommunityOnly {
				t.Fatal("explicit community-only request became official check")
			}
		})
	}
	if len(dataset.Cases) != 60 || turns != 65 {
		t.Fatalf("dataset coverage changed: %d cases / %d turns", len(dataset.Cases), turns)
	}
}

func TestCourseRequestPreferenceUpdatesAndClarification(t *testing.T) {
	noSchedule := parseCourseRequest([]storage.Message{{Role: "user", Content: "结合课表查戏曲鉴赏"}, {Role: "user", Content: "这次不要查课表，只查开课"}})
	if noSchedule.MatchSchedule {
		t.Fatal("latest request to skip timetable ignored")
	}
	for _, q := range []string{"推荐作业少一点的课", "轻松一点，给分好一点", "选一门简单一点的通识"} {
		if requestBoundary(q) != "" {
			t.Fatalf("degree adverb mistaken for clock: %s", q)
		}
	}
	for _, tc := range []struct {
		questions      []string
		days, sections []int
		boundary       bool
	}{
		{[]string{"只考虑周二下午的课", "改成只要周四晚上"}, []int{4}, requestRange(10, 14), false},
		{[]string{"只考虑周二和周四的课", "不要周二"}, []int{4}, nil, false},
		{[]string{"只要周五的课", "周五必须全天空着"}, nil, nil, true},
		{[]string{"下学期会开吗", "只查本学期戏曲鉴赏"}, nil, nil, false},
		{[]string{"周二下午不要排课"}, nil, nil, true},
		{[]string{"只考虑周二下午和周四晚上"}, nil, nil, true},
		{[]string{"不用推荐，只查戏曲鉴赏开课"}, nil, nil, false},
		{[]string{"戏曲鉴赏和影视音乐赏析哪个符合？只查开课和课表。"}, nil, nil, false},
		{[]string{"本学期是哪个学期？影视音乐赏析时间按哪个学期查的"}, nil, nil, false},
	} {
		history := []storage.Message{{Role: "assistant", State: "complete", Content: "只能周五下午"}}
		for _, q := range tc.questions {
			history = append(history, storage.Message{Role: "user", State: "complete", Content: q})
		}
		got := parseCourseRequest(history)
		if (got.Boundary != "") != tc.boundary {
			t.Fatalf("%v: %+v", tc.questions, got)
		}
		if !tc.boundary && (!reflect.DeepEqual(got.AllowedDays, tc.days) || !reflect.DeepEqual(got.AllowedSections, tc.sections)) {
			t.Fatalf("%v: %+v", tc.questions, got)
		}
	}
}

func TestCourseRequestExplicitCount(t *testing.T) {
	for question, want := range map[string][2]int{
		"影视音乐赏析、数字游戏设计与艺术赏析、书法鉴赏都想选": {0, 12},
		"推荐一门": {1, 1}, "推荐两门": {2, 2}, "想选两三门": {2, 3}, "推荐四门": {4, 4}, "找11门课": {11, 11}, "看十二门": {12, 12},
	} {
		got := parseCourseRequest([]storage.Message{{Role: "user", State: "complete", Content: question}})
		if got.MinCourses != want[0] || got.MaxCourses != want[1] {
			t.Fatalf("%s: got %d–%d want %v", question, got.MinCourses, got.MaxCourses, want)
		}
	}
}

func TestCourseRequestCommunityOnly(t *testing.T) {
	for _, tc := range []struct {
		question string
		want     bool
	}{
		{"小红书有什么杭电选修体验？", true},
		{"再提醒我一下它怎么考核？", true},
		{"书法鉴赏的口碑怎么样？", true},
		{"看口碑给我推荐两门课", false},
		{"通识有没有轻松点的课啊，别作业一堆。", false},
	} {
		got := parseCourseRequest([]storage.Message{{Role: "user", State: "complete", Content: tc.question}})
		if got.CommunityOnly != tc.want || got.Boundary != "" {
			t.Fatalf("%s: %+v", tc.question, got)
		}
	}
}

func TestOfficialOnlyRoutingPreservesCommunityRequests(t *testing.T) {
	enrolled := parseCourseRequest([]storage.Message{{Role: "user", Content: "我已选的形势与政策（国家安全教育）1，还能再加另一个班吗？"}})
	if enrolled.EnrolledCourse != "形势与政策（国家安全教育）1" {
		t.Fatal("explicit enrolled course name was rewritten")
	}
	if parseCourseRequest([]storage.Message{{Role: "user", Content: "只查教务"}}).RequireVerification {
		t.Fatal("missing candidate must allow clarification")
	}
	if parseCourseRequest([]storage.Message{{Role: "user", Content: "我已选的课，还能再加一个班吗？"}}).RequireVerification {
		t.Fatal("generic enrolled-course reference is not a course name")
	}
	for _, q := range []string{
		"我已选的形势与政策（国家安全教育）1，还能再加另一个班吗？",
		"我已选的大学生与法，还能再加一个班吗？",
		"影视音乐赏析有哪些班？",
		"本学期是哪个学期？影视音乐赏析的时间按哪个学期查的？",
		"火星殖民与量子占卜导论这学期有开吗？",
		"帮我查C5692013，本学期有没有班能放进我的课表？",
		"不要晚课，帮我在影视音乐赏析里找一个不撞课的下午班。",
		"我只想周二晚上加一门，影视音乐赏析能放吗？",
		"只用教务，别查网友评价，影视音乐鉴赏这学期开不开？",
		"影视音悦鉴赏能塞到我的课表吗？不搜帖子。",
	} {
		got := parseCourseRequest([]storage.Message{{Role: "user", Content: q}})
		if !got.OfficialOnly || !got.RequireVerification {
			t.Fatalf("unnecessary community tools: %s", q)
		}
	}
	for _, q := range []string{
		"先看看我的课表，再给我推荐几门轻松的通识。",
		"周三下午忙，剩下能塞点啥，别太累。",
		"影视音乐赏析怎么样，作业多吗？",
		"只查小红书的影视音乐赏析体验。",
		"只看QQ里书法鉴赏的评价。",
		"影视音乐赏析适合零基础吗？",
	} {
		history := []storage.Message{{Role: "user", Content: "结合课表查影视音乐赏析"}, {Role: "user", Content: q}}
		if parseCourseRequest(history).OfficialOnly {
			t.Fatalf("community request suppressed: %s", q)
		}
	}
}
