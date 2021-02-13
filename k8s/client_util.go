package k8s

import (
	"os"
	"path/filepath"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// findKubeCfgFile looks for possible location for kubeconfig file
func findKubeCfgFile() (string, error) {
	// try KUBECONFIG env
	kubecfg := os.Getenv(clientcmd.RecommendedConfigPathEnvVar)
	if kubecfg != "" {
		return kubecfg, nil
	}

	// return known default
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, clientcmd.RecommendedHomeDir, clientcmd.RecommendedFileName), nil
}

func loadConfig(kubeconfig, context string) (*rest.Config, error) {
	if kubeconfig == ""{
		kcfg, err := findKubeCfgFile()
		if err != nil {
			return nil, err
		}
		kubeconfig = kcfg
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
		&clientcmd.ConfigOverrides{
			CurrentContext: context,
		}).ClientConfig()
}
