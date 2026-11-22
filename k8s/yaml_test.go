package k8s

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFormatObjectYAML_StripsManagedFieldsAndRedactsSecrets(t *testing.T) {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "db-creds",
			Namespace: "prod",
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Operation: metav1.ManagedFieldsOperationApply},
			},
		},
		Data: map[string][]byte{
			"password": []byte("super-secret-value"),
		},
		StringData: map[string]string{
			"token": "raw-token-value",
		},
	}

	out, err := FormatObjectYAML(secret)
	if err != nil {
		t.Fatalf("FormatObjectYAML() error: %v", err)
	}
	if strings.Contains(out, "managedFields") {
		t.Fatalf("expected managedFields stripped, got:\n%s", out)
	}
	if strings.Contains(out, "super-secret-value") || strings.Contains(out, "raw-token-value") || strings.Contains(out, "c3VwZXItc2VjcmV0LXZhbHVl") {
		t.Fatalf("expected secret values redacted, got:\n%s", out)
	}
	if !strings.Contains(out, "<redacted>") {
		t.Fatalf("expected <redacted> marker in Secret YAML, got:\n%s", out)
	}
	// Source object must not be mutated.
	if len(secret.ManagedFields) != 1 || string(secret.Data["password"]) != "super-secret-value" {
		t.Fatalf("source Secret object was mutated")
	}
}
