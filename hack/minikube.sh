#!/bin/sh

minikube start --nodes 2 --memory=4096 --cpus=2 \
  --kubernetes-version=v1.23.1 \
  --vm-driver=hyperkit \
  --bootstrapper=kubeadm \
  --extra-config=apiserver.enable-admission-plugins="LimitRanger,NamespaceExists,NamespaceLifecycle,ResourceQuota,ServiceAccount,DefaultStorageClass,MutatingAdmissionWebhook"
