# CLI Reference

Complete command-line reference for ktop.

## Basic Usage

```bash
ktop [flags]
```

ktop uses your kubeconfig file (from `$KUBECONFIG` or `~/.kube/config`) to connect to your cluster.

## Kubernetes Connection Flags

| Flag | Description |
|------|-------------|
| `--kubeconfig` | Path to kubeconfig file |
| `--context` | Kubeconfig context to use |
| `--cluster` | Kubeconfig cluster to use |
| `--user` | Kubeconfig user to use |
| `-n, --namespace` | Namespace to display (default: all) |
| `-A, --all-namespaces` | Show all namespaces |
| `-s, --server` | API server address |
| `--token` | Bearer token for authentication |

## Metrics Configuration Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--metrics-source` | `prometheus` | Source: `prometheus`, `metrics-server`, or `none` |
| `--prometheus-scrape-interval` | `5s` | How often to scrape (min: 5s) |
| `--prometheus-retention` | `1h` | How long to keep metrics (min: 5m) |
| `--prometheus-max-samples` | `10000` | Max samples per time series |
| `--prometheus-components` | `kubelet,cadvisor` | Components to scrape |
| `--ksm` | `auto` | `kube-state-metrics` enrichment mode: `auto` (auto-discovers via API service proxy and degrades silently) or `off` |
| `--ksm-endpoint` | `""` | Explicit `kube-state-metrics` endpoint (`http(s)://...` URL or `namespace/service[:port]`) |

### Available Prometheus Components

| Component | Description |
|-----------|-------------|
| `kubelet` | Node metrics (CPU, memory, pod counts) |
| `cadvisor` | Container metrics (CPU, memory, network I/O, disk I/O) |

Additional control plane components (apiserver, etcd, scheduler, controller-manager, kube-proxy) will be added in future releases.

## Display Options Flags

| Flag | Description |
|------|-------------|
| `--node-columns` | Comma-separated node columns to show |
| `--pod-columns` | Comma-separated pod columns to show |
| `--show-all-columns` | Show all columns (default: true) |
| `--sort-nodes` | Initial sort column and order for nodes (e.g. `+Name`, `-Memory`) |
| `--sort-pods` | Initial sort column and order for pods (e.g. `-Memory`, `+CPU`) |
| `--theme` | UI color theme: `auto` (default), `dark`, `light`, `ansi`, or `monochrome` |
| `--color-mode` | Terminal color depth override: `auto` (default), `truecolor`, `256`, `16`, or `none` |
| `--page` | Initial TUI page to display: `overview` (default), `workloads`, `events`, `storage`, or `network` |

## Non-Interactive (`--noui`) Flags

Use `--noui` when running `ktop` in CI pipelines, cron tasks, or shell scripts that consume structured JSON over `stdout`. See the [Non-Interactive Mode Guide](noui.md) for output schema details and `jq` recipes.

| Flag | Default | Description |
|------|---------|-------------|
| `--noui` | `false` | Disable interactive terminal UI and emit structured output to `stdout` |
| `-d, --duration` | `0s` | Collection window duration (for example `30s`, `5m`); `0s` captures a single snapshot |
| `-i, --interval` | `5s` | Sampling interval between snapshots during a timed collection window |
| `-o, --output` | `json` | Output format in `--noui` mode: `json` (single document) or `jsonl` (line-delimited stream) |
| `--quiet` | `false` | Suppress readiness and collection progress messages on `stderr` |

## Logging Flags

Diagnostic logs are written outside `stdout` so neither the terminal UI nor `--noui` JSON output is corrupted.

| Flag | Default | Description |
|------|---------|-------------|
| `--log` | `file` | Log destination: `file` (`~/.ktop/ktop.log`) or `stderr` |
| `--log-level` | `info` | Log verbosity: `debug`, `info`, `warn`, or `error` |
| `--log-format` | `text` | Log record format: `text` or `json` |

