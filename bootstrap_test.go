package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jaketmoon/hdu-station/internal/config"
)

func TestCreateApplicationSeedsAndReloadsConfiguration(t *testing.T) {
	root := t.TempDir()
	seed := config.FromEnvironment(func(key string) string {
		values := map[string]string{
			"HDU_STATION_CAMPUS_KEY":     "campus-seed",
			"HDU_STATION_OPENAI_API_KEY": "openai-seed",
			"HDU_STATION_OPENAI_MODEL":   "gpt-seed",
		}
		return values[key]
	})

	app, err := createApplicationAt(root, seed)
	if err != nil {
		t.Fatal(err)
	}
	if !app.Bootstrap().Configuration.CampusConfigured {
		t.Fatal("campus configuration should be reported as configured")
	}
	if got := app.Settings().CampusAuth.Method; got != "pat" {
		t.Fatalf("campus auth method = %q, want pat", got)
	}

	secondSeed := config.Default()
	reloaded, err := createApplicationAt(root, secondSeed)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Bootstrap().Configuration.DefaultProvider != "openai" {
		t.Fatal("existing YAML must win over a later environment seed")
	}
	if _, err := os.Stat(filepath.Join(root, "config.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestCreateApplicationRegistersNeoMCPAcademicReadTools(t *testing.T) {
	app, err := createApplicationAt(t.TempDir(), config.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)

	definitions := app.ReadOnlyTools()
	seen := map[string]bool{}
	for _, definition := range definitions {
		seen[definition.Name] = true
	}
	for _, name := range []string{
		"hdu_academic_class_search",
		"hdu_academic_course_selection",
		"hdu_academic_schedule",
		"hdu_academic_schedule_now",
	} {
		if !seen[name] {
			t.Fatalf("registered tools do not include %q: %#v", name, definitions)
		}
	}
	allDefinitions := app.Tools()
	allSeen := map[string]bool{}
	for _, definition := range allDefinitions {
		allSeen[definition.Name] = true
	}
	if !allSeen["sandbox_execute"] {
		t.Fatalf("all Agent tools do not include Sandbox execution: %#v", allDefinitions)
	}
	for _, definition := range definitions {
		if definition.Name == "sandbox_execute" {
			t.Fatal("ReadOnlyTools exposed the mutating Sandbox executor")
		}
	}
}

func TestCreateApplicationLoadsCourseSelectionSkillIntoAgent(t *testing.T) {
	app, err := createApplicationAt(t.TempDir(), config.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)

	if app.engine == nil {
		t.Fatal("application did not initialize the Agent")
	}
	prompt := app.engine.SystemPrompt()
	for _, required := range []string{
		"name: course-selection",
		"只做查询和解释",
		"tencent_search_guild_feed",
		"不确定性",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("Agent system prompt is missing course-selection rule %q", required)
		}
	}
}

func TestCreateApplicationRegistersTencentSearchForCourseSelectionChannels(t *testing.T) {
	app, err := createApplicationAt(t.TempDir(), config.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	var definitionFound bool
	for _, definition := range app.ReadOnlyTools() {
		if definition.Name == "tencent_search_guild_feed" {
			definitionFound = true
			var schema struct {
				Properties struct {
					GuildID struct {
						Enum []string `json:"enum"`
					} `json:"guild_id"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(definition.Parameters, &schema); err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(schema.Properties.GuildID.Enum, ","); got != "17780811658383776,83580271760343397,90666891645605969" {
				t.Fatalf("Tencent guild enum = %q", got)
			}
		}
	}
	if !definitionFound {
		t.Fatal("course-selection channels should register Tencent read-only search")
	}
	for _, definition := range app.Tools() {
		if definition.Name == "tencent_publish_feed" || definition.Name == "tencent_do_comment" || definition.Name == "tencent_kick_guild_member" {
			t.Fatalf("Tencent write tool must not be registered: %q", definition.Name)
		}
	}
}

func TestCallReadOnlyToolCannotInvokeSandboxExecute(t *testing.T) {
	app, err := createApplicationAt(t.TempDir(), config.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer app.shutdown(nil)
	if _, err := app.CallReadOnlyTool("sandbox_execute", `{"command":"printf","args":["unsafe"]}`); err == nil || !strings.Contains(err.Error(), "not read-only") {
		t.Fatalf("sandbox_execute crossed read-only binding: %v", err)
	}
}

func TestReadOptionalEnvironmentAllowsMissingFile(t *testing.T) {
	values, err := readOptionalEnvironment(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 0 {
		t.Fatalf("unexpected values: %#v", values)
	}
}
