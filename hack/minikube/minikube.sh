#!/bin/sh

minikube start --nodes 4 --memory=4096 --cpus=2 \
  --vm-driver=docker \
  --bootstrapper=kubeadm \
  --extra-config=apiserver.enable-admission-plugins="LimitRanger,NamespaceExists,NamespaceLifecycle,ResourceQuota,ServiceAccount,DefaultStorageClass,MutatingAdmissionWebhook"
