# ChangeLog

## 0.2.8 - 2026-09-07

- Preserved the last published subscription when all sources fail, quality probing fails, coverage is incomplete, or no node passes validation.
- Rejected local and private source URLs and proxy server addresses before fetching or benchmarking.
- Required a GitHub-provided SHA-256 digest for downloaded Mihomo releases.
- Pinned GitHub Actions to immutable commit SHAs.
- Completed the pre-release security audit and verified 73 tests, `go vet ./...`, formatting checks, and `go test -race ./...` with CGO enabled.

## 0.2.7 - 2026-09-07

- Allowed pinning older Mihomo releases whose assets predate GitHub digests: the SHA-256 digest is verified when present, and a warning is emitted instead of failing the deployment when it is missing.
- Restricted the non-compatible Mihomo asset fallback to the standard v1 main build so the go120/go123 toolchain variants are no longer picked first.
- Added a CI test that loads the repository's `config.yaml`, so invalid runtime configuration fails the build instead of passing silently.
- Made the update script's run lookup tolerate a full ref such as `refs/heads/main` in addition to a short branch name.

## 0.2.6 - 2026-09-07

- Included `config.yaml` changes in CI so runtime configuration updates receive automated validation.
- Prevented shell injection through the manual `mihomo_version` workflow input by passing it through environment variables and validating release tags.
- Verified downloaded Mihomo release assets with their GitHub-provided SHA-256 digest before execution.
- Fixed the update script's workflow run lookup so it matches the triggered run by event, ref, and creation time instead of assuming the newest run is the correct one.

## 0.2.5 - 2026-09-07

- Fixed empty subscriptions to use Mihomo's built-in DIRECT proxy instead of a non-functional `127.0.0.1:1` SOCKS5 placeholder.
- Added support for legacy Base64-wrapped Shadowsocks URIs and SSR links with bracketed IPv6 servers.
- Fixed SSR parameter decoding so printable plain-text values are not corrupted by an ambiguous Base64 decode.
- Kept benchmark timeout estimation aligned with the minimum three probe rounds used by the actual probe logic.
- Added regression tests covering legacy SS links, SSR IPv6 links, plain-text SSR parameters, and the DIRECT fallback configuration.

## 0.2.4 - 2026-09-07

- Isolated un-named Mihomo config rejections: a rejected batch is now probed node by node so a malformed node whose error text does not mention it is dropped instead of aborting the whole benchmark, while rejections that survive isolation still fail loudly.
- Matched candidate names in Mihomo rejection output at word boundaries, so short node names like "us" no longer match ordinary English words inside error text.
- Surfaced benchmark deadline expiry as an error instead of silently publishing an empty node set, and kept the nodes measured before a partial probe failure instead of discarding them.
- Required an auth credential for Hysteria v1 nodes and normalized flat "network: http" transport fields into http-opts.
- Preferred the server location over the node name when they disagree in region detection.
- Added a regression test for SSR links whose base64 password ends in "/".

## 0.2.3 - 2026-09-06

- Removed the regional (HK-POOL, JP-POOL, US-POOL) and AI (AI-POOL) proxy groups, along with the AI domain rules that routed openai.com, chatgpt.com, and anthropic.com traffic. The generated subscription now ships only AUTO-FAST, ALL, FALLBACK, and PROXY, with FALLBACK chaining from AUTO-FAST to ALL as its last resort.

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
