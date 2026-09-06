## Behavior Guidelines:

- Do not preserve backward compatibility. Remove obsolete code paths instead of adding compatibility layers, fallbacks, or migrations.

- Choose the simplest implementation that fully meets the current requirements. Avoid speculative abstractions, configuration, and indirection.
- Grow the system in layers. Start from the smallest version that works end to end, and add each new capability on top of a product that already works. Never trade a working product for unfinished complexity.
- Keep components modular and concerns clearly separated.
- Prefer established, well-maintained libraries when they reduce overall complexity or improve reliability. Do not reimplement common functionality without a clear reason.
- Rely on the dependencies already in the project before writing your own implementation or adding packages. Do not assume a library lacks a capability without checking its documentation and types.
- Make architectural decisions for the long term. Do not accept a stopgap that only works for now and is meant to be replaced later.
- When outputting Markdown-formatted content, please avoid using bold and italic styles unless you deem them necessary.

## Project Guidelines

- Keep user-facing documentation and CLI messages in English unless a localized interface is explicitly requested.
- Treat `config.yaml` as the source configuration and `clash.yaml` as generated output; do not commit generated subscription changes unless they are intentionally being published.
- Preserve protocol semantics when parsing or normalizing nodes. Add focused tests for every new protocol field or URI form.
- Run `gofmt`, `go vet ./...`, and `go test ./...` after Go changes.
- Keep GitHub Actions changes aligned with the local commands documented in `README.md`.
- Do not add credentials, downloaded Mihomo binaries, generated artifacts, or machine-specific files to the repository.
- Keep documentation concise and update `ChangeLog.md` with a versioned entry for each release, and keep the version constant in `internal/version/version.go` in sync with the latest `ChangeLog.md` entry.
