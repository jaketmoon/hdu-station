package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const maxTencentOutput = 1 << 20

type CommandRunner func(context.Context, string, ...string) ([]byte, error)

type TencentSearchGuildFeedTool struct {
	binary          string
	run             CommandRunner
	allowedGuildIDs map[string]struct{}
}

func NewTencentSearchGuildFeedTool(binary string) *TencentSearchGuildFeedTool {
	return &TencentSearchGuildFeedTool{binary: binary, run: runCommand}
}

func NewTencentSearchGuildFeedToolForGuilds(binary string, guildIDs ...string) *TencentSearchGuildFeedTool {
	tool := NewTencentSearchGuildFeedTool(binary)
	tool.allowedGuildIDs = make(map[string]struct{}, len(guildIDs))
	for _, guildID := range guildIDs {
		guildID = strings.TrimSpace(guildID)
		if isTencentGuildID(guildID) {
			tool.allowedGuildIDs[guildID] = struct{}{}
		}
	}
	return tool
}

func TencentCLIPathAt(dataRoot string) string {
	return tencentCLIPath(dataRoot, runtime.GOOS, runtime.GOARCH)
}

func tencentCLIPath(dataRoot, goos, goarch string) string {
	if strings.TrimSpace(dataRoot) == "" {
		return ""
	}
	packageInfo, err := tencentPackageFor(goos, goarch)
	if err != nil {
		return ""
	}
	name := "tencent-channel-cli"
	if goos == "windows" {
		name += ".exe"
	}
	binaryPath := filepath.Join(dataRoot, "tools", "tencent-channel-cli", tencentCLIVersion, name)
	markerPath := filepath.Join(filepath.Dir(binaryPath), "install.json")
	if valid, err := validInstalledTencentCLI(binaryPath, markerPath, packageInfo.Integrity); err == nil && valid {
		return binaryPath
	}
	return ""
}

func newTencentSearchGuildFeedToolForTest(binary string, runner CommandRunner) *TencentSearchGuildFeedTool {
	return &TencentSearchGuildFeedTool{binary: binary, run: runner}
}

func (tool *TencentSearchGuildFeedTool) Definition() Definition {
	guildIDs := make([]string, 0, len(tool.allowedGuildIDs))
	for guildID := range tool.allowedGuildIDs {
		guildIDs = append(guildIDs, guildID)
	}
	sort.Strings(guildIDs)
	guildSchema := map[string]any{
		"type":      "string",
		"minLength": 1,
		"maxLength": 32,
		"pattern":   "^[0-9]{1,32}$",
	}
	if len(guildIDs) > 0 {
		guildSchema["enum"] = guildIDs
	}
	parameters, _ := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"guild_id": guildSchema,
			"query": map[string]any{
				"type":      "string",
				"minLength": 2,
				"maxLength": 240,
			},
		},
		"required":             []string{"guild_id", "query"},
		"additionalProperties": false,
	})
	return Definition{
		Name:        "tencent_search_guild_feed",
		Description: "Search posts inside a Tencent Channel that the user has already joined.",
		Parameters:  parameters,
	}
}

func (tool *TencentSearchGuildFeedTool) ReadOnly() bool { return true }

func (tool *TencentSearchGuildFeedTool) Call(ctx context.Context, arguments json.RawMessage) (Result, error) {
	var input struct {
		GuildID string `json:"guild_id"`
		Query   string `json:"query"`
	}
	if err := json.Unmarshal(arguments, &input); err != nil {
		return Result{}, fmt.Errorf("decode Tencent Channel arguments: %w", err)
	}
	input.GuildID = strings.TrimSpace(input.GuildID)
	input.Query = strings.TrimSpace(input.Query)
	if !isTencentGuildID(input.GuildID) {
		return Result{}, errors.New("Tencent Channel guild_id must contain only 1 to 32 digits")
	}
	if len(tool.allowedGuildIDs) == 0 {
		return Result{}, errors.New("Tencent Channel course-selection channel index is not configured")
	}
	if _, ok := tool.allowedGuildIDs[input.GuildID]; !ok {
		return Result{}, errors.New("Tencent Channel guild_id is outside the configured course-selection channels")
	}
	if len([]rune(input.Query)) < 2 || len([]rune(input.Query)) > 240 {
		return Result{}, errors.New("Tencent Channel query must be between 2 and 240 characters")
	}
	if tool.binary == "" {
		return Result{}, errors.New("Tencent Channel CLI is not installed in the Station data root")
	}
	if tool.run == nil {
		return Result{}, errors.New("Tencent Channel CLI runner is not initialized")
	}
	output, err := tool.run(ctx, tool.binary,
		"feed", "search-guild-feeds",
		"--guild-id", input.GuildID,
		"--query", input.Query,
		"--json",
	)
	if err != nil {
		return Result{}, fmt.Errorf("search Tencent Channel guild feed: %w", err)
	}
	if len(output) > maxTencentOutput {
		return Result{}, errors.New("Tencent Channel response exceeds the 1 MiB limit")
	}
	sanitized, err := sanitizeTencentCLIJSON(output)
	if err != nil {
		return Result{}, fmt.Errorf("decode Tencent Channel response: %w", err)
	}
	return Result{Text: string(sanitized)}, nil
}

// sanitizeTencentCLIJSON keeps community content useful to the model while
// removing connector-owned credentials, pagination state, and identifiers
// that are only useful for a follow-up write operation. Station registers no
// Tencent write tools, so forwarding those fields would increase privacy
// exposure without adding decision value.
func sanitizeTencentCLIJSON(output []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(output))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("response contains multiple JSON values")
		}
		return nil, err
	}
	return json.Marshal(sanitizeTencentValue(value))
}

func sanitizeTencentValue(value any) any {
	switch typed := value.(type) {
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, sanitizeTencentValue(item))
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if omitTencentField(key) {
				continue
			}
			result[key] = sanitizeTencentValue(item)
		}
		return result
	default:
		return value
	}
}

func omitTencentField(key string) bool {
	forbidden := map[string]struct{}{
		"accesstoken": {}, "refreshtoken": {}, "authorization": {}, "cookie": {},
		"devicecode": {}, "qrcode": {}, "appsecret": {}, "secret": {}, "state": {},
		"guildid": {}, "channelid": {}, "tinyid": {}, "userid": {}, "feedid": {},
		"commentid": {}, "replyid": {}, "authorid": {}, "targetuserid": {},
		"roleid": {}, "levelroleid": {}, "id": {}, "raw": {}, "channelinfo": {},
		"channelsign": {}, "createtimeraw": {}, "attachinfo": {}, "feedattachinfo": {},
		"feedattchinfo": {}, "nextpagecookie": {}, "cookievalue": {},
		"phone": {}, "mobile": {}, "email": {}, "qq": {}, "realname": {},
		"nickname": {}, "username": {}, "displayname": {}, "authorname": {},
		"creatorname": {}, "author": {}, "creator": {}, "publisher": {},
		"user": {}, "member": {}, "owner": {},
	}
	_, found := forbidden[normalizeTencentFieldName(key)]
	return found
}

func normalizeTencentFieldName(key string) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(key) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			builder.WriteRune(character)
		}
	}
	return builder.String()
}

func isTencentGuildID(value string) bool {
	if value == "" || len(value) > 32 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func runCommand(ctx context.Context, binary string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, binary, arguments...)
	output, err := command.Output()
	if err != nil {
		return nil, err
	}
	return output, nil
}
