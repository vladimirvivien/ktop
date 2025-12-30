# ktop User Guide

## What is ktop?

ktop is a terminal-based monitoring tool for Kubernetes clusters, similar to the Unix `top` command. It provides a real-time view of your cluster's nodes, pods, and containers with resource usage metrics.

Unlike `kubectl top`, which shows a static snapshot, ktop continuously updates and allows you to drill down from cluster overview to individual container logs—all from a single terminal interface.

## Prerequisites

- **Kubernetes cluster access** - A running cluster you can connect to
- **kubeconfig file** - Valid credentials (typically at `~/.kube/config`)
- **Optional: Metrics infrastructure** - Metrics-Server instance running on the cluster or accessible kubelet/cAdvisor endpoints for resource usage data

## Installation

### kubectl plugin

```bash
# Install krew if not present: https://krew.sigs.k8s.io/docs/user-guide/setup/install/
kubectl krew install ktop
kubectl ktop
```

### Homebrew (macOS/Linux)

```bash
brew tap vladimirvivien/oss-tools
brew install ktop
```

### Docker

```bash
export KUBECONFIG=$HOME/.kube/config
docker run --network=host --rm -it \
  -v $KUBECONFIG:/config \
  -e KUBECONFIG=/config \
  -e TERM=xterm-256color \
  ghcr.io/vladimirvivien/ktop:latest
```

### Go install

```bash
go install github.com/vladimirvivien/ktop@latest
```

### Binary download

Download from [GitHub Releases](https://github.com/vladimirvivien/ktop/releases/latest) and add to your PATH.

### Build from source

```bash
git clone https://github.com/vladimirvivien/ktop.git
cd ktop
go build .
```

The project also includes a cross-platform build script:
```bash
go run ./ci/build.go
```

## Running ktop

Basic usage:
```bash
ktop
```

ktop uses your kubeconfig file (from `$KUBECONFIG` or `~/.kube/config`) to connect. Override settings with flags:

```bash
# Use specific context
ktop --context my-cluster

# Filter to a namespace
ktop --namespace production

# Combine options
ktop --context staging --namespace default --metrics-source=prometheus
```

## Metrics Sources

ktop supports three metrics sources. It automatically falls back through them if the preferred source is unavailable.

| Source | Description |
|--------|-------------|
| `prometheus` | Scrapes metrics directly from kubelet/cAdvisor |
| `metrics-server` | Uses Kubernetes Metrics Server API |
| `none` | No metrics collection, requests/limits only |

### Automatic Fallback

When you run `ktop` without specifying a source, it tries:
1. **prometheus** - If RBAC permits access to node metrics endpoints
2. **metrics-server** - If Metrics Server is available on the cluster
3. **none** - Shows resource requests/limits instead of usage

### Prometheus Mode

Prometheus mode scrapes metrics directly from Kubernetes components, providing richer data including network and disk I/O.

```bash
ktop --metrics-source=prometheus
```

**Requirements:**
- RBAC permissions to access `/metrics` endpoints on nodes
- Not available in most managed Kubernetes services (control plane is inaccessible)

Apply RBAC permissions:
```bash
kubectl apply -f https://raw.githubusercontent.com/vladimirvivien/ktop/main/hack/deploy/rbac-prometheus.yaml
```

### Metrics Server Mode

Works with the standard Kubernetes Metrics Server:

```bash
ktop --metrics-source=metrics-server
```

This is the most compatible option for managed Kubernetes services.

## Navigation

ktop uses a hierarchical navigation model:

```
Overview → Node Detail → (back to Overview)
         → Pod Detail → Container Detail → (back through each level)
```

### Key Controls

| Key | Action |
|-----|--------|
| **Enter** | Drill down into selected node, pod, or container |
| **ESC** | Go back to previous page (or exit filter mode if active) |
| **Tab** | Cycle focus between panels |
| **Ctrl+C** | Quit immediately |

### Tips

- The **footer** shows available shortcuts for the current context
- Press **ESC twice** from the Overview page to quit (first ESC shows confirmation)
- When a table column header has a highlighted letter, press that letter to sort by that column
- Press `/` when the header is focused to filter pods by namespace

## Pages

### Overview

The main dashboard showing cluster health at a glance. Displays summary statistics, nodes with their resource usage, and pods with status and metrics.

**Navigation:** Select a node or pod and press Enter to see details. Press Tab to move between panels.

### Node Detail

Shows everything about a single node: system information, conditions (Ready, MemoryPressure, etc.), recent events, and all pods running on that node.

**Navigation:** Select a pod and press Enter. Press ESC to return to Overview.

### Pod Detail

Displays pod conditions, events, and a list of containers. Shows per-container CPU and memory usage.

**Navigation:** Select a container and press Enter for logs. Press `n` to jump to the node this pod runs on. Press ESC to go back.

### Container Detail

Shows container specification (image, ports, probes, resource limits) and live streaming logs.

**Log controls:**
- `s` - Toggle streaming on/off
- `t` - Toggle timestamps
- `w` - Toggle line wrap
- `m` - Load 100 more older lines
- `x` - Expand logs to full screen
- `/` - Filter logs (grep-style)
- `g/G` - Jump to top/bottom

Press ESC to return to Pod Detail. If filtering is active, first ESC exits filter mode.

## Troubleshooting

### "prometheus source failed" / Falls back to metrics-server

Your user doesn't have RBAC permissions to access node metrics endpoints. Either:
- Apply the RBAC manifest: `kubectl apply -f https://raw.githubusercontent.com/vladimirvivien/ktop/main/hack/deploy/rbac-prometheus.yaml`
- Use metrics-server mode: `ktop --metrics-source=metrics-server`

### "Terminal too small"

ktop requires at least 31 rows. Resize your terminal or use a larger font.

### Metrics show as "-" or "n/a"

- If using `--metrics-source=none`, this is expected (shows requests/limits only)
- If using prometheus/metrics-server, check that your metrics infrastructure is running:
  ```bash
  kubectl top nodes  # Should show CPU/memory
  ```

### Permission errors

Ensure your kubeconfig user has access to nodes, pods, events, and metrics resources. Common minimum permissions:
- `get`, `list`, `watch` on `nodes`, `pods`, `events`
- `get` on `nodes/proxy` (for prometheus mode)
