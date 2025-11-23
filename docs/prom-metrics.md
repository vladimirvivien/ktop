# Using Prometheus Metrics with ktop

This guide walks you through using ktop to collect and display metrics directly from Kubernetes component endpoints (kubelet, cAdvisor, API server, etc.) instead of relying on the Metrics Server.

## Overview

ktop supports two metrics sources:

1. **Metrics Server** (default) - Standard Kubernetes metrics from metrics-server API
2. **Prometheus** - Enhanced metrics scraped directly from Kubernetes components

The Prometheus source provides richer metrics including:
- Network I/O statistics (Rx/Tx bytes)
- System load averages (1m, 5m, 15m)
- Container counts per node
- Per-container resource breakdowns
- And 100+ additional Kubernetes metrics

## Quick Start

### Basic Prometheus Mode

```bash
ktop --metrics-source=prometheus
```

This uses default settings:
- **Components**: kubelet, cAdvisor
- **Scrape interval**: 15 seconds
- **Retention**: 1 hour
- **Max samples**: 10,000 per time series

### Custom Configuration

```bash
ktop --metrics-source=prometheus \
     --prometheus-scrape-interval=30s \
     --prometheus-retention=2h \
     --prometheus-components=kubelet,cadvisor,apiserver
```

## Prerequisites

### 1. RBAC Permissions

The Prometheus mode accesses component metrics via the **Kubernetes API proxy**, which requires specific RBAC permissions.

**Required permissions:**
- `get`, `list` on `nodes` - For node discovery
- `get` on `nodes/proxy` - For kubelet and cAdvisor metrics (e.g., `/api/v1/nodes/{node}/proxy/metrics`)
- `get`, `list` on `pods` - For component pod discovery
- `get` on `pods/proxy` - For component metrics (e.g., etcd, scheduler, controller-manager)
- `get` on non-resource URL `/metrics` - For API server metrics

**For minikube/kind/k3s clusters:** You typically have admin access by default, so no additional RBAC setup is needed.

**For production clusters:** Apply the provided RBAC manifest:

```bash
# Review the permissions
cat hack/deploy/rbac-prometheus.yaml

# Apply the ClusterRole
kubectl apply -f hack/deploy/rbac-prometheus.yaml

# Update the ClusterRoleBinding with your username
kubectl edit clusterrolebinding ktop-prometheus-reader-binding
# Change: subjects[0].name from "your-username" to your actual username
```

**To check your current permissions:**

```bash
kubectl auth can-i get nodes/proxy
kubectl auth can-i get pods/proxy
kubectl auth can-i get --raw /metrics
```

**For managed clusters (GKE, EKS, AKS):** Even with RBAC permissions, component endpoints may be restricted by the cloud provider. Prometheus mode may not work in these environments.

### 2. Network Access

ktop accesses component metrics **through the Kubernetes API proxy**, not by directly connecting to node IPs. This means:

- ✅ You don't need direct network access to node IPs
- ✅ You don't need VPN or bastion hosts
- ✅ Works from anywhere you can run `kubectl`

The API server proxies requests to:
- **Kubelet/cAdvisor**: `/api/v1/nodes/{node}/proxy/metrics` → `https://node-ip:10250/metrics`
- **API Server**: `/metrics` → Direct API server endpoint
- **etcd/scheduler/etc**: `/api/v1/namespaces/kube-system/pods/{pod}:{port}/proxy/metrics`

**Requirements:**
- Your kubeconfig must have access to the Kubernetes API server (same as `kubectl`)
- The API server must be able to reach the component endpoints (true for standard clusters)

**Note:** Some managed Kubernetes providers (GKE, EKS, AKS) may restrict or disable the API proxy for security reasons.

## Testing on Local Clusters

### Minikube

```bash
# 1. Ensure minikube is running
minikube status

# 2. Verify kubectl access
kubectl cluster-info

# 3. Check if metrics-server is installed (optional)
kubectl get deployment metrics-server -n kube-system

# 4. Run ktop with Prometheus
ktop --metrics-source=prometheus
```

**Expected behavior:**
- ktop will attempt to scrape metrics from kubelet and cAdvisor on the minikube node
- Initial connection may take 15-30 seconds as metrics are collected
- Header should show "metrics: connected" (green) when successful
- If RBAC/network issues occur, header shows "metrics: not connected" (red)

### Kind (Kubernetes in Docker)

```bash
# 1. Ensure kind cluster is running
kind get clusters

# 2. Verify kubectl context
kubectl config current-context

# 3. Run ktop with Prometheus
ktop --metrics-source=prometheus
```

