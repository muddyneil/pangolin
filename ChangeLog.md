# ChangeLog

## 0.2.2 - 2026-09-08

- Deployments now run automatically every two hours (at 00:13, 02:13, ... 22:13 UTC) instead of once a day, keeping the published subscription fresher. The offset from the top of the hour avoids the busiest GitHub server window.

## 0.2.1 - 2026-09-07

- Deployments now run automatically every day at 04:00 UTC in addition to the manual trigger.
- Added a `mihomo_version` workflow input to pin the Mihomo release tag (latest by default) and a smoke test that verifies the downloaded core before generation.
- Hardened the Mihomo download: transient GitHub API failures now fall through to the fallback pattern instead of aborting the step, and the core is cached by release tag across runs.
- CI now checks `gofmt`, cancels superseded runs, enforces a 30-minute job timeout, and skips documentation-only changes.
- Added Dependabot tracking for GitHub Actions and Go module updates.

## 0.2.0 - 2026-09-06

- Added SSR URI (ssr://) parsing with base64url links and obfs/proto/remarks parameters.
- Fixed HTTPS proxy credentials: the username is no longer dropped for TLS HTTP proxies.
- Accepted hysteria2 nodes authenticated via auth= / auth-str in addition to userinfo passwords.
- Applied conventional default ports (http 80, https 443, socks5 1080, TLS protocols 443) when a URI omits the port.
- Normalized proxy types case-insensitively instead of silently dropping them.
- Redesigned benchmark eligibility: a node is published when at least two thirds of delay probes succeed with at least one complete round, instead of requiring every round to pass; transient probe bursts no longer discard healthy nodes.
- Fixed a data race in Mihomo log capture, reported dropped node names, and surfaced the primary source error when all fetch URLs fail.
- Wrote the generated subscription as 0644 and skipped Base64 compaction for clearly non-Base64 payloads.
- Validated the eligibility threshold with a real GitHub Actions A/B run (109 candidates, 58 vs 50 published, median latency unchanged).
- Version metadata and the fetch User-Agent now derive from a single version constant.

## 0.1.0 - 2026-09-06

- Added GitHub Actions CI for vetting, tests, and Linux AMD64 builds.
- Added manual GitHub Pages deployment with the official Mihomo core.
- Added source fallbacks, JSON/YAML/Base64 parsing, URI protocol parsing, filtering, and deduplication.
- Added support for SS, SSR, VMess, VLESS, Trojan, Hysteria, Hysteria2, TUIC, HTTP, and SOCKS5 nodes.
- Added transport, TLS, Reality, authentication, and protocol-specific URI normalization.
- Added round-robin source merging, unique names, Mihomo benchmarking, quality thresholds, and validated subscription output.
- Added regional groups, AI routing, fallback routes, and DIRECT fallback handling.
