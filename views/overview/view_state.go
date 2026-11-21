package overview

import (
	"strings"
	"sync"

	"github.com/vladimirvivien/ktop/application"
)

// ViewState represents a UI view target (page or modal overlay).
type ViewState struct {
	PageType   application.PageType
	ResourceID string // Identifies the resource being viewed (format depends on PageType)
}

// ViewStateManager tracks the active underlying page state and the independent
// floating Pod/Container Inspector modal state with thread-safe access.
type ViewStateManager struct {
	mu      sync.RWMutex
	current ViewState
	modal   ViewState
}

// NewViewStateManager creates a new view state manager initialized to Overview.
func NewViewStateManager() *ViewStateManager {
	return &ViewStateManager{
		current: ViewState{PageType: application.PageOverview},
	}
}

// SetOverview transitions the base view state to the overview page.
func (m *ViewStateManager) SetOverview() {
	m.mu.Lock()
	m.current = ViewState{PageType: application.PageOverview}
	m.mu.Unlock()
}

// SetNodeDetail transitions the base view state to a specific node detail subpage.
func (m *ViewStateManager) SetNodeDetail(nodeName string) {
	m.mu.Lock()
	m.current = ViewState{
		PageType:   application.PageNodeDetail,
		ResourceID: nodeName,
	}
	m.mu.Unlock()
}

// SetPodModal records an active Pod Inspector modal without altering the underlying page state.
func (m *ViewStateManager) SetPodModal(namespace, podName string) {
	m.mu.Lock()
	m.modal = ViewState{
		PageType:   application.PagePodDetail,
		ResourceID: namespace + "/" + podName,
	}
	m.mu.Unlock()
}

// SetContainerModal records an active Container Logs view inside the Pod Inspector modal.
func (m *ViewStateManager) SetContainerModal(namespace, podName, containerName string) {
	m.mu.Lock()
	m.modal = ViewState{
		PageType:   application.PageContainerLogs,
		ResourceID: namespace + "/" + podName + "/" + containerName,
	}
	m.mu.Unlock()
}

// ClearModal clears the active Pod/Container Inspector modal state.
func (m *ViewStateManager) ClearModal() {
	m.mu.Lock()
	m.modal = ViewState{}
	m.mu.Unlock()
}

// Get returns the current underlying page view state.
func (m *ViewStateManager) Get() ViewState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// GetNodeDetail returns the node name if currently viewing a node detail subpage.
func (m *ViewStateManager) GetNodeDetail() (nodeName string, ok bool) {
	state := m.Get()
	if state.PageType != application.PageNodeDetail {
		return "", false
	}
	return state.ResourceID, true
}

// GetPodModal returns namespace and pod name when the Pod Inspector modal is displaying pod detail.
func (m *ViewStateManager) GetPodModal() (namespace, podName string, ok bool) {
	m.mu.RLock()
	state := m.modal
	m.mu.RUnlock()
	if state.PageType != application.PagePodDetail {
		return "", "", false
	}
	parts := strings.SplitN(state.ResourceID, "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// GetContainerModal returns container identity when the Pod Inspector modal is displaying container logs.
func (m *ViewStateManager) GetContainerModal() (namespace, podName, containerName string, ok bool) {
	m.mu.RLock()
	state := m.modal
	m.mu.RUnlock()
	if state.PageType != application.PageContainerLogs {
		return "", "", "", false
	}
	parts := strings.SplitN(state.ResourceID, "/", 3)
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}
