# Sandbox runtime contract

HDU Station never executes a user command through the desktop host shell. The
platform backends have separate lifecycle responsibilities, but both use a
Station-owned, digest-verified guest artifact.

## Image manifest

The image source is configured in the local YAML file (or the equivalent
development environment variables):

```yaml
sandbox:
  image_url: https://downloads.example.test/hdu-station/runtime.raw
  image_sha256: <64 hexadecimal characters>
  image_signature: <base64 Ed25519 signature of the SHA-256 digest>
  image_public_key: <base64 Ed25519 public key>
```

`HDU_STATION_SANDBOX_IMAGE_URL`, `HDU_STATION_SANDBOX_IMAGE_SHA256`,
`HDU_STATION_SANDBOX_IMAGE_SIGNATURE` and
`HDU_STATION_SANDBOX_IMAGE_PUBLIC_KEY` are the development environment names.
The URL and digest must be supplied together. The signature and public key are
optional as a pair during development; when present, the Ed25519 signature
must cover the raw 32-byte SHA-256 digest. The URL must be HTTPS and cannot
contain userinfo, a query, or a fragment. Redirects are checked by the same
rule. There is no built-in image URL or public key.

The lifecycle downloads and verifies the artifact before installing the
guest. It streams the response into a private temporary file, enforces a
16 GiB limit, compares the SHA-256 digest, and then renames the digest-named
artifact into the Station-owned image directory. Only after that does it
atomically replace `image.json`. A failed or interrupted download therefore
cannot install an unverified guest artifact.

The first product build does not ship a default image URL or digest. A release
must publish a platform-specific, signed image and provide its metadata through
the product configuration before the Sandbox can be installed. Without that
metadata, the visible result is “Sandbox image is not installed”; the desktop
never falls back to host execution.

The first implementation reads the resulting `HDU Station/sandbox/image.json`:

```json
{
  "platform": "darwin/arm64",
  "disk": "images/runtime.disk-<sha256>",
  "sha256": "<64 lowercase hexadecimal characters>"
}
```

Windows uses the same manifest shape with `archive` instead of `disk` and
`platform: "windows/amd64"`. Paths are relative to the sandbox directory;
absolute paths and `..` traversal are rejected. The image is hashed before a
guest is installed or booted. A manually provisioned manifest remains
supported when no download source is configured.

Artifact verification also rejects symbolic-link path components and checks the
resolved artifact remains below the Station-owned Sandbox directory. A digest
match on a file reached through an escaping link is not accepted.

The distribution service still needs to provide the signed artifact, pinned
digest and the release public key. The client verifies both the pinned
SHA-256 and, when configured, the Ed25519 signature over that digest; key
rotation/manifest-chain policy and the production distribution endpoint remain
release work. Until either a configured source or a manually provisioned
manifest is present, a missing manifest is reported as unavailable and no
command is run.

On macOS, `vfkit` is a separate Station-owned sidecar. The current pinned
release is downloaded from the fixed official vfkit GitHub release URL on the
first Sandbox install. Its short-lived signed redirect may only target GitHub's
official `release-assets.githubusercontent.com` HTTPS host; the resulting file
is verified against its pinned SHA-256 and stored below
`HDU Station/tools/vfkit/`. A valid marker and digest are required before it is
executed. The desktop build does not require `vfkit` to be pre-installed; a
missing or corrupt sidecar is reported as unavailable or an install error.

## Guest protocol

The Linux guest image contains the `hdu-station-runtime` binary from
`cmd/sandbox-runtime`. On macOS it listens on vsock port `1024`; `vfkit`
exposes the host side as a Unix socket. On Windows, the WSL backend invokes the
same binary with `--stdio` and exchanges one JSON object per line. Requests and
responses are one JSON object per line:

```json
{"version":1,"op":"execute","command":"python","args":["-c","print(1)"]}
{"version":1,"stdout":"1\n","stderr":"","exit_code":0}
```

The host sends one request and half-closes the Unix-socket write side; the
guest then reaches EOF after serving that request and closes the connection.
The host accepts exactly one bounded response line per call and requires the
entire connection to end after that line; same-line trailing bytes and a second
response line are rejected. WSL stdio uses the same strict decoder. This keeps
malformed or duplicated guest output from being mistaken for a successful
execution.

The Windows host bounds each WSL control-plane command to 30 seconds and each
guest runtime invocation to 11 minutes unless the caller supplies an earlier
deadline. A failed WSL distribution query is treated as an error, not as
"distribution absent", so installation and cleanup cannot operate on an
unverified external state.

`op: "ping"` is used during startup. The runtime executes commands with direct
argv semantics (it does not implicitly invoke a shell), applies a ten-minute
command timeout and bounded stdout/stderr, and never mounts the host
application directory. The guest image build must install it at
`/usr/local/bin/hdu-station-runtime` for the Windows path.

The runtime validates the request independently of the desktop tool: command
names are limited to 256 bytes, there can be at most 64 arguments, and each
argument is limited to 4096 bytes. NUL bytes are rejected both by the host
tool boundary and inside the guest runtime. These limits are enforced again
inside the guest so a caller that reaches the runtime directly cannot bypass
the host-side tool schema. A command timeout is returned as a runtime error
even if the terminated process reports an exit code.

Image builders can produce the static guest binaries with
`make sandbox-runtime`; the outputs are Linux amd64 and arm64 binaries under
`build/bin/`. The image assembly and distribution service must then place the
selected binary at `/usr/local/bin/hdu-station-runtime` and publish the
corresponding digest/signature metadata.

For the macOS arm64 release candidate, the repository provides
`make sandbox-image-darwin-arm64`. It pins and SHA-512-verifies Debian's arm64
cloud image, builds the guest runtime, creates a one-time NoCloud seed that
installs and enables the runtime service, and requires an Ed25519 signing key
before it can emit signed release metadata. The release engineer must perform
the one-time vfkit boot and runtime ping before finalizing the raw disk: the
script intentionally will not label an unbooted image as publishable.

## Platform behavior

- macOS arm64 launches `vfkit` with an EFI bootloader, a raw guest disk, NAT
  networking and a virtio-vsock-to-Unix-socket bridge. The host reports the
  backend ready and accepts execution only after the guest runtime answers a
  protocol `ping`; a running `vfkit` process alone is not readiness.
- Windows amd64 imports the archive into the fixed
  `HDU-Station-Sandbox` WSL2 distribution. The installer disables drive
  automount, Windows executable interop and automatic Windows PATH injection;
  user commands are sent to the guest runtime over stdio rather than passed as
  WSL control arguments. Station writes an ownership marker below its own data
  root and only reports, starts, stops, executes in, or unregisters a
  same-named distribution when that marker matches. A pre-existing distribution
  with the same display name is never treated as Station-owned.
- Unsupported platforms, missing images, missing guest runtimes and missing
  platform dependencies all return `sandbox.ErrUnavailable` or an explicit
  install error. There is no host fallback.
