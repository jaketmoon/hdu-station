# HDU Station Architecture

## 1. Runtime topology

```text
Wails WebView (React)
        |
        v
Go desktop core
  |-- local YAML config + SQLite
  |-- provider-neutral Agent loop
  |-- trusted read-only tools
  |     |-- HDUHelp Neo CLI
  |     |-- Tencent Channel CLI
  |     `-- Web Search / Fetch
  `-- sandbox.Executor
        |-- macOS Virtualization.framework/vfkit
        `-- Windows WSL2 private distro
```

The desktop core is the only composition root. There is no HTTP application server, multi-user control plane, PostgreSQL, Redis, Shared Runner or Docker dependency.

## 2. One conversation entry

The UI exposes one chat surface. The Agent selects tools from the request and reports tool progress in the same timeline. Sandbox is an execution invariant, not a user-facing mode:

- normal model inference and trusted read-only integrations do not boot the VM;
- free-form commands and generated code always call `sandbox.Executor`;
- if the Sandbox is unavailable, execution fails visibly instead of falling back to the host.

## 3. Model boundary

The Agent uses one internal message, content-block, tool-call and streaming event model. Provider adapters implement:

- OpenAI Chat Completions;
- OpenAI Responses;
- Anthropic Messages.

Provider protocol is explicit in configuration. Compatible OpenAI endpoints are not capability-probed by guessing. At least one configured provider is required to start a conversation.

## 4. Trusted tools

Trusted tools are typed Go adapters with bounded inputs and outputs. The initial registry contains only read operations.

### Campus

Station launches the system browser for HDUHelp Neo device authorization, saves the resulting campus key to YAML, and invokes packaged Neo CLI read commands. Course selection, withdrawal and profile mutation are out of scope.

### Tencent Channel

The official CLI owns its login credential. Station exposes only joined-channel lookup, in-guild feed search, feed detail, comments and replies. It never joins, publishes, comments, likes, deletes or manages members.

The three HDU channel URLs are data owned by the course-selection Skill and must not be duplicated in global configuration.

### Web

`web_search` and `web_fetch` are model-independent tools. DuckDuckGo HTML is the no-key best-effort backend; Brave and Tavily are optional configured providers. Fetch enforces URL scheme, response size and timeout limits.

## 5. Skills

Skills are local, versioned directories containing `SKILL.md`, optional references, scripts and agent metadata. The initial course-selection Skill combines:

1. Neo course and timetable facts;
2. exact-channel Tencent searches;
3. public Web evidence;
4. deterministic recall wording for good/easy-course research;
5. source-labelled synthesis without enrollment writes.

## 6. Local data

All Station-owned persistent files live under one platform application-data root:

```text
HDU Station/
  config.yaml
  station.db
  skills/
  tools/
  workspaces/
  sandbox/
  cache/
  logs/
```

Development `.env` values bootstrap tests only and are never copied into source control. Product configuration is YAML by explicit project decision.

## 7. Sandbox platforms

The Sandbox image is downloaded on first code execution and verified by digest.

- macOS 13+ arm64 uses Virtualization.framework through a vfkit-compatible backend.
- Windows 10/11 x64 imports a dedicated WSL2 distro below the Station data root. Drive automount and Windows interop are disabled.

The platform interface covers availability, installation, start, execution, stop and purge. Platform-specific code must not silently degrade to host execution.

## 8. Cleanup and uninstall

`Clear all data` stops the Sandbox and removes every Station-owned item below the application-data root plus credentials created by Station. `Uninstall` additionally schedules removal of the app after shutdown. Shared OS components such as WSL2 and WebView2, and unrelated WSL distributions, are never removed.

## 9. Dependency direction

```text
frontend -> Wails bindings -> app use cases
app -> agent / config / storage / tools / sandbox interfaces
providers and platform backends -> interfaces
main -> composition root
```

Packages must stay concrete until a second implementation or a test seam proves an interface useful.

