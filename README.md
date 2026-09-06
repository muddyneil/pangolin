# Pangolin

Pangolin is a Go-based proxy subscription generator for Mihomo and Clash-compatible clients. It collects proxy nodes from configured subscription sources, normalizes common protocols, removes invalid or duplicate nodes, benchmarks node quality, builds a validated subscription, and publishes the result through GitHub Pages.

## Features

- Fetches subscriptions from primary URLs with optional fallback URLs.
- Parses JSON, YAML, Base64-encoded subscriptions, and common proxy URIs.
- Supports common SS, SSR, VMess, VLESS, Trojan, Hysteria, Hysteria2, TUIC, HTTP, and SOCKS5 configurations.
- Normalizes transport, TLS, Reality, authentication, and protocol-specific fields.
- Filters invalid, oversized, unsupported, and duplicate nodes.
- Merges multiple sources with round-robin distribution and unique node names.
- Runs Mihomo delay checks with multiple rounds, median latency, jitter, and quality thresholds.
- Generates regional proxy groups, AI routing groups, fallback routes, and a DIRECT fallback.
- Validates the generated configuration with Mihomo before publishing it.
- Provides GitHub Actions CI and manual GitHub Pages deployment.

## Repository Files

```text
config.yaml                 # Subscription sources maintained by the repository
clash.yaml                  # Generated subscription output
cmd/pangolin                 # CLI entry point
internal/                    # Application, source, benchmark, and subscription logic
.github/workflows/ci.yml     # Tests, vetting, and Linux build
.github/workflows/deploy.yml # Manual subscription deployment
```

## Configure Sources

Edit `config.yaml`. Each source has a name, a primary URL, and an optional list of fallback URLs:

```yaml
sources:
  - name: "example"
    primary: "https://example.com/clash.yaml"
    fallbacks:
      - "https://example.com/backup.yaml"
```

The source must expose a supported proxy subscription. Mihomo is not configured in `config.yaml`; the deployment workflow downloads the official Linux AMD64 core from the MetaCubeX/mihomo releases.

## GitHub Actions Deployment

1. Push the repository to GitHub with a configured `config.yaml`.
2. In repository settings, enable GitHub Pages with `GitHub Actions` as the source.
3. Open the Actions tab.
4. Select `Deploy subscription`.
5. Click `Run workflow`.
6. Wait for the workflow to download Mihomo, generate `clash.yaml`, and deploy the Pages artifact.

The generated subscription is available at:

```text
https://<owner>.github.io/<repository>/clash.yaml
```

The deployment workflow requires Pages write permission and the GitHub Pages environment. It downloads the latest compatible official Mihomo Linux AMD64 release at deployment time.

## Local Development

Requirements:

- Go version declared in `go.mod`
- A Mihomo core binary

For a local Linux run, place the executable at `tools/mihomo/mihomo`, then run:

```sh
go run ./cmd/pangolin
```

To use an alternate source configuration:

```sh
go run ./cmd/pangolin --config path/to/config.yaml
```

The generated subscription is written to `clash.yaml` in the repository root.

Run the checks locally with:

```sh
go vet ./...
go test ./...
go build -trimpath -ldflags='-s -w' -o dist/pangolin-linux-amd64 ./cmd/pangolin
```

To print the version:

```sh
go run ./cmd/pangolin --version
```
