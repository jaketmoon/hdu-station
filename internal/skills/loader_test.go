package skills

import (
	"strings"
	"testing"
)

func TestSimulationManagementPausedButReadAndCollectionRemain(t *testing.T) {
	prompt, _ := CourseSelection()
	for _, name := range []string{"course-simulation-management", "course-simulation-replanning"} {
		if _, err := documents.ReadFile(name + "/SKILL.md"); err != nil {
			t.Fatalf("disabled skill must remain available in source: %v", err)
		}
		if strings.Contains(prompt, "name: "+name+"\n") {
			t.Fatalf("disabled skill loaded: %s", name)
		}
	}
	for _, name := range []string{"timetable-fit", "course-collection-management"} {
		if !strings.Contains(prompt, "name: "+name+"\n") {
			t.Fatalf("retained skill missing: %s", name)
		}
	}
}
