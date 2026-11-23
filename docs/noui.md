# Non-Interactive Mode (`--noui`)

Automation scripts, CI/CD checks, and cron jobs need structured telemetry from a cluster without allocating an interactive terminal UI. Passing `--noui` switches `ktop` from the terminal UI into a synchronous snapshot collector that emits machine-readable JSON or JSONL on `stdout`, isolates diagnostic logs from data output, and exits with predictable operational status codes.

## Quick Start

In its default single-shot form, `--noui` waits for informer caches to sync and initial metrics to warm up, writes one complete cluster report as formatted JSON to `stdout`, and exits:

```bash
ktop --noui | jq .
```

To observe cluster behavior over time, add `-d` (`--duration`) and `-i` (`--interval`). Multi-sample runs retain every interval snapshot in `samples` alongside aggregate statistics:

```bash
# Sample every 5s over a 30s window and write a single JSON report
ktop --noui -d 30s -i 5s -o json > cluster-report.json

# Stream one minified JSON snapshot per line for pipelines or log shippers
ktop --noui -d 5m -i 5s -o jsonl --quiet | jq -c '.summary | {health_score, pods_running, oom_kill_count}'
```

When you omit `-d` (`--duration`), `ktop` captures a single point-in-time snapshot (`sample_count: 1`). Sending `SIGINT` or `SIGTERM` during a timed window stops collection cleanly and flushes the samples captured up to that point.

## Standard I/O and Logging Isolation

Because pipeline stages such as `jq` parse every byte written to `stdout`, `ktop --noui` keeps data and diagnostics strictly separated across streams and files:

| Output Stream | Destination | Behavior |
| :--- | :--- | :--- |
| Structured telemetry (`json` / `jsonl`) | `stdout` | Receives only valid JSON (or line-delimited JSON in `-o jsonl`); zero banners or log lines |
| Progress notices | `stderr` | Prints readiness and collection progress unless suppressed with `--quiet` |
| Structured application logs | `~/.ktop/ktop.log` | Default destination controlled by `--log-level` and `--log-format`; pass `--log=stderr` to redirect logs to `stderr` |

For silent automation runs inside cron tasks or containers, add `--quiet` so `stderr` emits nothing unless an error prevents execution:

```bash
ktop --noui --quiet > snapshot.json
```

## Health Score and Policy Gating

Different teams and deployment environments tolerate different levels of transient pod churn, node pressure, or missing metrics. Rather than enforcing a built-in pass/fail opinion inside `ktop`, `--noui` surfaces a deterministic `0`–`100` `health_score` alongside raw failure and saturation counters so your own pipeline scripts express the exact gating policy you want.

### Score Deductions

Each snapshot computes `summary.health_score` starting at `100` and applies bounded deductions for observable cluster conditions:

| Condition | Deduction | Source Field |
| :--- | :--- | :--- |
| Unready nodes | `-30` per unready node | `nodes_count - nodes_ready` |
| Pod failures, OOM kills, or crash loops | `-15` if any count is above `0` | `failed_pods`, `oom_kill_count`, `crashloop_pods` |
| High Pressure Stall Information (PSI) | `-10` if dominant node stall axis exceeds `20%` | `avg_node_cpu_stall_pct`, `avg_node_mem_stall_pct`, `avg_node_io_stall_pct` |
| Active node pressure conditions | `-5` per pressured node | `node_pressure_count` |
| Metrics source unavailable | `-5` if node usage metrics are absent | `has_usage_cpu` / metrics source status |

When running with `--duration`, every `-o json` report also computes `health_score_stats` (`min`, `max`, `avg`, `latest`) across the collected window so checks can distinguish a brief restart blip from sustained degradation.

### CI/CD Gate Examples with `jq`

Using `jq -e`, scripts can turn any boolean policy into an automated exit status after capturing a report:

```bash
# Fail the pipeline if the point-in-time health score drops below 80
ktop --noui --quiet | jq -e '.summary.health_score >= 80'

# Enforce both a sustained floor and an average threshold across a 30s soak window
ktop --noui -d 30s -i 5s --quiet | jq -e '.health_score_stats.min >= 70 and .health_score_stats.avg >= 85'

# Reject a deployment if any OOM kill or CrashLoopBackOff container appeared
ktop --noui --namespace production --quiet | jq -e '.summary.oom_kill_count == 0 and .summary.crashloop_pods == 0'
```

## Useful `jq` Recipes

Because both `-o json` and `-o jsonl` share the same nested fields for `cluster`, `summary`, `nodes`, and `pods`, the same `jq` filters work across single-shot reports and windowed captures.

```bash
# Top 5 pods by working-set memory usage (bytes)
ktop --noui --quiet | jq -r '
  .pods
  | sort_by(-.usage_memory_bytes)
  | .[:5]
  | .[]
  | "\(.namespace)/\(.name)\t\(.usage_memory_bytes)B\t\(.usage_cpu_millis)m"
'

# List pods experiencing non-zero memory PSI stalls
ktop --noui --quiet | jq -r '
  .pods[]
  | select(.psi.mem_stall_pct > 0)
  | "\(.namespace)/\(.name)\tmem_stall=\(.psi.mem_stall_pct)%"
'

# Identify nodes reporting kernel pressure conditions or NotReady status
ktop --noui --quiet | jq '
  [.nodes[] | select(.status != "Ready" or ((.pressures // []) | length > 0)) | {name, status, pressures}]
'
```

