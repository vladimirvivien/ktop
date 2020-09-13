#! /bin/bash
kubectl create secret generic db-credentials \
    --from-literal username=admin \
    --from-literal password=admin
