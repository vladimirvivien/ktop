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
| `--prometheus-scrape-interval` | `15s` | How often to scrape (min: 5s) |
| `--prometheus-retention` | `1h` | How long to keep metrics (min: 5m) |
| `--prometheus-max-samples` | `10000` | Max samples per time series |
| `--prometheus-components` | `kubelet,cadvisor` | Components to scrape |

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

# Scrape only kubelet (lower memory usage)
ktop --metrics-source=prometheus --prometheus-components=kubelet

# Minimal memory footprint
ktop --metrics-source=prometheus \
     --prometheus-components=kubelet \
     --prometheus-retention=30m \
     --prometheus-max-samples=5000
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
