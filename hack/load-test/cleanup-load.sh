#!/bin/bash
#
# cleanup-load.sh - Remove all load-test pods and namespace
#
# Usage:
#   ./cleanup-load.sh                         # Cleanup default namespace
#   LOAD_NAMESPACE=custom ./cleanup-load.sh   # Cleanup custom namespace
#
set -e

NAMESPACE="${LOAD_NAMESPACE:-load-test}"

echo "=== Cleaning up load-test deployment ==="
echo ""

# Check if namespace exists
if ! kubectl get namespace "${NAMESPACE}" &>/dev/null; then
    echo "ℹ Namespace '${NAMESPACE}' does not exist. Nothing to clean up."
    exit 0
fi

# Show what will be deleted
POD_COUNT=$(kubectl get pods -n "${NAMESPACE}" --no-headers 2>/dev/null | wc -l | tr -d ' ')
echo "→ Found ${POD_COUNT} pods in namespace '${NAMESPACE}'"

# Delete namespace (this deletes all resources in it)
echo "→ Deleting namespace '${NAMESPACE}'..."
kubectl delete namespace "${NAMESPACE}" --wait=false

echo ""
echo "✓ Cleanup initiated. Namespace deletion in progress."
echo ""
echo "Note: Pods may take a few seconds to terminate."
echo "Check status: kubectl get pods -n ${NAMESPACE}"