**Known limitation:** Kind clusters may have restricted access to node endpoints depending on your Docker network setup.

### k3s

```bash
# 1. Verify k3s is running
kubectl get nodes

# 2. Run ktop with Prometheus
ktop --metrics-source=prometheus \
     --prometheus-components=kubelet,cadvisor
```

**Note:** k3s may not expose all component metrics endpoints. Start with just kubelet and cadvisor.

## Available Components

You can configure which Kubernetes components to scrape metrics from:

| Component | Endpoint | Metrics Provided |
|-----------|----------|------------------|
| `kubelet` | Node:10250/metrics | Node CPU, memory, network, load averages, pod counts |
| `cadvisor` | Node:10250/metrics/cadvisor | Container-level CPU, memory, network, disk I/O |
| `apiserver` | API server:6443/metrics | Request latency, counts, authentication, authorization |
| `etcd` | Node:2379/metrics | Database size, latency, raft consensus |
| `scheduler` | Node:10259/metrics | Queue depth, scheduling latency |
| `controller-manager` | Node:10257/metrics | Reconciliation loops, work queue depth |
| `kube-proxy` | Node:10249/metrics | Network proxy rules, sync latency |

**Recommended for most users:**
```bash
ktop --metrics-source=prometheus \
     --prometheus-components=kubelet,cadvisor
```

**Full control plane monitoring (advanced):**
```bash
ktop --metrics-source=prometheus \
     --prometheus-components=kubelet,cadvisor,apiserver,etcd,scheduler,controller-manager
```

## Configuration Options

### Scrape Interval

How often to collect metrics from components:

```bash
# Fast refresh (higher resource usage)
ktop --metrics-source=prometheus --prometheus-scrape-interval=10s

# Balanced (default)
ktop --metrics-source=prometheus --prometheus-scrape-interval=15s

# Conservative (lower resource usage)
ktop --metrics-source=prometheus --prometheus-scrape-interval=60s
```

**Minimum:** 5 seconds

**Recommendation:** 15-30 seconds for most use cases

### Retention Time

How long to keep metrics in memory:

```bash
# Short retention (lower memory usage)
ktop --metrics-source=prometheus --prometheus-retention=30m

# Default
ktop --metrics-source=prometheus --prometheus-retention=1h

# Extended retention (for trend analysis)
ktop --metrics-source=prometheus --prometheus-retention=6h
```

**Minimum:** 5 minutes

**Memory impact:** Longer retention = more memory usage. Each time series sample is ~16 bytes.

### Max Samples

Limit the number of samples per time series:

```bash
# Conservative (lower memory)
ktop --metrics-source=prometheus --prometheus-max-samples=5000

# Default
ktop --metrics-source=prometheus --prometheus-max-samples=10000

# High capacity (more history)
ktop --metrics-source=prometheus --prometheus-max-samples=50000
```

**Memory impact:** 10,000 samples × 100 time series × 16 bytes ≈ 16 MB

## Troubleshooting

### "failed to start prometheus collection"

**Cause:** Insufficient RBAC permissions or network access issues

**Solutions:**
1. Check RBAC permissions (see Prerequisites section above)
2. Verify network access to node endpoints
3. Try with fewer components: `--prometheus-components=kubelet`
4. Fall back to metrics-server: `--metrics-source=metrics-server`

**Check permissions:**
```bash
kubectl auth can-i get nodes/proxy
kubectl auth can-i get nodes/metrics
```

### "prometheus source is not healthy"

**Cause:** Initial scrape failed or ongoing connection issues

**Solutions:**
1. Wait 15-30 seconds for initial scrape to complete
2. Check component endpoints are accessible:
   ```bash
   # Get node IP
   kubectl get nodes -o wide

   # Test kubelet endpoint (replace NODE_IP)
   kubectl proxy &
   curl http://localhost:8001/api/v1/nodes/<node-name>/proxy/metrics
   ```
3. Reduce scrape interval: `--prometheus-scrape-interval=30s`
4. Enable only kubelet: `--prometheus-components=kubelet`

### "metrics: not connected" in header

**Cause:** Prometheus scraping hasn't succeeded yet or has no healthy sources

**Check:**
1. Wait 15-30 seconds after startup for initial scrape
2. Look for error messages in the UI (if any)
3. Try running with debug logging (future feature)
4. Verify cluster type supports endpoint access (not GKE/EKS/AKS)

### Memory usage is too high

