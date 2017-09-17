package kclient

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type KClient struct {
	k8s kubernetes.Interface
}

func New(k8s kubernetes.Interface) *KClient {
	return &KClient{k8s}
}


func (c *KClient) GetPodsByNS(ns string) ([]v1.Pod, error) {
	pods, err := c.k8s.Core().Pods(ns).List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pods.Items, nil
}