## Operational Exit Codes

`ktop` returns distinct non-zero exit codes to distinguish configuration errors from cluster connectivity, metrics availability, and readiness timeouts:

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | General application or runtime error |
| `2` | Invalid CLI flags, configuration values, or unexpected arguments |
| `3` | Kubernetes API connection or RBAC authorization failure |
| `4` | Explicitly requested metrics source unavailable |
| `5` | Readiness warmup or collection request timeout |

## Themes and Terminal Color Adaptation

Terminal emulators vary in background luminance and color-depth support, from 24-bit TrueColor local consoles to 16-color multiplexers or `$NO_COLOR` sessions. `ktop` adapts both its semantic color palette and color encoding tier to keep status indicators, sparklines, and selection highlights legible without per-host configuration.

Use `--theme` to pick a palette or leave it set to `auto` for automatic detection:

| Theme | Intended Terminal |
|-------|-------------------|
| `auto` | Resolves `$NO_COLOR` / `TERM=dumb` to `monochrome`, checks `$COLORFGBG`, probes OSC 11 background luminance on interactive TTYs, and defaults to `dark` |
| `dark` | High-contrast palette for dark terminal backgrounds |
| `light` | Deep-foreground palette tuned for light terminal backgrounds |
| `ansi` | Uses the terminal profile's 16 ANSI colors and default background |
| `monochrome` | Emits no color sequences; uses bold, underline, reverse video, and `!` error prefixes |

Use `--color-mode` (`truecolor`, `256`, `16`, `none`, or `auto`) when connecting through SSH, `tmux`, or terminfo profiles where automatic detection under- or over-reports color capability.

Persist preferred defaults in `~/.ktop/config.yaml`:

```yaml
ui:
  theme: auto
  color_mode: auto
  default_page: overview
```

Command-line `--theme`, `--color-mode`, and `--page` flags override saved values for the current session only.

## Advanced Connection Flags

| Flag | Description |
|------|-------------|
| `--as` | Username to impersonate |
| `--as-group` | Group to impersonate (can be repeated) |
| `--as-uid` | UID to impersonate |
| `--cache-dir` | Default cache directory |
| `--certificate-authority` | Path to CA cert file |
| `--client-certificate` | Path to client certificate |
| `--client-key` | Path to client key file |
| `--insecure-skip-tls-verify` | Skip server certificate verification |
| `--tls-server-name` | Server name for certificate validation |
| `--request-timeout` | Request timeout (e.g., 1s, 2m, 3h) |

## Examples

### Basic Usage

```bash
# Start with default settings
ktop

# Use a specific context
ktop --context production-cluster

# Filter to a namespace
ktop --namespace kube-system

# Combine options
ktop --context staging --namespace default

# Initial table sorting
ktop --sort-pods=-Memory --sort-nodes=+Name
```

### Metrics Source Selection

```bash
# Use Prometheus for enhanced metrics
ktop --metrics-source=prometheus

# Use Metrics Server
ktop --metrics-source=metrics-server

# No metrics (show requests/limits only)
ktop --metrics-source=none
```

### Prometheus Configuration

```bash
# Extended retention
ktop --metrics-source=prometheus --prometheus-retention=6h

# Faster scraping
ktop --metrics-source=prometheus --prometheus-scrape-interval=10s

# Low-memory profile for large or high-churn clusters
ktop --metrics-source=prometheus \
     --prometheus-scrape-interval=15s \
     --prometheus-retention=10m \
     --prometheus-max-samples=300

# Lowest memory footprint (CPU/memory only via Metrics Server)
ktop --metrics-source=metrics-server
```

### Authentication

```bash
# Use bearer token
ktop --server=https://k8s.example.com --token=$TOKEN

# Impersonate user
ktop --as=developer --as-group=developers

# Skip TLS verification (not recommended)
ktop --insecure-skip-tls-verify
```
