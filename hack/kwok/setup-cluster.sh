#!/bin/bash
# KWOK Cluster Setup for ktop Testing
# One-command setup: creates cluster, generates and applies manifests

set -e

CLUSTER_NAME="${KWOK_CLUSTER_NAME:-ktop-test}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Configuration
NUM_NODES="${KWOK_NUM_NODES:-10}"
NODE_CPU="${KWOK_NODE_CPU:-8}"
NODE_MEMORY="${KWOK_NODE_MEMORY:-16Gi}"
PODS_PER_NODE="${KWOK_PODS_PER_NODE:-20}"
POD_NAMESPACE="${KWOK_POD_NAMESPACE:-test-workload}"
ENABLE_METRICS="${KWOK_ENABLE_METRICS:-false}"

echo "=== KWOK Cluster Setup for ktop Testing ==="
echo "Cluster: $CLUSTER_NAME"
echo "Nodes: $NUM_NODES (${NODE_CPU} CPU, ${NODE_MEMORY} memory)"
echo "Pods: $((NUM_NODES * PODS_PER_NODE)) ($PODS_PER_NODE per node, namespace: $POD_NAMESPACE)"
echo "Metrics: $ENABLE_METRICS"
echo ""

# Check if cluster already exists
if kwokctl get clusters 2>/dev/null | grep -q "^$CLUSTER_NAME$"; then
    echo "❌ Cluster '$CLUSTER_NAME' already exists."
    echo "Delete it first with: ./hack/kwok/cleanup-cluster.sh"
    exit 1
fi

# 1. Create metrics configuration if enabled
METRICS_CONFIG=""
if [ "$ENABLE_METRICS" = "true" ]; then
    echo "1. Generating metrics configuration..."
    METRICS_FILE="$SCRIPT_DIR/.metrics-usage.yaml"

    cat > "$METRICS_FILE" <<'EOF'
apiVersion: kwok.x-k8s.io/v1alpha1
kind: ClusterResourceUsage
metadata:
  name: usage-from-annotation
spec:
  usages:
  - usage:
      cpu:
        expression: |
          "kwok.x-k8s.io/usage-cpu" in pod.metadata.annotations
          ? Quantity(pod.metadata.annotations["kwok.x-k8s.io/usage-cpu"])
          : Quantity("1m")
      memory:
        expression: |
          "kwok.x-k8s.io/usage-memory" in pod.metadata.annotations
          ? Quantity(pod.metadata.annotations["kwok.x-k8s.io/usage-memory"])
          : Quantity("1Mi")
---
apiVersion: kwok.x-k8s.io/v1alpha1
kind: Metric
metadata:
  name: metrics-resource
spec:
  metrics:
  - dimension: node
    help: |
      [ALPHA] 1 if there was an error while getting metrics from the node, 0 otherwise
    kind: gauge
    name: scrape_error
    value: "0"
  - dimension: container
    help: |
      [ALPHA] Start time of the container since unix epoch in seconds
    kind: gauge
    labels:
    - name: container
      value: container.name
    - name: namespace
      value: pod.metadata.namespace
    - name: pod
      value: pod.metadata.name
    name: container_start_time_seconds
    value: pod.SinceSecond()
  - dimension: container
    help: |
      [ALPHA] Cumulative cpu time consumed by the container in core-seconds
    kind: counter
    labels:
    - name: container
      value: container.name
    - name: namespace
      value: pod.metadata.namespace
    - name: pod
      value: pod.metadata.name
    name: container_cpu_usage_seconds_total
    value: pod.CumulativeUsage("cpu", container.name)
  - dimension: container
    help: |
      [ALPHA] Current working set of the container in bytes
    kind: gauge
    labels:
    - name: container
      value: container.name
    - name: namespace
      value: pod.metadata.namespace
    - name: pod
      value: pod.metadata.name
    name: container_memory_working_set_bytes
    value: pod.Usage("memory", container.name)
  - dimension: pod
    help: |
      [ALPHA] Cumulative cpu time consumed by the pod in core-seconds
    kind: counter
    labels:
    - name: namespace
      value: pod.metadata.namespace
    - name: pod
      value: pod.metadata.name
    name: pod_cpu_usage_seconds_total
    value: pod.CumulativeUsage("cpu")
  - dimension: pod
    help: |
      [ALPHA] Current working set of the pod in bytes
    kind: gauge
    labels:
    - name: namespace
      value: pod.metadata.namespace
    - name: pod
      value: pod.metadata.name
    name: pod_memory_working_set_bytes
    value: pod.Usage("memory")
  - dimension: node
    help: |
      [ALPHA] Cumulative cpu time consumed by the node in core-seconds
    kind: counter
    name: node_cpu_usage_seconds_total
    value: node.CumulativeUsage("cpu")
  - dimension: node
    help: |
      [ALPHA] Current working set of the node in bytes
    kind: gauge
    name: node_memory_working_set_bytes
    value: node.Usage("memory")
  path: /metrics/nodes/{nodeName}/metrics/resource
EOF

    METRICS_CONFIG="--enable-metrics-server -c $METRICS_FILE"
fi

# 2. Create kwokctl cluster
if [ "$ENABLE_METRICS" = "true" ]; then
    echo "2. Creating kwokctl cluster with metrics-server..."
else
    echo "2. Creating kwokctl cluster..."
fi
kwokctl create cluster --name "$CLUSTER_NAME" $METRICS_CONFIG --wait 60s
kubectl config use-context "kwok-$CLUSTER_NAME"
sleep 5

