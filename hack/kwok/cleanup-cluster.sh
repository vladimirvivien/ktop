#!/bin/bash
# KWOK Cluster Cleanup Script

CLUSTER_NAME="${KWOK_CLUSTER_NAME:-ktop-test}"

echo "=== Cleaning up KWOK cluster ==="
echo "Cluster: $CLUSTER_NAME"
echo ""

# Check if cluster exists
if ! kwokctl get clusters | grep -q "^$CLUSTER_NAME$"; then
    echo "Cluster '$CLUSTER_NAME' does not exist."
    echo "Available clusters:"
    kwokctl get clusters
    exit 1
fi

# Delete the cluster
kwokctl delete cluster --name "$CLUSTER_NAME"

echo ""
echo "Cluster deleted successfully!"
echo ""
echo "To recreate the cluster, run:"
echo "  ./hack/kwok/setup-cluster.sh"
echo ""