## Output Schema Reference (`v0.7.1` Stable)

The top-level JSON structure emitted in `--noui` mode is stable starting in `v0.7.1`. Numeric resource quantities use canonical integer units so downstream tools never need to parse Kubernetes suffix strings: CPU values are expressed in millicores (`millis`), memory and storage values in bytes, and PSI stall or CFS throttling values as `0.0`–`100.0` percentages.

### Single-Document Report (`-o json`)

In `-o json` mode (the default), `ktop` buffers snapshots across the collection window and writes one JSON report object when collection finishes:

```json
{
  "metadata": {
    "ktop_version": "v0.7.1",
    "collection_start": "2026-11-15T12:00:00Z",
    "collection_end": "2026-11-15T12:00:30Z",
    "duration_seconds": 30.012,
    "interval_seconds": 5,
    "sample_count": 7
  },
  "cluster": {
    "context": "prod-cluster",
    "kubernetes_version": "v1.36.0",
    "metrics_source": "prometheus",
    "ready": true,
    "psi_beta": false
  },
  "summary": {
    "health_score": 95,
    "nodes_ready": 3,
    "nodes_count": 3,
    "pods_running": 42,
    "failed_pods": 0,
    "oom_kill_count": 0,
    "crashloop_pods": 0,
    "allocatable_cpu_millis": 12000,
    "usage_cpu_millis": 3100,
    "avg_node_cpu_stall_pct": 0.4
  },
  "health_score_stats": {
    "min": 95,
    "max": 100,
    "avg": 97.14,
    "latest": 95
  },
  "nodes": [],
  "pods": [],
  "workloads": {
    "has_metrics": true,
    "deployments": [],
    "statefulsets": [],
    "daemonsets": [],
    "jobs": []
  },
  "events": {
    "summary": {
      "total": 12,
      "warning_count": 2,
      "normal_count": 10,
      "top_reason": "BackOff",
      "top_reason_count": 2
    },
    "items": []
  },
  "samples": []
}
```

| Field | Presence | Description |
| :--- | :--- | :--- |
| `metadata` | Always | Tool version, UTC collection timestamps, actual elapsed `duration_seconds`, configured `interval_seconds`, and `sample_count` |
| `cluster` | Always | Active kubeconfig `context`, server `kubernetes_version`, active `metrics_source` (`prometheus`, `metrics-server`, or `none`), `ready` boolean, and `psi_beta` flag (`true` on clusters older than `1.36`) |
| `summary` | Always | Latest cluster-wide counts, resource totals, I/O rates, PSI stall averages, and `health_score` |
| `health_score_stats` | Always | Window-level `min`, `max`, `avg`, and `latest` health score across all collected samples |
| `nodes` | Always | Latest slice of node records (`name`, `status`, `pressures`, capacities, usage, and node-root `psi`) |
| `pods` | Always | Latest slice of pod records (`namespace`, `name`, `status`, `restarts`, `psi`, `cpu_throttled_pct`, optional `resize_status`, `disruption_reason`, `resource_claims`, `failing_probes`, and per-container breakdown) |
| `workloads` | When available | Latest cluster workload rollups (`deployments`, `statefulsets`, `daemonsets`, `jobs`) with replica counts, status, and aggregated CPU/memory usage |
| `events` | When available | Recent cluster or namespace-scoped Kubernetes Events with summary totals (`total`, `warning_count`, `normal_count`, `top_reason`) and bounded `items` list |
| `governance` | When KSM available | Auto-discovered `kube-state-metrics` governance state (`hpas`, `pdbs`, `resource_quotas`, `cronjobs`); omitted when KSM is absent or `--ksm=off` |
| `slis` | When SLIs/probes available | Built-in Kubernetes `/metrics/slis` component healthcheck status (`apiserver`, `kubelet`) and `/metrics/probes` availability |
| `samples` | Windowed runs (`-d > 0`) | Ordered array of per-interval snapshot objects (`timestamp`, `cluster`, `summary`, `nodes`, `pods`, `workloads`, `events`, `governance`, `slis`); omitted in single-shot mode |

### Streaming Lines (`-o jsonl`)

In `jsonl` mode, `ktop` emits each `samples` element immediately as one self-contained, single-line JSON object flushed to `stdout` at every `--interval` tick. Each line contains `timestamp`, `cluster`, `summary`, `nodes`, and `pods`.

## Operational Exit Codes

Exit codes report operational execution status so wrapper scripts can tell apart flag mistakes, cluster connectivity problems, missing metrics backends, and readiness timeouts:

| Code | Meaning |
| :--- | :--- |
| `0` | Collection completed and output was written cleanly |
| `1` | General runtime or I/O failure |
| `2` | Invalid CLI flags or arguments (such as an unknown `--output` format or negative `-d`) |
| `3` | Kubernetes API connection or core authorization failure (`pods` or cluster-scoped `nodes`/`namespaces` forbidden) |
| `4` | Explicitly requested metrics source unavailable (for example, `--metrics-source=prometheus` without `nodes/proxy` access) |
| `5` | Cluster informer sync or metrics warmup exceeded `--request-timeout` or internal readiness deadline |

See the [CLI Reference](cli.md) for the complete flag inventory across interactive and headless modes.
