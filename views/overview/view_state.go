package overview

import (
	"strings"
	"sync"

	"github.com/vladimirvivien/ktop/application"
)

// ViewState represents the current UI view state
type ViewState struct {
	PageType   application.PageType
	ResourceID string // Identifies the resource being viewed (format depends on PageType)
}

// ViewStateManager manages the current view state with thread-safe access.
// This is the single source of truth for what page/resource the UI is displaying.
type ViewStateManager struct {
	mu      sync.RWMutex
	current ViewState
}

// NewViewStateManager creates a new view state manager initialized to Overview
func NewViewStateManager() *ViewStateManager {
	return &ViewStateManager{
		current: ViewState{PageType: application.PageOverview},
	}
}

// SetOverview transitions to the overview page
func (m *ViewStateManager) SetOverview() {
	m.mu.Lock()
	m.current = ViewState{PageType: application.PageOverview}
	m.mu.Unlock()
}

// SetNodeDetail transitions to viewing a specific node
func (m *ViewStateManager) SetNodeDetail(nodeName string) {
	m.mu.Lock()
	m.current = ViewState{
		PageType:   application.PageNodeDetail,
		ResourceID: nodeName,
	}
	m.mu.Unlock()
}

// SetPodDetail transitions to viewing a specific pod
func (m *ViewStateManager) SetPodDetail(namespace, podName string) {
	m.mu.Lock()
	m.current = ViewState{
		PageType:   application.PagePodDetail,
		ResourceID: namespace + "/" + podName,
	}
	m.mu.Unlock()
}

// SetContainerLogs transitions to viewing container logs
func (m *ViewStateManager) SetContainerLogs(namespace, podName, containerName string) {
	m.mu.Lock()
	m.current = ViewState{
		PageType:   application.PageContainerLogs,
		ResourceID: namespace + "/" + podName + "/" + containerName,
	}
	m.mu.Unlock()
}

// Get returns the current view state (thread-safe snapshot)
func (m *ViewStateManager) Get() ViewState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// GetNodeDetail returns the node name if currently viewing a node detail page.
// Returns ("", false) if not on a node detail page.
func (m *ViewStateManager) GetNodeDetail() (nodeName string, ok bool) {
	state := m.Get()
	if state.PageType != application.PageNodeDetail {
		return "", false
	}
	return state.ResourceID, true
}

// GetPodDetail returns the namespace and pod name if currently viewing a pod detail page.
// Returns ("", "", false) if not on a pod detail page.
func (m *ViewStateManager) GetPodDetail() (namespace, podName string, ok bool) {
	state := m.Get()
	if state.PageType != application.PagePodDetail {
		return "", "", false
	}
	parts := strings.SplitN(state.ResourceID, "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// GetContainerLogs returns the container info if currently viewing container logs.
// Returns ("", "", "", false) if not on a container logs page.
func (m *ViewStateManager) GetContainerLogs() (namespace, podName, containerName string, ok bool) {
	state := m.Get()
	if state.PageType != application.PageContainerLogs {
		return "", "", "", false
	}
	parts := strings.SplitN(state.ResourceID, "/", 3)
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}
