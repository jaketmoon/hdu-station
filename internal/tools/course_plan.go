package tools

const coursePlanStepLimit = 100_000

// bestCoursePlan maximizes the number of distinct courses, taking at most one
// class per course. Equal-size solutions retain course/class input order.
// maxCourses <= 0 means 12. complete is false when the 12-course or search-step
// bound prevents proving optimality; the returned plan is still compatible.
func bestCoursePlan(fits []CourseFit, maxCourses int) (classIDs []string, complete bool) {
	return coursePlanWithBudget(fits, maxCourses, coursePlanStepLimit)
}

func coursePlanWithBudget(fits []CourseFit, maxCourses, budget int) ([]string, bool) {
	if maxCourses <= 0 || maxCourses > 12 {
		maxCourses = 12
	}
	groups := [][]Offering{}
	groupIndex := map[string]int{}
	seenClasses := map[string]bool{}
	complete := true
	for _, fit := range fits {
		o := fit.Offering
		if fit.Status != "fits" || !o.TimeComplete || len(o.Times) == 0 || o.CourseID == "" || o.ClassID == "" || seenClasses[o.ClassID] {
			continue
		}
		index, ok := groupIndex[o.CourseID]
		if !ok {
			if len(groups) == 12 {
				complete = false
				continue
			}
			index = len(groups)
			groupIndex[o.CourseID] = index
			groups = append(groups, nil)
		}
		seenClasses[o.ClassID] = true
		groups[index] = append(groups[index], o)
	}

	// Depth-first branch and bound: selecting a class precedes skipping its
	// course, and a branch cannot improve once even all remaining courses would
	// only tie the best result. Count rejected candidates too, so a large group
	// of conflicting classes cannot bypass the deterministic work limit.
	chosen := []Offering{}
	best := []string{}
	steps := 0
	exhausted := false
	var visit func(int)
	visit = func(index int) {
		if len(chosen) > len(best) {
			best = best[:0]
			for _, o := range chosen {
				best = append(best, o.ClassID)
			}
		}
		if exhausted || len(best) == maxCourses || len(chosen)+len(groups)-index <= len(best) {
			return
		}
		for _, o := range groups[index] {
			if steps >= budget {
				exhausted = true
				return
			}
			steps++
			compatible := true
		check:
			for _, old := range chosen {
				for _, a := range old.Times {
					for _, b := range o.Times {
						if _, hit := clash(a, b); hit {
							compatible = false
							break check
						}
					}
				}
			}
			if compatible {
				chosen = append(chosen, o)
				visit(index + 1)
				chosen = chosen[:len(chosen)-1]
			}
			if exhausted || len(best) == maxCourses {
				return
			}
		}
		if steps >= budget {
			exhausted = true
			return
		}
		steps++
		visit(index + 1)
	}
	visit(0)
	return best, complete && !exhausted
}
