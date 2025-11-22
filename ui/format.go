package ui

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
)

// FormatMemory formats a memory quantity in the most appropriate unit (Mi or Gi)
// Uses Gi only when >= 10Gi to match kubectl behavior, otherwise uses Mi
func FormatMemory(qty *resource.Quantity) string {
	if qty == nil {
		return "0Mi"
	}

	bytes := qty.Value()
	if bytes == 0 {
		return "0Mi"
	}

	// Calculate Mi (binary)
	mi := bytes / (1024 * 1024)

	// Only use Gi for very large values (>= 10240 Mi = 10 Gi)
	// This matches kubectl's behavior of preferring Mi for smaller values
	if mi >= 10240 {
		gi := bytes / (1024 * 1024 * 1024)
		return fmt.Sprintf("%dGi", gi)
	}

	// Display in Mi for everything else
	return fmt.Sprintf("%dMi", mi)
}
