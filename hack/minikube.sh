#!/bin/sh

minikube start --memory=2192 --cpus=2 \
  --kubernetes-version=v1.20.0 \
  --vm-driver=docker \
  --bootstrapper=kubeadm \
  --extra-config=apiserver.enable-admission-plugins="LimitRanger,NamespaceExists,NamespaceLifecycle,ResourceQuota,ServiceAccount,DefaultStorageClass,MutatingAdmissionWebhook"