**Solutions:**
1. Reduce retention time: `--prometheus-retention=30m`
2. Reduce max samples: `--prometheus-max-samples=5000`
3. Scrape fewer components: `--prometheus-components=kubelet`
4. Increase scrape interval: `--prometheus-scrape-interval=60s`

**Check current memory:**
```bash
ps aux | grep ktop
```

### Metrics look incorrect or stale

**Check:**
1. Verify scrape interval is reasonable (15-30s recommended)
2. Ensure retention time hasn't been exceeded
3. Compare with kubectl: `kubectl top nodes` and `kubectl top pods`
4. Try metrics-server mode to compare: `ktop --metrics-source=metrics-server`

## Comparing Metrics Sources

### Metrics Server vs Prometheus

| Feature | Metrics Server | Prometheus |
|---------|----------------|------------|
| **CPU/Memory** | ✅ Basic | ✅ Detailed |
| **Network I/O** | ❌ | ✅ |
| **Load Averages** | ❌ | ✅ |
| **Container Counts** | ❌ | ✅ |
| **Historical Data** | ❌ (current only) | ✅ (configurable) |
| **Setup Required** | Metrics Server deployment | RBAC + network access |
| **Works in Managed K8s** | ✅ (GKE, EKS, AKS) | ❌ (restricted) |
| **Memory Usage** | Low (~50 MB) | Medium (~100-200 MB) |

### When to Use Prometheus Mode

**Use Prometheus when:**
- Running on local/self-managed clusters (minikube, kind, k3s, bare metal)
- Need network I/O statistics
- Need system load averages
- Need container-level breakdowns
- Want historical data for trend analysis

**Use Metrics Server when:**
- Running on managed Kubernetes (GKE, EKS, AKS)
- Basic CPU/memory metrics are sufficient
- Want lower memory footprint
- Metrics Server is already deployed

## Performance Tuning

### Low Resource Profile

For minimal memory and CPU usage:

```bash
ktop --metrics-source=prometheus \
     --prometheus-components=kubelet \
     --prometheus-scrape-interval=60s \
     --prometheus-retention=30m \
     --prometheus-max-samples=5000
```

**Expected usage:** ~80-100 MB memory, <1% CPU

### Balanced Profile (Default)

Good balance of metrics richness and resource usage:

```bash
ktop --metrics-source=prometheus \
     --prometheus-components=kubelet,cadvisor \
     --prometheus-scrape-interval=15s \
     --prometheus-retention=1h \
     --prometheus-max-samples=10000
```

**Expected usage:** ~120-150 MB memory, 1-2% CPU

### Rich Metrics Profile

Maximum metrics collection for deep observability:

```bash
ktop --metrics-source=prometheus \
     --prometheus-components=kubelet,cadvisor,apiserver,etcd,scheduler,controller-manager \
     --prometheus-scrape-interval=10s \
     --prometheus-retention=6h \
     --prometheus-max-samples=50000
```

**Expected usage:** ~300-500 MB memory, 3-5% CPU

## Examples

### Example 1: Basic Local Development

```bash
# Quick check of local minikube cluster
ktop --metrics-source=prometheus
```

### Example 2: Extended Monitoring Session

```bash
# Monitor cluster for several hours with full metrics
ktop --metrics-source=prometheus \
     --prometheus-retention=6h \
     --prometheus-components=kubelet,cadvisor,apiserver
```

### Example 3: Minimal Footprint

```bash
# Lightweight monitoring for resource-constrained environments
ktop --metrics-source=prometheus \
     --prometheus-components=kubelet \
     --prometheus-scrape-interval=60s \
     --prometheus-retention=30m
```

### Example 4: Comparing Sources

```bash
# Run metrics-server mode
ktop --metrics-source=metrics-server
# Note the CPU/Memory values

# Run prometheus mode
ktop --metrics-source=prometheus
# Compare with metrics-server values
```

## Future Enhancements

The following features are planned for future releases:

- **Enhanced columns** (`--enhanced-columns` flag) - Display network I/O, load averages, and container counts in the node view
- **Trend indicators** - Visual indicators showing if resource usage is increasing/decreasing
- **Source health indicator** - Show which metrics source is active in the UI
- **Auto-fallback** - Automatically switch between sources based on availability
- **Metrics export** - Export collected metrics to file for analysis

## See Also

- [Main README](../README.md) - General ktop usage
- [Architecture Documentation](prom.md) - Technical design details
- [kubectl top](https://kubernetes.io/docs/reference/kubectl/generated/kubectl_top/) - Compare with built-in metrics
