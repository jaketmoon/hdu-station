package skills

import (
	_ "embed"
	"regexp"
	"strings"
)

//go:embed course-selection/SKILL.md
var document string

func CourseSelection() (string, []string) {
	parts := strings.SplitN(document, "<!-- channel-scope -->", 2)
	return parts[0], regexp.MustCompile("[0-9]{17}").FindAllString(parts[1], -1)
}
