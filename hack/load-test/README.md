# Load Test for ktop

Deploy real pods with actual CPU and memory workloads for testing ktop's metrics visualization.

Unlike KWOK simulated clusters, these pods generate **real resource consumption** using [stress-ng](https://github.com/ColinIanKing/stress-ng), providing accurate metrics for testing bargraphs, color transitions, and overall UI responsiveness.

## Quick Start

```bash
# Deploy 20 pods with steady workload
./hack/load-test/deploy-load.sh

# Watch in ktop
ktop -n load-test

# Cleanup when done
./hack/load-test/cleanup-load.sh
```

## Configuration

All settings are controlled via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `LOAD_NAMESPACE` | `load-test` | Kubernetes namespace for test pods |
| `LOAD_POD_COUNT` | `20` | Number of pods to create |
| `LOAD_CPU_MILLICORES` | `100` | Base CPU request per pod (millicores) |
| `LOAD_MEMORY_MB` | `64` | Base memory per pod (MiB) |
| `LOAD_PATTERN` | `steady` | Workload pattern (see below) |
| `LOAD_STRESS_IMAGE` | `alexeiled/stress-ng:latest` | Container image to use |

## Workload Patterns

### `steady` (default)
Constant CPU and memory usage. Good for baseline testing.

```bash
./hack/load-test/deploy-load.sh
```

### `spiky`
Variable load levels across pods (30-90% CPU utilization). Tests color transitions in ktop as load varies.

```bash
LOAD_PATTERN=spiky ./hack/load-test/deploy-load.sh
```

### `ramping`
Pods start at different load levels (20-100%), simulating gradual cluster load increase. Good for testing bargraph progression.

```bash
LOAD_PATTERN=ramping ./hack/load-test/deploy-load.sh
```

### `mixed`
Each pod gets a different pattern (rotating through steady, spiky, ramping). Most realistic simulation.

```bash
LOAD_PATTERN=mixed ./hack/load-test/deploy-load.sh
```

## Example Scenarios

### Small test (low resource usage)
```bash
LOAD_POD_COUNT=5 LOAD_CPU_MILLICORES=50 LOAD_MEMORY_MB=32 ./hack/load-test/deploy-load.sh
```

### Large scale test
```bash
LOAD_POD_COUNT=100 ./hack/load-test/deploy-load.sh
```

### High memory workload
```bash
LOAD_POD_COUNT=10 LOAD_MEMORY_MB=256 ./hack/load-test/deploy-load.sh
```

### CPU-intensive workload
```bash
LOAD_POD_COUNT=10 LOAD_CPU_MILLICORES=500 ./hack/load-test/deploy-load.sh
```

### Test color transitions
```bash
# Deploy pods at various load levels to see olivedrab → yellow → red transitions
LOAD_PATTERN=ramping LOAD_POD_COUNT=30 ./hack/load-test/deploy-load.sh
```

## What to Observe in ktop

After deploying load-test pods, run `ktop -n load-test` and observe:

1. **Cluster Summary**: Total CPU/memory usage increases
2. **Node Panel**: Per-node resource consumption with color-coded percentages
3. **Pod Panel**: Individual pod metrics with:
   - Braille bargraphs showing fill levels
   - Color transitions (olivedrab → yellow → red) based on usage
   - Real CPU/memory values updating every refresh cycle

### Expected Bargraph Behavior

| Usage | Color | Bargraph |
|-------|-------|----------|
| 0-70% | olivedrab | `[⣿⣿⣿⣿⣿⠀⠀⠀⠀⠀]` |
| 70-90% | yellow | `[⣿⣿⣿⣿⣿⣿⣿⠀⠀⠀]` |
| 90-100% | red | `[⣿⣿⣿⣿⣿⣿⣿⣿⣿⠀]` |

## Resource Requirements

Default deployment (20 pods):
- **CPU**: ~2000m (2 cores) requested
- **Memory**: ~1280Mi (~1.3Gi) requested

Ensure your cluster has sufficient resources. For minikube:
```bash
minikube start --cpus=4 --memory=8192
```

## Troubleshooting

### Pods stuck in Pending
Check node resources:
```bash
kubectl describe nodes | grep -A5 "Allocated resources"
kubectl top nodes
```

### Pods in CrashLoopBackOff
The stress-ng image may not be available. Try alternative image:
```bash
LOAD_STRESS_IMAGE=polinux/stress ./hack/load-test/deploy-load.sh
```

### No metrics showing
Ensure metrics-server is installed:
```bash
kubectl top pods -n load-test
# If this fails, install metrics-server:
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
```

### Image pull errors
Pre-pull the image:
```bash
docker pull alexeiled/stress-ng:latest
# Or for minikube:
minikube ssh -- docker pull alexeiled/stress-ng:latest
```

## Cleanup

Remove all test resources:
```bash
./hack/load-test/cleanup-load.sh
```

Or manually:
```bash
kubectl delete namespace load-test
```

## Comparison: Load Test vs KWOK

| Feature | Load Test | KWOK |
|---------|-----------|------|
| Real metrics | ✅ Yes | ❌ Simulated |
| Resource consumption | ✅ Actual | ❌ Fake |
| Scale | Limited by cluster | ✅ Thousands of pods |
| Setup speed | Slower (pulls images) | ✅ Fast |
| Use case | Metrics accuracy testing | Performance/scale testing |

**Recommendation**: Use load-test for metrics visualization testing, KWOK for large-scale UI performance testing.
