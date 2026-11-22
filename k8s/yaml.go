package k8s

import (
	"encoding/json"
	"fmt"

	"sigs.k8s.io/yaml"
)

const redactedSecretValue = "<redacted>"

// FormatObjectYAML serializes a Kubernetes object (or sub-spec) to YAML without
// mutating the source object. It removes noisy metadata.managedFields and redacts
// Secret data/stringData values so sensitive material is never rendered directly.
func FormatObjectYAML(obj any) (string, error) {
	if obj == nil {
		return "", fmt.Errorf("no object available")
	}
	rawJSON, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}
	var generic map[string]any
	if err := json.Unmarshal(rawJSON, &generic); err == nil && generic != nil {
		if meta, ok := generic["metadata"].(map[string]any); ok {
			delete(meta, "managedFields")
		}
		if kind, ok := generic["kind"].(string); ok && kind == "Secret" {
			redactSecretMap(generic, "data")
			redactSecretMap(generic, "stringData")
		}
		out, err := yaml.Marshal(generic)
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
	out, err := yaml.Marshal(obj)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func redactSecretMap(root map[string]any, field string) {
	entries, ok := root[field].(map[string]any)
	if !ok {
		return
	}
	for k := range entries {
		entries[k] = redactedSecretValue
	}
}
