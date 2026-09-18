package tools

import (
	"context"
	"time"
)

type AcademicTermInput struct{}
type AcademicTermResult struct {
	Term    AcademicTerm `json:"term"`
	Source  string       `json:"source"`
	Warning string       `json:"warning,omitempty"`
}

// A metadata question has no course-name precondition and does not read a
// personal timetable. It uses the same default resolution as campus queries.
func (s *CampusSession) ReadAcademicTerm(ctx context.Context, _ AcademicTermInput) (AcademicTermResult, error) {
	s.Calls++
	s.progress("正在查询教务默认学期…")
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	term, source, err := s.resolveTerm(ctx, OfferingInput{})
	result := AcademicTermResult{Term: term, Source: source}
	if err != nil {
		result.Warning = err.Error()
	}
	s.lastTermResult = &result
	return result, nil
}
