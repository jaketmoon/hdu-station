package tools

import "testing"

func TestCandidateSelectionOnlyChangesRelatedQueries(t *testing.T) {
	exact := OfferingQuery{Query: "戏曲鉴赏", Classes: []Offering{{ClassID: "opera", CourseID: "002"}}}
	got := selectCourseIDs(exact, []string{"001"})
	if len(got.Classes) != 1 || got.Classes[0].ClassID != "opera" || got.NeedsSelection {
		t.Fatal("choosing another course erased an exact match")
	}
	ambiguous := selectCourseIDs(OfferingQuery{Query: "影视音乐鉴赏", Candidates: []CourseCandidate{
		{CourseID: "001", Classes: []Offering{{ClassID: "film-a", CourseID: "001"}, {ClassID: "film-b", CourseID: "001"}}},
		{CourseID: "003", Classes: []Offering{{ClassID: "other", CourseID: "003"}}},
	}}, nil)
	got = selectCourseIDs(ambiguous, []string{"001"})
	if len(got.Classes) != 2 || got.NeedsSelection || got.Warning != "" {
		t.Fatal("selected query did not complete or lost a class")
	}
	got = selectCourseIDs(ambiguous, []string{"002"})
	if !got.NeedsSelection || len(got.Candidates) != 2 || len(got.Classes) != 0 {
		t.Fatal("unrelated ID destroyed pending candidates")
	}
	got = selectCourseIDs(ambiguous, []string{"001", "003"})
	if len(got.Classes) != 3 || got.NeedsSelection {
		t.Fatal("multiple explicitly selected candidates not retained")
	}
	got = selectCourseIDs(ambiguous, []string{"forged"})
	if len(got.Classes) != 0 || !got.NeedsSelection {
		t.Fatal("unknown ID promoted a candidate")
	}
}

func TestCandidateSelectionIsIdempotent(t *testing.T) {
	q := OfferingQuery{Candidates: []CourseCandidate{{CourseID: "001", Classes: []Offering{{ClassID: "a", CourseID: "001"}, {ClassID: "b", CourseID: "001"}}}}}
	q = selectCourseIDs(q, []string{"001"})
	q = selectCourseIDs(q, []string{"001"})
	if len(q.Classes) != 2 {
		t.Fatal("repeated selection duplicated classes")
	}
}
