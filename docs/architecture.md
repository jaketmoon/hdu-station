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
  |     |-- HDUHelp Neo MCP
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
- free-form commands and generated code are exposed only through the
  `sandbox_execute` Agent tool, which always calls `sandbox.Executor`;
- if the Sandbox is unavailable, execution fails visibly instead of falling back to the host.

## 3. Model boundary

The Agent uses one internal message, content-block, tool-call and streaming event model. `ChatStream` emits only bounded text deltas over the local Wails event bridge; the final assistant message is persisted after the same read-only tool loop completes. Provider adapters implement:

- OpenAI Chat Completions;
- OpenAI Responses;
- Anthropic Messages.

Provider protocol is explicit in configuration. Compatible OpenAI endpoints are not capability-probed by guessing. At least one configured provider is required to start a conversation.

The Agent boundary is bounded independently of provider behavior: a response
may contain at most eight tool calls, tool names are limited to 128 bytes,
tool arguments to 64 KiB, tool results returned to the next model request to
1 MiB, and one response may trigger at most six tool-loop rounds. Streaming
text and streaming tool arguments have the same bounded limits. Model HTTP
clients reject redirects because provider credentials are sent in request
headers.

The engine prepends an immutable safety instruction to every provider request:
campus, Tencent and web results are untrusted evidence, never system
instructions. Prompt-injection text in a post, fetched page or tool error cannot
authorize a new tool, reveal credentials, bypass Sandbox or perform a write.

## 4. Trusted tools

Trusted integrations are typed Go adapters with bounded inputs and outputs and
the Campus/QQ/Web registry contains only read operations. The Agent registry
also contains the explicit `sandbox_execute` tool; it is the sole non-read-only
tool and is bound directly to the injected Sandbox executor.

### Campus

Station saves the user-provided HDUHelp Neo PAT to private local YAML and invokes a fixed, stateless MCP endpoint from a host-only typed adapter. Its HTTP client has a bounded timeout and refuses redirects because the PAT is carried in `Authorization`. The PAT is never passed to the Sandbox or returned by Wails status DTOs, and the adapter exposes only four academic read tools. Neo's generated MCP contract groups HTTP parameters by location, so the three parameterized academic tools send `arguments.query`; `schedule.now` sends no location group. Local status progresses from `pat_configured_unverified` to `pat_verified` only after a successful read; 401/403 or an invalid token becomes `pat_rejected`, a tool-level missing-scope result becomes `scope_missing`, and transport failures become `unavailable`. Course selection, withdrawal and profile mutation are out of scope. The legacy CLI adapter remains isolated for migration/tests but is not registered by the desktop composition root.

Neo PAT scopes are checked per read tool: `academic:course:read` for class search, `academic:studentselection:read` for the current user's selections, and `academic:schedule:read` for timetable reads. A tool-level missing-scope error is surfaced as `scope_missing`, distinct from a rejected or revoked PAT. The current Neo server contract does not provide a Station-specific device authorization flow; Station therefore keeps device authorization disabled and does not impersonate another client. Station also never reads, copies or overwrites hduhelp-cli's separate configuration automatically: users explicitly provide the PAT in Station settings. A formal `hdu-station` public client must be registered and documented before a desktop login flow is added.

### Tencent Channel

The official CLI owns its login credential. If the CLI is absent, Station downloads
the pinned platform package from the official npm registry, verifies its SHA-512
integrity, and extracts only the expected binary below the Station data root.
After install, Station executes only that marker-and-digest-verified binary and
never resolves a same-named executable from the host PATH. Node is not required.
The current Station tool surface exposes only in-guild feed
search for numeric guild IDs explicitly configured by the course-selection Skill.
It does not yet expose guild discovery, cross-guild search, feed detail, comments,
replies or notifications. Station never joins, publishes, comments, likes,
deletes or manages members.

Before a Tencent search result enters the Agent loop, the host adapter parses the
CLI JSON and removes connector credentials, pagination state, write-operation
identifiers, raw provider objects and direct user-identity fields. It preserves
the community text, public timestamps and public share links needed for a
course-selection comparison. Malformed or multi-value output is rejected rather
than forwarded as opaque connector data.

The CLI login credential remains outside Station's YAML and Sandbox. A missing
CLI state file or an unjoined channel is reported as an integration error; Station
does not guess a guild ID, retry with another channel, or fall back to a host-side
HTTP implementation.

The three HDU channel URLs are data owned by the course-selection Skill and must not be duplicated in global configuration.

### Web

`web_search` and `web_fetch` are model-independent tools. DuckDuckGo HTML is the no-key best-effort backend; Brave and Tavily are optional configured providers. Fetch enforces URL scheme, response size and a maximum 15-second timeout. The Web-specific client disables redirects even when the composition root supplies a custom client, so a public URL cannot redirect the fetch into a loopback, private or link-local address.

## 5. Skills

Skills are local, versioned directories containing `SKILL.md`, optional references, scripts and agent metadata. The initial course-selection Skill combines:

1. Neo course and timetable facts;
2. exact-channel Tencent searches;
3. public Web evidence;
4. deterministic recall wording for good/easy-course research;
5. source-labelled synthesis without enrollment writes.

The skill is versioned under `internal/skills/course-selection/SKILL.md`, embedded in the desktop binary, and can be overridden below the single application data root at `skills/course-selection/SKILL.md`.

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

The Sandbox image is downloaded when its lifecycle is installed or started and verified by a local manifest and SHA-256 digest (and optionally an Ed25519 signature). On macOS, the pinned `vfkit` sidecar is likewise downloaded and verified below the Station data root; it is not a host prerequisite. The guest exposes a small versioned NDJSON runtime: vfkit bridges it to a host-only Unix socket on macOS, while WSL uses stdio; the host never invokes the requested command directly.

- macOS 13+ arm64 uses Virtualization.framework through a vfkit-compatible backend.
- Windows 10/11 x64 imports a dedicated WSL2 distro below the Station data root. Drive automount and Windows interop are disabled.

The platform interface covers availability, installation, start, execution, stop and purge. macOS launches vfkit with a signed raw disk, NAT networking and a virtio-vsock-to-Unix-socket bridge. Windows imports a signed tar archive into a Station-owned WSL2 distribution and records an ownership marker below the Station data root, so a same-named external distribution is never started or unregistered. The guest runtime independently enforces command, argument and output bounds; platform-specific code must not silently degrade to host execution.

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
