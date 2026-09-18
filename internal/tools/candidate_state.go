package tools

const candidateLimitWarning = "本轮保留的课程查询已达48个；本次新增查询未执行，已有结果保留。请明确需要保留哪些课程后再查询。"

func (s *CampusSession) candidateLimit(names []string, term AcademicTerm, replace bool) bool {
	seen := map[string]bool{}
	if !replace && s.lastOfferings != nil && term.valid() && term == s.lastOfferings.Term {
		for _, query := range s.lastOfferings.Queries {
			seen[query.Query] = true
		}
	}
	for _, name := range names {
		seen[name] = true
	}
	return len(seen) > 48
}

// Merge within this answer and semester, preserving each original query. A
// disambiguation call is a patch; replacement requires an explicit input flag.
func (s *CampusSession) mergeOfferings(next OfferingResult, in OfferingInput) OfferingResult {
	old := s.lastOfferings
	if in.ReplaceCandidates || old == nil || !next.Term.valid() || next.Term != old.Term {
		return next
	}
	merged := append([]OfferingQuery(nil), old.Queries...)
	index := map[string]int{}
	for i, query := range merged {
		index[query.Query] = i
	}
	for _, query := range next.Queries {
		if i, ok := index[query.Query]; ok {
			// Reusing a query for new time preferences must not undo a previous
			// name choice. An explicit related course ID below can replace it.
			if len(in.CourseIDs) == 0 && query.NeedsSelection && len(merged[i].Classes) > 0 {
				ids := []string{}
				for _, class := range merged[i].Classes {
					ids = append(ids, class.CourseID)
				}
				query = selectCourseIDs(query, ids)
			}
			merged[i] = query
		} else {
			index[query.Query] = len(merged)
			merged = append(merged, query)
		}
	}
	for i, query := range merged {
		merged[i] = selectCourseIDs(query, in.CourseIDs)
	}
	next.Queries = merged
	return next
}

func (s *CampusSession) resetPreferencesForTerm(term AcademicTerm) {
	previous := AcademicTerm{}
	if s.lastOfferings != nil {
		previous = s.lastOfferings.Term
	} else if s.lastFit != nil {
		previous = s.lastFit.Term
	}
	if previous.valid() && term.valid() && term != previous {
		s.fitPreferences = FitCoursesInput{}
		s.displayPreferences = FitCoursesInput{}
		s.scheduleRequested = false
	}
}
