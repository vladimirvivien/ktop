package controllers

import (
	"fmt"
	"log"
	"time"

	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	informers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type PodController struct {
	clientset kubernetes.Interface

	factory   informers.SharedInformerFactory
	podLister corelisters.PodLister
	podSynced cache.InformerSynced
}

func NewPod(clientset kubernetes.Interface, resync time.Duration) *PodController {
	informerFactory := informers.NewSharedInformerFactory(clientset, resync)
	podInformer := informerFactory.Core().V1().Pods()

	ctrl := &PodController{
		factory:   informerFactory,
		clientset: clientset,
	}

	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ctrl.handleObject,
		UpdateFunc: func(old, new interface{}) {
			fmt.Println("UPDATED: ", new)
			newPod := new.(*coreV1.Pod)
			oldPod := old.(*coreV1.Pod)
			if newPod.ResourceVersion == oldPod.ResourceVersion {
				// only update when new is different from old.
				return
			}
			ctrl.handleObject(new)
		},
		DeleteFunc: ctrl.handleObject,
	})

	ctrl.podLister = podInformer.Lister()
	ctrl.podSynced = podInformer.Informer().HasSynced

	return ctrl
}

func (c *PodController) Run(stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()

	if ok := cache.WaitForCacheSync(stopCh, c.podSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	c.factory.Start(stopCh)
	<-stopCh
	log.Println("Stopping controller")
	return nil
}

func (c *PodController) handleObject(obj interface{}) {
	fmt.Println(obj)
}
