# KWOK Testing Setup for ktop

Simple setup for testing ktop with simulated Kubernetes clusters using KWOK.

## Quick Start

```bash
# Create test cluster (10 nodes, 200 pods)
./hack/kwok/setup-cluster.sh

# Test ktop
./ktop

# Cleanup when done
./hack/kwok/cleanup-cluster.sh
```

## Configuration

Customize the cluster via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `KWOK_CLUSTER_NAME` | `ktop-test` | Cluster name |
| `KWOK_NUM_NODES` | `10` | Number of nodes |
| `KWOK_NODE_CPU` | `8` | CPU cores per node |
| `KWOK_NODE_MEMORY` | `16Gi` | Memory per node |
| `KWOK_PODS_PER_NODE` | `20` | Pods per node |
| `KWOK_POD_NAMESPACE` | `test-workload` | Namespace for pods |
| `KWOK_ENABLE_METRICS` | `false` | Enable metrics-server simulation |

### Examples

**Small cluster for quick testing:**
```bash
KWOK_NUM_NODES=5 ./hack/kwok/setup-cluster.sh
# Creates: 5 nodes, 100 pods
```

**Large cluster for performance testing:**
```bash
KWOK_NUM_NODES=50 KWOK_PODS_PER_NODE=30 ./hack/kwok/setup-cluster.sh
# Creates: 50 nodes, 1500 pods
```

**High-resource nodes:**
```bash
KWOK_NUM_NODES=10 \
KWOK_NODE_CPU=32 \
KWOK_NODE_MEMORY=256Gi \
./hack/kwok/setup-cluster.sh
# Creates: 10 nodes (32 CPU, 256Gi each), 200 pods
```

**With metrics-server (simulated metrics):**
```bash
KWOK_ENABLE_METRICS=true ./hack/kwok/setup-cluster.sh
# Creates: 10 nodes, 200 pods with simulated CPU/memory usage
# Wait ~45 seconds after setup for metrics to be available
```

## What Gets Created

- **Isolated cluster**: Uses kwokctl to create a separate test cluster
- **Nodes**: Configured CPU/memory, all in Ready state
- **Pods**: Random resource requests (CPU: 100-500m, Memory: 128-512Mi)
- **Automatic context switch**: Setup script switches to the KWOK cluster

## Testing ktop

KWOK supports two testing modes:

### Mode 1: Without Metrics (Default)
KWOK simulates nodes and pods but **does not provide real metrics**:
- `kubectl top nodes` won't work
- `kubectl top pods` won't work
- ktop will run in **fallback mode** (shows resource requests/limits)

Perfect for testing:
- ✅ Fallback behavior when metrics are unavailable
- ✅ Startup performance with many nodes/pods
- ✅ Memory formatting (Mi vs Gi display)
- ✅ Column filtering (`--node-columns`, `--pod-columns`)
- ✅ UI responsiveness with large clusters

### Mode 2: With Metrics (`KWOK_ENABLE_METRICS=true`)
KWOK runs with metrics-server and simulates CPU/memory usage:
- `kubectl top nodes` will work
- `kubectl top pods` will work
- ktop will display **real metrics** (simulated usage values)
- Each pod gets random usage: 50-90% of resource requests

Perfect for testing:
- ✅ Metrics display and formatting
- ✅ Percentage calculations
- ✅ Metrics-server integration
- ✅ Real-world metric scenarios

### Verify the cluster:

```bash
kubectl get nodes
kubectl get pods -n test-workload
```

### Test ktop:

```bash
# Default mode (will use fallback - no metrics available)
./ktop

# Test column filtering
./ktop --node-columns=NAME,CPU,MEM
./ktop --pod-columns=NAMESPACE,POD,STATUS
```

You should see:
- All nodes displayed with "% requested" (not "% used")
- All pods displayed with resource requests
- No errors or crashes
- Fast startup time

## Context Management

The setup script automatically switches to the KWOK cluster context: `kwok-ktop-test`

**Check current context:**
```bash
kubectl config current-context
```

**Switch back to your regular cluster:**
```bash
kubectl config use-context minikube  # or your cluster name
```

**List all contexts:**
```bash
kubectl config get-contexts
```

## Cleanup

**Using the script:**
```bash
./hack/kwok/cleanup-cluster.sh
```

**Manual cleanup:**
```bash
kwokctl delete cluster --name ktop-test
```

**List all KWOK clusters:**
```bash
kwokctl get clusters
```

## Troubleshooting

### Cluster already exists

```bash
./hack/kwok/cleanup-cluster.sh
./hack/kwok/setup-cluster.sh
```

### ktop shows no pods

Check you're using the correct context:
```bash
kubectl config current-context
kubectl get pods --all-namespaces
```

### Port conflicts

Use a different cluster name:
```bash
KWOK_CLUSTER_NAME="ktop-test-2" ./hack/kwok/setup-cluster.sh
```

## Files in This Directory

| File | Description |
|------|-------------|
| `setup-cluster.sh` | Main setup script - creates cluster, nodes, and pods |
| `cleanup-cluster.sh` | Cleanup script - deletes the cluster |
| `README.md` | This documentation |
| `kwok.sh` | Legacy script (reference only) |
| `kwok-nodes.yaml` | Legacy static nodes (reference only) |
| `kwok-deployments.yaml` | Legacy deployments (reference only) |

The setup script dynamically generates all resources - no manual YAML editing needed.

## Resources

- [KWOK Documentation](https://kwok.sigs.k8s.io/)
- [KWOK GitHub](https://github.com/kubernetes-sigs/kwok)
- [kwokctl Reference](https://kwok.sigs.k8s.io/docs/user/kwokctl/)
