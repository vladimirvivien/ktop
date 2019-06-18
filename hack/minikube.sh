#!/bin/sh

minikube start --memory=8192 --cpus=4 \
  --kubernetes-version=v1.11.3 \
  --vm-driver=vmwarefusion \
  --bootstrapper=kubeadm \
  --extra-config=apiserver.enable-admission-plugins="LimitRanger,NamespaceExists,NamespaceLifecycle,ResourceQuota,ServiceAccount,DefaultStorageClass,MutatingAdmissionWebhook"