# 3. Generate nodes manifest
echo "3. Generating nodes manifest..."
NODES_FILE="$SCRIPT_DIR/.nodes.yaml"
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

> "$NODES_FILE"  # Clear file
for i in $(seq 0 $((NUM_NODES - 1))); do
    cat >> "$NODES_FILE" <<EOF
apiVersion: v1
kind: Node
metadata:
  annotations:
    node.alpha.kubernetes.io/ttl: "0"
    kwok.x-k8s.io/node: fake
  labels:
    beta.kubernetes.io/arch: amd64
    beta.kubernetes.io/os: linux
    kubernetes.io/arch: amd64
    kubernetes.io/hostname: kwok-node-$i
    kubernetes.io/os: linux
    kubernetes.io/role: agent
    node-role.kubernetes.io/agent: ""
    type: kwok
  name: kwok-node-$i
spec:
  taints:
  - effect: NoSchedule
    key: kwok.x-k8s.io/node
    value: fake
status:
  allocatable:
    cpu: "$NODE_CPU"
    memory: $NODE_MEMORY
    pods: "110"
  capacity:
    cpu: "$NODE_CPU"
    memory: $NODE_MEMORY
    pods: "110"
  nodeInfo:
    architecture: amd64
    bootID: ""
    containerRuntimeVersion: ""
    kernelVersion: ""
    kubeProxyVersion: fake
    kubeletVersion: fake
    machineID: ""
    operatingSystem: linux
    osImage: ""
    systemUUID: ""
  phase: Running
  conditions:
  - lastHeartbeatTime: $TIMESTAMP
    lastTransitionTime: $TIMESTAMP
    message: kubelet is posting ready status
    reason: KubeletReady
    status: "True"
    type: Ready
  addresses:
  - address: 10.0.0.$i
    type: InternalIP
---
EOF
done

# 4. Apply nodes
echo "4. Creating $NUM_NODES nodes..."
kubectl apply -f "$NODES_FILE"

# 5. Generate pods manifest
echo "5. Generating pods manifest..."
PODS_FILE="$SCRIPT_DIR/.pods.yaml"
kubectl create namespace "$POD_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f - > /dev/null

> "$PODS_FILE"  # Clear file
POD_COUNT=0
for node_idx in $(seq 0 $((NUM_NODES - 1))); do
    for pod_idx in $(seq 1 $PODS_PER_NODE); do
        POD_COUNT=$((POD_COUNT + 1))
        CPU_REQ=$((100 + RANDOM % 400))
        MEM_REQ=$((128 + RANDOM % 384))

        # Calculate usage metrics (50-90% of requests) if metrics enabled
        USAGE_ANNOTATIONS=""
        if [ "$ENABLE_METRICS" = "true" ]; then
            CPU_USAGE=$((CPU_REQ * (50 + RANDOM % 40) / 100))
            MEM_USAGE=$((MEM_REQ * (50 + RANDOM % 40) / 100))
            USAGE_ANNOTATIONS="    kwok.x-k8s.io/usage-cpu: ${CPU_USAGE}m
    kwok.x-k8s.io/usage-memory: ${MEM_USAGE}Mi"
        fi

        cat >> "$PODS_FILE" <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: app-$(printf "%04d" $POD_COUNT)
  namespace: $POD_NAMESPACE
  labels:
    app: test-app
  annotations:
$USAGE_ANNOTATIONS
spec:
  nodeName: kwok-node-$node_idx
  containers:
  - name: app
    image: nginx:latest
    resources:
      requests:
        cpu: ${CPU_REQ}m
        memory: ${MEM_REQ}Mi
      limits:
        cpu: $((CPU_REQ * 2))m
        memory: $((MEM_REQ * 2))Mi
status:
  phase: Running
  conditions:
  - type: Ready
    status: "True"
    lastTransitionTime: $TIMESTAMP
  containerStatuses:
  - name: app
    ready: true
    restartCount: 0
    image: nginx:latest
    imageID: docker-pullable://nginx@sha256:fake
    containerID: docker://fake
    state:
      running:
        startedAt: $TIMESTAMP
  hostIP: 10.0.0.$node_idx
  podIP: 10.244.$node_idx.$pod_idx
  startTime: $TIMESTAMP
---
EOF
    done
done

# 6. Apply pods
echo "6. Creating $POD_COUNT pods..."
kubectl apply -f "$PODS_FILE"

# Cleanup temporary files
rm -f "$NODES_FILE" "$PODS_FILE"
if [ "$ENABLE_METRICS" = "true" ]; then
    rm -f "$METRICS_FILE"
fi

echo ""
echo "=== ✅ Setup Complete! ==="
echo ""
echo "Cluster: $CLUSTER_NAME"
echo "Nodes: $NUM_NODES"
echo "Pods: $POD_COUNT"
if [ "$ENABLE_METRICS" = "true" ]; then
    echo "Metrics: Enabled (metrics-server running)"
    echo ""
    echo "⚠️  Wait ~45 seconds for metrics-server to collect initial metrics"
fi
echo ""
echo "Verify:"
echo "  kubectl get nodes"
echo "  kubectl get pods -n $POD_NAMESPACE"
if [ "$ENABLE_METRICS" = "true" ]; then
    echo "  kubectl top nodes"
    echo "  kubectl top pods -n $POD_NAMESPACE"
fi
echo ""
echo "Test ktop:"
echo "  ./ktop"
echo ""
echo "Cleanup:"
echo "  ./hack/kwok/cleanup-cluster.sh"
echo ""
