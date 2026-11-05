# PSI Metrics

Pressure Stall Information measures the percentage of wall time a cgroup spent with at least one task blocked waiting on a resource. ktop surfaces PSI in a `STALL` column on the Pods and Nodes tables and as an aggregate `Stall:` value on the cluster summary strip.

PSI distinguishes a healthy busy workload from a throttled one — a signal that pure utilization metrics (`kubectl top`-style CPU%) cannot show.

## What the cell shows

```
IO 1.15%    I/O bound (e.g. etcd writing to disk)
CPU 0.24%   CPU bound (scheduler hot path, throttled cgroup)
MEM 8.0%    memory bound (page-cache pressure, swapping)
·           genuinely zero
```

The label names the dominant axis — whichever of CPU / MEM / IO has the highest stall percentage at this moment. Colors:

| Range | Color |
|-------|-------|
| < 2% | green |
| 2 – 10% | yellow |
| 10 – 25% | orange |
| ≥ 25% | red |

## Why PSI, not just CPU%

| Pod state | `kubectl top` CPU | STALL | Interpretation |
|-----------|------------------|-------|----------------|
| Healthy busy | 80% | 0% | Using its allocation, getting work done |
| **Throttled** | 30% | CPU 15% | **Wants CPU, isn't getting it** — cgroup limit or noisy neighbor |
| Memory-pressured | 40% | MEM 8% | Waiting on page faults / reclaim |
| Disk-bound | 20% | IO 12% | Storage backlog |
| Idle | 5% | · | Healthy |

The Healthy-busy and Throttled rows are indistinguishable by CPU% alone. STALL separates them.

## Aggregation

- **Per pod** — MAX across the pod's containers per axis. A pod with one stalled container surfaces that container's number; idle siblings do not dilute it.
- **Per node** — the node's root cgroup (`id="/"` in cAdvisor labels). Whole-node stall.
- **Cluster summary `Stall:`** — mean across nodes per axis, then dominant-axis selection. Average is the right cluster-wide health number; the per-node row still shows the individual worst-affected nodes.

## Sorting

Press `l` while focused on the Pods or Nodes table to sort by the dominant stall axis. Press again to flip direction.

## Requirements

| Component | Required | Notes |
|-----------|----------|-------|
| Linux kernel | ≥ 4.20 | PSI was added in 4.20 |
| cgroup version | v2 | Per-cgroup PSI requires cgroup v2 |
| Kubernetes server | ≥ 1.34 (Beta), 1.36 (GA) | `KubeletPSI` feature gate enabled by default in 1.34; locked on in 1.36 |
| Metrics source | Prometheus | Metrics-server does not surface PSI; STALL cells will read `·` everywhere |

When the kernel on a node is older than 4.20, the per-row STALL cell is prefixed with a yellow `⚠` glyph because the underlying counters either don't exist or report best-effort values.

## Caveat: K8s versions below 1.36

PSI is GA in Kubernetes 1.36+ ([release blog](https://kubernetes.io/blog/2026/05/12/kubernetes-v1-36-psi-metrics-ga/)). Values are still shown on 1.34 and 1.35 clusters, but a [known kubelet bug](https://github.com/kubernetes/kubernetes/issues/136333) causes some pre-GA builds to emit zero-valued PSI metrics even when the host OS does not support PSI. Real zero and fake zero are indistinguishable at the metric layer. Use the procedure below to verify when a node's STALL column persistently reads `·` despite a busy workload.

## Verifying PSI is live on a node

```bash
# Confirm the kernel exposes PSI
kubectl debug node/<node-name> -it --image=busybox -- cat /proc/pressure/cpu

# A working node returns three lines like:
#   some avg10=0.00 avg60=0.04 avg300=0.07 total=12345678
#   ...
# If /proc/pressure/cpu does not exist, the kernel lacks PSI support.

# Confirm kubelet is exporting PSI metrics
kubectl get --raw "/api/v1/nodes/<node-name>/proxy/metrics/cadvisor" \
  | grep -E '^container_pressure_(cpu|memory|io)_(waiting|stalled)_seconds_total' \
  | head
```

If `/proc/pressure/cpu` exists but the metrics endpoint emits zeros only, your kubelet build is affected by the zero-emission bug; upgrade to 1.36 or a patched 1.34/1.35.

## First-run behavior

STALL cells populate after a **40-second warmup**. The stall rate is computed over a 40-second window; before the window fills, cells read `·` regardless of actual stall.

## Reference

- [Understand PSI Metrics — Kubernetes docs](https://kubernetes.io/docs/reference/instrumentation/understand-psi-metrics/)
- [Kubernetes 1.36 PSI GA announcement](https://kubernetes.io/blog/2026/05/12/kubernetes-v1-36-psi-metrics-ga/)
- [KEP-4205: PSI Metric](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/4205-psi-metric/README.md)
- [Linux kernel PSI documentation](https://docs.kernel.org/accounting/psi.html)
