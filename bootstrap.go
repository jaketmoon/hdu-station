package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/jaketmoon/hdu-station/internal/agent"
	"github.com/jaketmoon/hdu-station/internal/appdata"
	"github.com/jaketmoon/hdu-station/internal/config"
	"github.com/jaketmoon/hdu-station/internal/sandbox"
	stationSkills "github.com/jaketmoon/hdu-station/internal/skills"
	"github.com/jaketmoon/hdu-station/internal/storage"
	stationtools "github.com/jaketmoon/hdu-station/internal/tools"
	"github.com/joho/godotenv"
)

func createApplication() (*App, error) {
	root, err := appdata.ResolveRoot()
	if err != nil {
		return nil, err
	}
	values, err := readOptionalEnvironment(".env")
	if err != nil {
		return nil, err
	}
	lookup := func(key string) string {
		if value, ok := os.LookupEnv(key); ok {
			return value
		}
		return values[key]
	}
	return createApplicationAt(root, config.FromEnvironment(lookup))
}

func createApplicationAt(root string, seed config.Config) (*App, error) {
	store := config.NewStore(root)
	cfg, err := store.LoadOrCreate(seed)
	if err != nil {
		return nil, fmt.Errorf("load Station configuration: %w", err)
	}
	skill, err := stationSkills.LoadCourseSelection(root)
	if err != nil {
		return nil, fmt.Errorf("load course-selection skill: %w", err)
	}
	sessions, err := storage.Open(root)
	if err != nil {
		return nil, fmt.Errorf("open Station storage: %w", err)
	}
	conversations, err := sessions.ListConversations(context.Background())
	if err != nil {
		_ = sessions.Close()
		return nil, fmt.Errorf("load Station conversations: %w", err)
	}
	if len(conversations) == 0 {
		if _, err := sessions.CreateConversation(context.Background(), "选课助手"); err != nil {
			_ = sessions.Close()
			return nil, fmt.Errorf("create initial conversation: %w", err)
		}
	}
	executor := sandbox.NewCurrentAtWithImage(root, sandbox.ImageSource{
		URL:       cfg.Sandbox.ImageURL,
		SHA256:    cfg.Sandbox.ImageSHA256,
		Signature: cfg.Sandbox.ImageSignature,
		PublicKey: cfg.Sandbox.ImagePublicKey,
	})
	registry, err := newToolRegistry(root, cfg, http.DefaultClient, executor, skill.TencentGuildIDs)
	if err != nil {
		_ = sessions.Close()
		return nil, fmt.Errorf("register Station tools: %w", err)
	}
	engine := agent.New(cfg, http.DefaultClient)
	engine.SetSystemPrompt(skill.Prompt)
	return newAppWithStore(root, cfg, sessions, engine, executor, registry), nil
}

func newReadOnlyToolRegistry(cfg config.Config, client *http.Client) (*stationtools.Registry, error) {
	return stationtools.NewReadOnlyRegistry(trustedReadOnlyTools(cfg, client)...)
}

func newToolRegistry(dataRoot string, cfg config.Config, client *http.Client, executor sandbox.Executor, guildIDs []string) (*stationtools.Registry, error) {
	registeredTools := make([]stationtools.Tool, 0, 4)
	registeredTools = append(registeredTools, trustedReadOnlyToolsAt(dataRoot, cfg, client, guildIDs)...)
	if executor != nil {
		registeredTools = append(registeredTools, stationtools.NewSandboxExecuteTool(executor))
	}
	return stationtools.NewRegistry(registeredTools...)
}

func newToolRegistryWithTencentBinary(dataRoot string, cfg config.Config, client *http.Client, executor sandbox.Executor, binary string, guildIDs []string) (*stationtools.Registry, error) {
	registeredTools := make([]stationtools.Tool, 0, 4)
	registeredTools = append(registeredTools, stationtools.NewWebSearchTool(cfg.WebSearch, client), stationtools.NewWebFetchTool(client))
	if len(guildIDs) > 0 {
		registeredTools = append(registeredTools, stationtools.NewTencentSearchGuildFeedToolForGuilds(binary, guildIDs...))
	}
	registeredTools = append(registeredTools, stationtools.NewNeoMCPAcademicTools(cfg.Campus.Key, stationtools.NewNeoMCPHTTPClient())...)
	if executor != nil {
		registeredTools = append(registeredTools, stationtools.NewSandboxExecuteTool(executor))
	}
	return stationtools.NewRegistry(registeredTools...)
}

func trustedReadOnlyTools(cfg config.Config, client *http.Client) []stationtools.Tool {
	return trustedReadOnlyToolsAt("", cfg, client, nil)
}

func trustedReadOnlyToolsAt(dataRoot string, cfg config.Config, client *http.Client, guildIDs []string) []stationtools.Tool {
	registeredTools := []stationtools.Tool{
		stationtools.NewWebSearchTool(cfg.WebSearch, client),
		stationtools.NewWebFetchTool(client),
	}
	if len(guildIDs) > 0 {
		registeredTools = append(registeredTools, stationtools.NewTencentSearchGuildFeedToolForGuilds(stationtools.TencentCLIPathAt(dataRoot), guildIDs...))
	}
	// Neo MCP is the primary campus path. The legacy CLI adapter remains in the
	// package for migration and deterministic tests, but it is not registered in
	// the desktop composition root and therefore cannot become an accidental
	// host-command fallback.
	registeredTools = append(registeredTools, stationtools.NewNeoMCPAcademicTools(cfg.Campus.Key, stationtools.NewNeoMCPHTTPClient())...)
	return registeredTools
}

func readOptionalEnvironment(path string) (map[string]string, error) {
	values, err := godotenv.Read(path)
	if err == nil {
		return values, nil
	}
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	return nil, fmt.Errorf("read development environment: %w", err)
}
