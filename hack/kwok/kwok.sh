#! /bin/bash 

kwok \
  --kubeconfig=~/.kube/config \
  --manage-all-nodes=true \
  --cidr=10.0.0.1/24 \
  --node-ip=10.0.0.1
