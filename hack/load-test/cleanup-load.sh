#!/bin/bash
#
# cleanup-load.sh - Remove all load-test workloads, pods, and namespace
#
# Usage:
#   ./cleanup-load.sh                         # Cleanup default namespace
#   LOAD_NAMESPACE=custom ./cleanup-load.sh   # Cleanup custom namespace
#
set -e

NAMESPACE="${LOAD_NAMESPACE:-load-test}"

echo "=== Cleaning up load-test workloads ==="
echo ""

if ! kubectl get namespace "${NAMESPACE}" &>/dev/null; then
    echo "ℹ Namespace '${NAMESPACE}' does not exist. Nothing to clean up."
    exit 0
fi

POD_COUNT=$(kubectl get pods -n "${NAMESPACE}" --no-headers 2>/dev/null | wc -l | tr -d ' ')
WL_COUNT=$(kubectl get deployments,statefulsets,daemonsets,jobs,cronjobs -n "${NAMESPACE}" --no-headers 2>/dev/null | wc -l | tr -d ' ')
echo "-> Found ${WL_COUNT} workload controller objects and ${POD_COUNT} pods in namespace '${NAMESPACE}'"

echo "-> Deleting namespace '${NAMESPACE}'..."
kubectl delete namespace "${NAMESPACE}" --wait=false

echo ""
echo "✓ Cleanup initiated. Namespace deletion in progress."
echo "Check status: kubectl get all -n ${NAMESPACE}"
