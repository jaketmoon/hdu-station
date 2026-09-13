package tools

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// A meeting occupies the Cartesian product of Weeks and Sections on Day.
type CourseTime struct {
	Day      int   `json:"day"`
	Weeks    []int `json:"weeks"`
	Sections []int `json:"sections"`
}

var meetingPattern = regexp.MustCompile(`^(?:星期|周)([一二三四五六日天])第([0-9,、\- ]+)节\{([^{}]+)\}$`)
var numberRangePattern = regexp.MustCompile(`^([0-9]+)(?:-([0-9]+))?$`)

func parseNumberRanges(raw string, max int) ([]int, bool) {
	seen := map[int]bool{}
	for _, part := range strings.Split(strings.ReplaceAll(raw, "、", ","), ",") {
		m := numberRangePattern.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil {
			return nil, false
		}
		a, _ := strconv.Atoi(m[1])
		b := a
		if m[2] != "" {
			b, _ = strconv.Atoi(m[2])
		}
		if a < 1 || b < a || b > max {
			return nil, false
		}
		for n := a; n <= b; n++ {
			seen[n] = true
		}
	}
	result := make([]int, 0, len(seen))
	for n := range seen {
		result = append(result, n)
	}
	sort.Ints(result)
	return result, len(result) > 0
}

func parseWeeks(raw string) ([]int, bool) {
	raw = strings.NewReplacer("（", "(", "）", ")", "，", ",").Replace(raw)
	seen := map[int]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		parity := 0
		if strings.HasSuffix(part, "(单)") {
			parity = 1
			part = strings.TrimSuffix(part, "(单)")
		}
		if strings.HasSuffix(part, "(双)") {
			parity = 2
			part = strings.TrimSuffix(part, "(双)")
		}
		part = strings.TrimPrefix(strings.TrimSuffix(part, "周"), "第")
		numbers, ok := parseNumberRanges(part, 30)
		if !ok {
			return nil, false
		}
		for _, n := range numbers {
			if parity == 0 || (parity == 1 && n%2 == 1) || (parity == 2 && n%2 == 0) {
				seen[n] = true
			}
		}
	}
	result := make([]int, 0, len(seen))
	for n := range seen {
		result = append(result, n)
	}
	sort.Ints(result)
	return result, len(result) > 0
}

// Only consume an entire known grammar. Unknown text never becomes free time.
func parseCourseTimes(raw string) ([]CourseTime, bool) {
	if strings.TrimSpace(raw) == "" || len(raw) > 8000 {
		return nil, false
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ';' || r == '；' || r == '\n' || r == '\r' })
	result := []CourseTime{}
	complete := len(parts) > 0
	for _, part := range parts {
		m := meetingPattern.FindStringSubmatch(strings.TrimSpace(part))
		if m == nil {
			complete = false
			continue
		}
		day := map[string]int{"一": 1, "二": 2, "三": 3, "四": 4, "五": 5, "六": 6, "日": 7, "天": 7}[m[1]]
		sections, ok := parseNumberRanges(m[2], 14)
		weeks, ok2 := parseWeeks(m[3])
		if !ok || !ok2 {
			complete = false
			continue
		}
		result = append(result, CourseTime{Day: day, Weeks: weeks, Sections: sections})
	}
	return result, complete && len(result) > 0
}

func overlap(a, b []int) []int {
	result := []int{}
	for _, x := range a {
		for _, y := range b {
			if x == y {
				result = append(result, x)
				break
			}
		}
	}
	return result
}
func clash(a, b CourseTime) (CourseTime, bool) {
	if a.Day != b.Day {
		return CourseTime{}, false
	}
	w, p := overlap(a.Weeks, b.Weeks), overlap(a.Sections, b.Sections)
	return CourseTime{Day: a.Day, Weeks: w, Sections: p}, len(w) > 0 && len(p) > 0
}
