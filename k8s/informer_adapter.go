package k8s

import (
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

type AddObjectEventFunc func(obj interface{})
type UpdateObjectEventFunc func(old, new interface{})
type DeleteObjectEventFunc func(obj interface{})

// InformerAdapter is a wrapper around the GenericInformer
// to make it easy to add callbacks
type InformerAdapter struct {
	informer     informers.GenericInformer
	handlerFuncs *cache.ResourceEventHandlerFuncs
}

func NewInformerAdapter(informer informers.GenericInformer) *InformerAdapter {
	handlers := &cache.ResourceEventHandlerFuncs{}
	informer.Informer().AddEventHandler(handlers)
	return &InformerAdapter{informer: informer, handlerFuncs: handlers}
}

func (c *InformerAdapter) SetAddObjectFunc(fn AddObjectEventFunc) *InformerAdapter {
	c.handlerFuncs.AddFunc = fn
	return c
}

func (c *InformerAdapter) SetUpdateObjectFunc(fn UpdateObjectEventFunc) *InformerAdapter {
	c.handlerFuncs.UpdateFunc = fn
	return c
}

func (c *InformerAdapter) SetDeleteObjectFunc(fn DeleteObjectEventFunc) *InformerAdapter {
	c.handlerFuncs.DeleteFunc = fn
	return c
}

func (c *InformerAdapter) Lister() cache.GenericLister {
	return c.informer.Lister()
}

func (c *InformerAdapter) Informer() cache.SharedIndexInformer {
	return c.informer.Informer()
}
