package tools

import (
	"context"
	"fmt"
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

func displayAcademicTerm(r AcademicTermResult) string {
	if !r.Term.valid() {
		return "尚未确认教务默认查询学期。" + campusCell(r.Warning) + "\n\n"
	}
	return fmt.Sprintf("教务当前默认查询学期为 **%s 学年第 %d 学期**。按“本学期”核实课程时使用这个学期。\n\n来源：校园教务配置。\n\n", r.Term.SchoolYear, r.Term.Semester)
}
