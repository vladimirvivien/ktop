package k8s

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/dynamic/fake"
)

func NewUnstructuredTestObj(apiVersion, kind, namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata": map[string]interface{}{
				"namespace": namespace,
				"name":      name,
			},
			"spec": name,
		},
	}
}

func TestCoordController(t *testing.T) {
	tests := []struct {
		name      string
		ctrlFunc  func(dynamicinformer.DynamicSharedInformerFactory, schema.GroupVersionResource, chan *unstructured.Unstructured) *InformerAdapter
		eventFunc func(schema.GroupVersionResource, *fake.FakeDynamicClient) *unstructured.Unstructured
		grv       schema.GroupVersionResource
	}{
		{
			name: "objectAdded event",
			grv:  schema.GroupVersionResource{Group: "extensions", Version: "v1beta1", Resource: "deployments"},
			ctrlFunc: func(fac dynamicinformer.DynamicSharedInformerFactory, grv schema.GroupVersionResource, objChan chan *unstructured.Unstructured) *InformerAdapter {
				inf := fac.ForResource(grv)
				ctrl := NewInformerAdapter(inf)
				ctrl.SetAddObjectFunc(func(obj interface{}) {
					objChan <- obj.(*unstructured.Unstructured)
				})
				if ctrl.handlerFuncs.AddFunc == nil {
					t.Error("EventHandlerFunc objectAddedFunc not set properly")
				}
				return ctrl
			},
			eventFunc: func(grv schema.GroupVersionResource, client *fake.FakeDynamicClient) *unstructured.Unstructured {
				testObject := NewUnstructuredTestObj("extensions/v1beta1", "Deployment", "test-ns", "test-name")
				createdObj, err := client.Resource(grv).Namespace("test-ns").Create(testObject, metav1.CreateOptions{})
				if err != nil {
					t.Error(err)
				}
				return createdObj
			},
		},
		{
			name: "objectUpdated event",
			grv:  schema.GroupVersionResource{Group: "group", Version: "ver1", Resource: "fooobjs"},
			ctrlFunc: func(fac dynamicinformer.DynamicSharedInformerFactory, grv schema.GroupVersionResource, objChan chan *unstructured.Unstructured) *InformerAdapter {
				inf := fac.ForResource(grv)
				ctrl := NewInformerAdapter(inf)
				ctrl.SetUpdateObjectFunc(func(old, new interface{}) {
					objChan <- new.(*unstructured.Unstructured)
				})
				if ctrl.handlerFuncs.UpdateFunc == nil {
					t.Error("EventHandlerFunc objectUpdatedFunc not set properly")
				}
				return ctrl
			},
			eventFunc: func(grv schema.GroupVersionResource, client *fake.FakeDynamicClient) *unstructured.Unstructured {
				testObject := NewUnstructuredTestObj("group/v1", "FooObj", "test-ns", "test-name")
				createdObj, err := client.Resource(grv).Namespace("test-ns").Create(testObject, metav1.CreateOptions{})
				if err != nil {
					t.Error(err)
				}
				createdObj.Object["spec"] = "newSpecName"
				updatedObj, err := client.Resource(grv).Namespace("test-ns").Update(createdObj, metav1.UpdateOptions{})
				return updatedObj
			},
		},

		{
			name: "objectDeleted event",
			grv:  schema.GroupVersionResource{Group: "group2", Version: "v2beta1", Resource: "barobjs"},
			ctrlFunc: func(fac dynamicinformer.DynamicSharedInformerFactory, grv schema.GroupVersionResource, objChan chan *unstructured.Unstructured) *InformerAdapter {
				inf := fac.ForResource(grv)
				ctrl := NewInformerAdapter(inf)
				ctrl.SetDeleteObjectFunc(func(obj interface{}) {
					objChan <- obj.(*unstructured.Unstructured)
				})
				if ctrl.handlerFuncs.DeleteFunc == nil {
					t.Error("EventHandlerFunc objectDeletedFunc not set properly")
				}
				return ctrl
			},
			eventFunc: func(grv schema.GroupVersionResource, client *fake.FakeDynamicClient) *unstructured.Unstructured {
				testObject := NewUnstructuredTestObj("group2/v2beta1", "BarObj", "test-ns", "test-name")
				createdObj, err := client.Resource(grv).Namespace("test-ns").Create(testObject, metav1.CreateOptions{})
				if err != nil {
					t.Error(err)
				}
				if err := client.Resource(grv).Namespace("test-ns").Delete(createdObj.GetName(), &metav1.DeleteOptions{}); err != nil {
					t.Error(err)
				}
				return createdObj
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			timeout := time.Duration(3 * time.Second)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			objChan := make(chan *unstructured.Unstructured, 1)

			client := fake.NewSimpleDynamicClient(runtime.NewScheme(), []runtime.Object{}...)
			fac := dynamicinformer.NewDynamicSharedInformerFactory(client, 0)
			test.ctrlFunc(fac, test.grv, objChan)

			fac.Start(ctx.Done())
			if synced := fac.WaitForCacheSync(ctx.Done()); !synced[test.grv] {
				t.Errorf("informer for %s hasn't synced", test.grv)
			}

			eventObj := test.eventFunc(test.grv, client)
			select {
			case rcvdObj := <-objChan:
				if !equality.Semantic.DeepEqual(eventObj, rcvdObj) {
					t.Fatalf("%v", diff.ObjectDiff(eventObj, rcvdObj))
				}
			case <-ctx.Done():
				t.Errorf("informer did not receive object, timed out after %v", timeout)
			}
		})
	}
}
