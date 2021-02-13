package k8s

import (
	"context"

	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
)

type NodeWatcher struct {
	client    *Client
	listFuncs []ListFunc
}

func NewNodeWatcher(client *Client) *NodeWatcher {
	return &NodeWatcher{client: client}
}

func (nw *NodeWatcher) AddNodeListFunc(fn ListFunc) {
	nw.listFuncs = append(nw.listFuncs, fn)
}

func (nw *NodeWatcher) Start(ctx context.Context) error {
	watcher, err := nw.client.ResourceInterface(NodesResource).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	nodesCh := make(chan *coreV1.NodeList, 1)

	go func() {
		for {
			select {
			case event := <-watcher.ResultChan():
				if event.Object == nil {
					continue
				}

				// react on Add, Delete, Modified events
				switch event.Type {
				case watch.Added, watch.Deleted, watch.Modified:
					listObj, err := nw.client.ResourceInterface(NodesResource).List(ctx, metav1.ListOptions{})
					if err != nil {
						continue
					}

					nodeList := new(coreV1.NodeList)
					if err := runtime.DefaultUnstructuredConverter.FromUnstructured(listObj.UnstructuredContent(), nodeList); err != nil {
						continue
					}

					// launch handler functions
					nodesCh <- nodeList
				}
			case <-ctx.Done():
				watcher.Stop()
			}
		}
	}()

	// func processor
	go func() {
		for nodeList := range nodesCh {
			if len(nw.listFuncs) == 0 {
				continue
			}
			for _, fn := range nw.listFuncs {
				if err := fn(ctx, nw.client.namespace, nodeList); err != nil {
					continue
				}
			}
		}
	}()

	return nil
}
