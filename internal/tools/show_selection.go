package tools

import "strings"

// Selection changes identity only; preserve all already-computed time evidence,
// including incomplete timetable results, without issuing another campus read.
func (s *CampusSession) selectShownCourses(ids []string) {
	old := s.lastOfferings
	next := *old
	next.Queries = append([]OfferingQuery(nil), old.Queries...)
	for i, q := range next.Queries {
		next.Queries[i] = selectCourseIDs(q, ids)
	}
	s.lastOfferings = &next
	if s.lastFit == nil || s.lastFit.Offerings != old {
		return
	}
	fit := *s.lastFit
	fit.Offerings = &next
	known := map[string]CourseFit{}
	for _, f := range fit.CandidateFits {
		known[f.Offering.ClassID] = f
	}
	for _, f := range fit.Fits {
		known[f.Offering.ClassID] = f
	}
	fit.Fits, fit.CandidateFits = nil, nil
	selected := map[string]bool{}
	pending := false
	for _, q := range next.Queries {
		pending = pending || q.NeedsSelection
		for _, o := range q.Classes {
			if selected[o.ClassID] {
				continue
			}
			selected[o.ClassID] = true
			f, ok := known[o.ClassID]
			if !ok {
				f = CourseFit{Offering: o, Status: "unknown", Reason: "尚未核实课表兼容性。"}
			}
			fit.Fits = append(fit.Fits, f)
		}
	}
	// Keep unselected alternatives available for another explicit choice.
	seen := map[string]bool{}
	for _, f := range append(append([]CourseFit(nil), s.lastFit.CandidateFits...), s.lastFit.Fits...) {
		id := f.Offering.ClassID
		if !selected[id] && !seen[id] {
			fit.CandidateFits = append(fit.CandidateFits, known[id])
			seen[id] = true
		}
	}
	complete := false
	fit.SuggestedPlan, complete = bestCoursePlan(fit.Fits, 12)
	fit.PlanIncomplete = !complete
	fit.Warnings = append([]string(nil), fit.Warnings...)
	if !pending {
		warnings := fit.Warnings[:0]
		for _, warning := range fit.Warnings {
			if !strings.HasPrefix(warning, "有候选名称尚未选择。") {
				warnings = append(warnings, warning)
			}
		}
		fit.Warnings = warnings
	}
	s.lastFit = &fit
}
