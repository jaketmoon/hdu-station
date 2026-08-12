package skills

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	CourseSelectionName    = "course-selection"
	CourseSelectionVersion = "0.1.2"
	maxTencentCourseGuilds = 3
)

//go:embed course-selection/SKILL.md
var embeddedCourseSelection []byte

type Skill struct {
	Name            string
	Version         string
	Prompt          string
	TencentGuildIDs []string
}

func LoadCourseSelection(dataRoot string) (Skill, error) {
	data := embeddedCourseSelection
	if strings.TrimSpace(dataRoot) != "" {
		path := filepath.Join(dataRoot, "skills", CourseSelectionName, "SKILL.md")
		if custom, err := os.ReadFile(path); err == nil {
			data = custom
		} else if !errors.Is(err, os.ErrNotExist) {
			return Skill{}, fmt.Errorf("read course-selection skill: %w", err)
		}
	}
	prompt := strings.TrimSpace(string(data))
	if prompt == "" {
		return Skill{}, errors.New("course-selection skill is empty")
	}
	guildIDs := extractTencentGuildIDs(prompt)
	if len(guildIDs) > maxTencentCourseGuilds {
		return Skill{}, fmt.Errorf("course-selection skill configures more than %d Tencent Channel guilds", maxTencentCourseGuilds)
	}
	return Skill{Name: CourseSelectionName, Version: CourseSelectionVersion, Prompt: prompt, TencentGuildIDs: guildIDs}, nil
}

var tencentGuildIDPattern = regexp.MustCompile(`(?im)\bguild[_ -]?id\s*[:=：]\s*[` + "`" + `]?([0-9]{1,32})` + "`" + `?`)

func extractTencentGuildIDs(prompt string) []string {
	matches := tencentGuildIDPattern.FindAllStringSubmatch(prompt, -1)
	seen := make(map[string]struct{}, len(matches))
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 || match[1] == "" {
			continue
		}
		if _, ok := seen[match[1]]; ok {
			continue
		}
		seen[match[1]] = struct{}{}
		ids = append(ids, match[1])
	}
	return ids
}
