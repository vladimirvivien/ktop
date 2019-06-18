package deployments

import (
	appsV1 "k8s.io/api/apps/v1"
)

// toDeploymentSlice converts []*apps.Deployment -> []apps.Deployment
func toDeploymentSlice(deps []*appsV1.Deployment) (out []appsV1.Deployment) {
	for _, ptr := range deps {
		out = append(out, *ptr)
	}
	return
}
