package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCourseSelectionUsesEmbeddedVersionedSkill(t *testing.T) {
	skill, err := LoadCourseSelection(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if skill.Name != CourseSelectionName || skill.Version != CourseSelectionVersion {
		t.Fatalf("unexpected skill metadata: %#v", skill)
	}
	if !strings.Contains(skill.Prompt, "水课") || !strings.Contains(skill.Prompt, "17780811658383776") {
		t.Fatalf("course-selection policy was not loaded: %s", skill.Prompt)
	}
	if got := strings.Join(skill.TencentGuildIDs, ","); got != "83580271760343397,90666891645605969,17780811658383776" {
		t.Fatalf("embedded Tencent guild IDs = %q", got)
	}
	for _, required := range []string{
		"hdu_academic_class_search",
		"hdu_academic_course_selection",
		"hdu_academic_schedule",
		"tencent_search_guild_feed",
		"web_search",
		"官方/结构化事实",
		"不确定性",
	} {
		if !strings.Contains(skill.Prompt, required) {
			t.Fatalf("course-selection skill is missing %q", required)
		}
	}
}

func TestLoadCourseSelectionUsesOnlyTheThreeVerifiedHDUChannels(t *testing.T) {
	skill, err := LoadCourseSelection("")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"83580271760343397", "90666891645605969", "17780811658383776"}
	if strings.Join(skill.TencentGuildIDs, ",") != strings.Join(want, ",") {
		t.Fatalf("guild IDs = %#v, want %#v", skill.TencentGuildIDs, want)
	}
	for _, forbidden := range []string{"69926411661219920", "hdu-course-selection-1"} {
		if strings.Contains(skill.Prompt, forbidden) {
			t.Fatalf("course-selection skill includes forbidden channel %q", forbidden)
		}
	}
}

func TestLoadCourseSelectionAllowsDataRootOverride(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "skills", CourseSelectionName, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("custom skill"), 0o600); err != nil {
		t.Fatal(err)
	}
	skill, err := LoadCourseSelection(root)
	if err != nil {
		t.Fatal(err)
	}
	if skill.Prompt != "custom skill" {
		t.Fatalf("prompt = %q", skill.Prompt)
	}
}

func TestLoadCourseSelectionExtractsOnlyExplicitNumericTencentGuildIDs(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "skills", CourseSelectionName, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content := "course\n- guild_id: `123456`\n- guild_id: 123456\n- guild_id: 987654\n- hdu-course-selection-1\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	skill, err := LoadCourseSelection(root)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(skill.TencentGuildIDs, ",") != "123456,987654" {
		t.Fatalf("guild IDs = %#v", skill.TencentGuildIDs)
	}
}

func TestLoadCourseSelectionFailsClosedWhenMoreThanThreeGuildsAreConfigured(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "skills", CourseSelectionName, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content := "guild_id: 100\nguild_id: 200\nguild_id: 300\nguild_id: 400\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCourseSelection(root); err == nil || !strings.Contains(err.Error(), "more than 3") {
		t.Fatalf("unexpected guild limit error: %v", err)
	}
}
