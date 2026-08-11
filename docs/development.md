# Development workflow

## Vertical slices

Implement one user-visible capability at a time:

1. define the Go use case and boundary;
2. add deterministic unit tests;
3. bind it to Wails;
4. implement loading, empty, error and success UI states;
5. exercise the real local path where credentials and platform availability permit;
6. commit the verified slice independently.

## Credential handling

- Real values live only in ignored `.env` or the product YAML file.
- Tests use fake values and local HTTP servers.
- Never place tokens in command arguments, snapshots, fixtures or logs.
- Tencent credentials remain owned by the official CLI/keychain.

## Reuse from hdu-mate

Copy only files needed by the active slice. Preserve useful UI, Agent event and tool execution concepts, while removing HTTP server, tenant, ACL, Shared Runner, Kubernetes and database assumptions in the same commit.

## Required checks

```bash
make test
make frontend-check
make build
```

Before a phase is declared complete, perform an independent code-quality and over-design review and resolve material findings.

