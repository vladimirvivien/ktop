# Metrics Server Integration

This document describes how ktop collects metrics from the Kubernetes Metrics Server.

## Overview

The [Kubernetes Metrics Server](https://github.com/kubernetes-sigs/metrics-server) is a cluster-wide aggregator of resource usage data. It collects metrics from kubelets and exposes them through the Kubernetes API. ktop queries this API to display CPU and memory usage for nodes and pods.

This is the most compatible metrics source, working with virtually all Kubernetes clusters including managed services like GKE, EKS, and AKS.


## Quick Start

```bash
# Explicit metrics server mode
ktop --metrics-source=metrics-server

# ktop auto-detects available sources
# Falls back to metrics-server if prometheus isn't available
ktop
```

## Architecture

```
┌───────────────────────────────────────────────────┐
│              Kubernetes Cluster                    │
│  ┌──────────────────┐  ┌──────────────────┐       │
│  │     kubelet      │  │     kubelet      │       │
│  │   (per node)     │  │   (per node)     │       │
│  └────────┬─────────┘  └────────┬─────────┘       │
│           └──────────┬──────────┘                 │
│                      │                            │
│           ┌──────────▼──────────┐                 │
│           │   Metrics Server    │                 │
│           │  (cluster-wide)     │                 │
│           └──────────┬──────────┘                 │
└──────────────────────┼────────────────────────────┘
                       │
              ┌────────▼────────┐
              │ Kubernetes API  │  metrics.k8s.io/v1beta1
              │ /apis/metrics/  │
              └────────┬────────┘
                       │
              ┌────────▼────────┐
              │MetricsServerSrc │  metrics/k8s/metrics_server_source.go
              │ (ktop adapter)  │  - API queries
              └────────┬────────┘  - local history buffers
                       │
              ┌────────▼────────┐
              │   ktop Views    │
              │  (UI display)   │
              └─────────────────┘
```

### How It Works

ktop queries the Metrics Server through the standard Kubernetes API:

- **Node metrics**: `/apis/metrics.k8s.io/v1beta1/nodes`
- **Pod metrics**: `/apis/metrics.k8s.io/v1beta1/pods`

These endpoints are available wherever you can run `kubectl top`.

## Metrics Collected

### Node Metrics

| Metric | API Field | Description |
|--------|-----------|-------------|
| CPU usage | `usage.cpu` | Current CPU consumption |
| Memory usage | `usage.memory` | Current memory consumption |

### Pod/Container Metrics

| Metric | API Field | Description |
|--------|-----------|-------------|
| CPU usage | `containers[].usage.cpu` | Per-container CPU consumption |
| Memory usage | `containers[].usage.memory` | Per-container memory consumption |

## Historical Data

ktop maintains local ring buffers to store metrics history when using Metrics Server mode. This enables sparklines and trend visualization even though Metrics Server itself only provides point-in-time data.

- Default buffer size: 120 samples (~10 minutes at default refresh rate)
- History is lost when ktop exits
- Only CPU and memory history is tracked

## RBAC Requirements

Metrics Server mode requires standard read access to the metrics API:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: ktop-metrics-server
rules:
- apiGroups: ["metrics.k8s.io"]
  resources: ["nodes", "pods"]
  verbs: ["get", "list"]
```

Most Kubernetes users already have these permissions. Check with:
```bash
kubectl auth can-i get pods.metrics.k8s.io
kubectl auth can-i list nodes.metrics.k8s.io
```

## Troubleshooting

### "metrics client not available"

**Cause**: Metrics Server is not installed or not accessible.

**Solutions**:
1. Check if Metrics Server is running:
   ```bash
   kubectl get deployment metrics-server -n kube-system
   ```
2. Verify metrics API is available:
   ```bash
   kubectl top nodes
   kubectl top pods
   ```
3. Install Metrics Server if missing:
   ```bash
   kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
   ```

### Metrics show as "-" or "n/a"

**Cause**: Metrics Server hasn't collected data yet, or pod is too new.

**Solutions**:
1. Wait 30-60 seconds after pod startup
2. Verify the pod is running: `kubectl get pod <name>`
3. Check Metrics Server logs: `kubectl logs -n kube-system deployment/metrics-server`

### "Metrics Server unavailable" errors

**Cause**: Network or permission issues reaching Metrics Server.

**Solutions**:
1. Check cluster connectivity: `kubectl cluster-info`
2. Verify RBAC permissions (see above)
3. Check if Metrics Server pod is healthy:
   ```bash
   kubectl get pods -n kube-system -l k8s-app=metrics-server
   ```

## Limitations

1. **Basic metrics only**: CPU and memory usage. No network, disk, or load metrics.

2. **No native history**: Metrics Server provides point-in-time snapshots only. ktop maintains local buffers for sparklines, but history is lost on restart.

3. **Aggregated values**: Container metrics are pre-aggregated. Less granular than Prometheus.

4. **Sampling delay**: Metrics Server typically updates every 15 seconds. Real-time precision is limited.

## Fallback Behavior

When `--metrics-source` is not specified:
1. ktop tries Prometheus mode first
2. If RBAC denies access, falls back to Metrics Server
3. If Metrics Server is unavailable, shows resource requests/limits only

When `--metrics-source=metrics-server` is explicitly set:
- Uses Metrics Server exclusively
- Falls back to requests/limits if Metrics Server is unavailable
