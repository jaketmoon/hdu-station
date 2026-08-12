package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTencentSearchGuildFeedUsesOnlyReadOnlySearchCommand(t *testing.T) {
	tool := newTencentSearchGuildFeedToolForTest("tencent-channel-cli", func(_ context.Context, binary string, arguments ...string) ([]byte, error) {
		if binary != "tencent-channel-cli" {
			t.Errorf("binary = %q", binary)
		}
		want := []string{"feed", "search-guild-feeds", "--guild-id", "123456", "--query", "选课", "--json"}
		if strings.Join(arguments, "\x00") != strings.Join(want, "\x00") {
			t.Errorf("arguments = %#v, want %#v", arguments, want)
		}
		return []byte(`{"feeds":[]}`), nil
	})
	tool.allowedGuildIDs = map[string]struct{}{"123456": {}}

	arguments, err := json.Marshal(map[string]string{"guild_id": "123456", "query": "选课"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.Call(context.Background(), arguments)
	if err != nil || result.Text != `{"feeds":[]}` {
		t.Fatalf("unexpected result: %#v, %v", result, err)
	}
}

func TestTencentSearchGuildFeedSanitizesConnectorFieldsBeforeReturningToAgent(t *testing.T) {
	tool := newTencentSearchGuildFeedToolForTest("tencent-channel-cli", func(context.Context, string, ...string) ([]byte, error) {
		return []byte(`{"feeds":[{"feed_id":"internal-feed","guild_id":"internal-guild","title":"选课经验","content":"这是一条社区信号","create_time":"2026-08-12 10:00:00","author":{"tiny_id":"private-user","nickname":"不应透传","name":"也不应透传"},"share_url":"https://pd.qq.com/s/example","raw":{"access_token":"secret-token"}}],"cookie":"pagination-secret"}`), nil
	})
	tool.allowedGuildIDs = map[string]struct{}{"123456": {}}

	result, err := tool.Call(context.Background(), json.RawMessage(`{"guild_id":"123456","query":"选课"}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"internal-feed", "internal-guild", "private-user", "不应透传", "secret-token", "pagination-secret", "feed_id", "guild_id", "access_token"} {
		if strings.Contains(result.Text, forbidden) {
			t.Fatalf("sanitized Tencent result contains %q: %s", forbidden, result.Text)
		}
	}
	for _, expected := range []string{"选课经验", "这是一条社区信号", "2026-08-12 10:00:00", "share_url"} {
		if !strings.Contains(result.Text, expected) {
			t.Fatalf("sanitized Tencent result lost %q: %s", expected, result.Text)
		}
	}
}

func TestTencentSearchGuildFeedRejectsNonJSONResponse(t *testing.T) {
	tool := newTencentSearchGuildFeedToolForTest("cli", func(context.Context, string, ...string) ([]byte, error) {
		return []byte("not-json"), nil
	})
	tool.allowedGuildIDs = map[string]struct{}{"123456": {}}
	_, err := tool.Call(context.Background(), json.RawMessage(`{"guild_id":"123456","query":"选课"}`))
	if err == nil || !strings.Contains(err.Error(), "decode Tencent Channel response") {
		t.Fatalf("unexpected non-JSON error: %v", err)
	}
}

func TestTencentSearchGuildFeedDoesNotFallbackWhenCLIIsMissing(t *testing.T) {
	tool := newTencentSearchGuildFeedToolForTest("", nil)
	tool.allowedGuildIDs = map[string]struct{}{"123456": {}}
	arguments := json.RawMessage(`{"guild_id":"123456","query":"选课"}`)
	_, err := tool.Call(context.Background(), arguments)
	if err == nil || !strings.Contains(err.Error(), "not installed in the Station data root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTencentCLIPathDoesNotUseHostPATH(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if path := tencentCLIPath(t.TempDir(), "darwin", "arm64"); path != "" {
		t.Fatalf("uninstalled Station CLI path = %q", path)
	}
}

func TestTencentCLIPathRequiresVerifiedStationInstall(t *testing.T) {
	root := t.TempDir()
	packageInfo, err := tencentPackageFor("windows", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	installRoot := filepath.Join(root, "tools", "tencent-channel-cli", tencentCLIVersion)
	if err := os.MkdirAll(installRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	binary := []byte("verified Station CLI")
	binaryPath := filepath.Join(installRoot, "tencent-channel-cli.exe")
	if err := os.WriteFile(binaryPath, binary, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(binary)
	marker, err := json.Marshal(tencentInstallMarker{PackageIntegrity: packageInfo.Integrity, BinarySHA256: hex.EncodeToString(digest[:])})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installRoot, "install.json"), marker, 0o600); err != nil {
		t.Fatal(err)
	}
	if path := tencentCLIPath(root, "windows", "amd64"); path != binaryPath {
		t.Fatalf("verified Station CLI path = %q, want %q", path, binaryPath)
	}
}

func TestTencentSearchGuildFeedRejectsUnconfiguredOrUnknownGuild(t *testing.T) {
	tool := newTencentSearchGuildFeedToolForTest("cli", func(context.Context, string, ...string) ([]byte, error) { return nil, nil })
	arguments := json.RawMessage(`{"guild_id":"123456","query":"选课"}`)
	if _, err := tool.Call(context.Background(), arguments); err == nil || !strings.Contains(err.Error(), "index is not configured") {
		t.Fatalf("unconfigured channel index was accepted: %v", err)
	}
	tool.allowedGuildIDs = map[string]struct{}{"654321": {}}
	if _, err := tool.Call(context.Background(), arguments); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("unknown guild was accepted: %v", err)
	}
}

func TestTencentSearchGuildFeedDefinitionPinsConfiguredGuilds(t *testing.T) {
	tool := NewTencentSearchGuildFeedToolForGuilds("cli", "654321", "123456", "654321")
	var definition Definition
	definition = tool.Definition()
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
	if strings.Join(schema.Properties.GuildID.Enum, ",") != "123456,654321" {
		t.Fatalf("guild enum = %#v", schema.Properties.GuildID.Enum)
	}
}

func TestTencentSearchGuildFeedRejectsNonNumericGuildID(t *testing.T) {
	tool := newTencentSearchGuildFeedToolForTest("cli", func(context.Context, string, ...string) ([]byte, error) {
		return []byte(`{"feeds":[]}`), nil
	})
	tool.allowedGuildIDs = map[string]struct{}{"123456": {}}
	_, err := tool.Call(context.Background(), json.RawMessage(`{"guild_id":"guild-1","query":"选课"}`))
	if err == nil || !strings.Contains(err.Error(), "only 1 to 32 digits") {
		t.Fatalf("non-numeric guild ID was accepted: %v", err)
	}
}
