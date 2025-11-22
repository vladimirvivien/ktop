package k8s

import (
	"context"
	"fmt"
	"sync"
	"time"

	appsV1 "k8s.io/api/apps/v1"
	authzV1 "k8s.io/api/authorization/v1"
	batchV1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd/api"
	metricsapi "k8s.io/metrics/pkg/apis/metrics"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

const (
	AllNamespaces = metav1.NamespaceAll
)

var (
	// GVRs used
	GVRs = map[string]schema.GroupVersionResource{
		"nodes":                  {Group: "", Version: "v1", Resource: "nodes"},
		"namespaces":             {Group: "", Version: "v1", Resource: "namespaces"},
		"pods":                   {Group: "", Version: "v1", Resource: "pods"},
		"persistentvolumes":      {Group: "", Version: "v1", Resource: "persistentvolumes"},
		"persistentvolumeclaims": {Group: "", Version: "v1", Resource: "persistentvolumeclaims"},
		"deployments":            {Group: appsV1.GroupName, Version: "v1", Resource: "deployments"},
		"daemonsets":             {Group: appsV1.GroupName, Version: "v1", Resource: "daemonsets"},
		"replicasets":            {Group: appsV1.GroupName, Version: "v1", Resource: "replicasets"},
		"statefulsets":           {Group: appsV1.GroupName, Version: "v1", Resource: "statefulsets"},
		"jobs":                   {Group: batchV1.GroupName, Version: "v1", Resource: "jobs"},
		"cronjobs":               {Group: batchV1.GroupName, Version: "v1", Resource: "cronjobs"},
	}

	authzdTable = make(map[string]bool)
)

type Client struct {
	sync.RWMutex
	clusterVersion    *version.Info
	namespace         string
	config            *restclient.Config
	apiConfig         api.Config
	clusterContext    string
	username          string
	kubeClient        kubernetes.Interface
	discoClient       discovery.CachedDiscoveryInterface
	metricsClient     *metricsclient.Clientset
	metricsAvailCount int
	refreshTimeout    time.Duration
	controller        *Controller
}

func New(flags *genericclioptions.ConfigFlags) (*Client, error) {
	if flags == nil {
		return nil, fmt.Errorf("configuration flagset is nil")
	}

	config, err := flags.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	disco, err := flags.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}

	metrics, err := metricsclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	var namespace = *flags.Namespace

	apiCfg, err := flags.ToRawKubeConfigLoader().RawConfig()
	if err != nil {
		return nil, err
	}

	username := "<empty>"
	currCtx, ok := apiCfg.Contexts[apiCfg.CurrentContext]
	if ok {
		username = currCtx.AuthInfo
	}

	// get api server version
	version, err := disco.ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server to get version: %s", err)
	}

	client := &Client{
		clusterVersion: version,
		namespace:      namespace,
		config:         config,
		apiConfig:      apiCfg,
		clusterContext: apiCfg.CurrentContext,
		username:       username,
		kubeClient:     kubeClient,
		discoClient:    disco,
		metricsClient:  metrics,
	}
	client.controller = newController(client)
	return client, nil
}

func (k8s *Client) Namespace() string {
	return k8s.namespace
}

func (k8s *Client) RESTConfig() *restclient.Config {
	return k8s.config
}

func (k8s *Client) ClusterContext() string {
	return k8s.clusterContext
}

func (k8s *Client) Username() string {
	return k8s.username
}

func (k8s *Client) GetServerVersion() string {
	return k8s.clusterVersion.String()
}

// AssertMetricsAvailable checks for available metrics server every 10th invocation.
// Otherwise, it returns the last known registration state of metrics server.
func (k8s *Client) AssertMetricsAvailable() error {
	k8s.Lock()
	defer k8s.Unlock()

	if k8s.metricsAvailCount != 0 {
		if k8s.metricsAvailCount%10 != 0 {
			k8s.metricsAvailCount++
		} else {
			k8s.metricsAvailCount = 0
		}
		return nil
	}

	groups, err := k8s.discoClient.ServerGroups()
	if err != nil {
		return err
	}

	avail := false
	for _, group := range groups.Groups {
		if group.Name == metricsapi.GroupName {
			avail = true
			break
		}
	}

	if !avail {
		return fmt.Errorf("metrics api not available")
	}

	k8s.metricsAvailCount++

	return nil
}

func (k8s *Client) Controller() *Controller {
	return k8s.controller
}

func (k8s *Client) GetMetricsClient() *metricsclient.Clientset {
	return k8s.metricsClient
}

// IsAuthz checks access authorization using SelfSubjectAccessReview
func (k8s *Client) IsAuthz(ctx context.Context, resource string, verbs []string) (bool, error) {
	k8s.Lock()
	defer k8s.Unlock()

	gvr, ok := GVRs[resource]
	if !ok {
		return false, fmt.Errorf("unsupported resource %s", resource)
	}

	makeAccessReview := func(grv schema.GroupVersionResource, verb string) *authzV1.SelfSubjectAccessReview {
		return &authzV1.SelfSubjectAccessReview{
			Spec: authzV1.SelfSubjectAccessReviewSpec{
				ResourceAttributes: &authzV1.ResourceAttributes{
					Namespace: k8s.Namespace(),
					Group:     grv.Group,
					Version:   grv.Version,
					Resource:  grv.Resource,
					Verb:      verb,
				},
			},
		}
	}

	arClient := k8s.kubeClient.AuthorizationV1().SelfSubjectAccessReviews()
	result := true
	for _, verb := range verbs {
		key := fmt.Sprintf("%s/%s/%s", gvr.String(), k8s.Namespace(), verb)
		if authzd, ok := authzdTable[key]; ok {
			result = result && authzd
			continue
		}
		ar := makeAccessReview(gvr, verb)
		arResult, err := arClient.Create(ctx, ar, metav1.CreateOptions{})
		if err != nil {
			delete(authzdTable, key)
			return false, err
		}
		allowed := arResult.Status.Allowed
		authzdTable[key] = allowed
		result = result && allowed
	}

	return result, nil
}

// AssertCoreAuthz asserts that user/context can access node and pods
func (k8s *Client) AssertCoreAuthz(ctx context.Context) error {
	resources := []string{"namespaces", "nodes", "pods"}
	accessible := true
	for _, res := range resources {
		authzd, err := k8s.IsAuthz(ctx, res, []string{"get", "list"})
		if err != nil {
			return err
		}
		accessible = accessible && authzd
	}
	if !accessible {
		return fmt.Errorf("user missing required authorizations")
	}
	return nil
}
