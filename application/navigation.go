package application

import "strings"

// PageType represents the type of page in the navigation stack
type PageType string

const (
	PageOverview       PageType = "overview"
	PageWorkloads      PageType = "workloads"
	PageWorkloadDetail PageType = "workload_detail"
	PageEvents         PageType = "events"
	PageNodeDetail     PageType = "node_detail"
	PagePodDetail      PageType = "pod_detail"
	PageContainerLogs  PageType = "container_logs"
	PageContainerSpec  PageType = "container_spec"
)

// PageState represents a page in the navigation stack
type PageState struct {
	PageType   PageType
	ResourceID string // e.g., "minikube" for node, "kube-system/coredns-xyz" for pod
	ScrollPos  int    // Scroll position to restore when navigating back
}

// NavigationStack manages page navigation history
type NavigationStack struct {
	rootType PageType
	stack    []PageState
}

// NewNavigationStack creates a new navigation stack with the overview as the initial page
func NewNavigationStack() *NavigationStack {
	return NewNavigationStackWithRoot(PageOverview)
}

// NewNavigationStackWithRoot creates a navigation stack rooted at root.
func NewNavigationStackWithRoot(root PageType) *NavigationStack {
	if root == "" {
		root = PageOverview
	}
	return &NavigationStack{
		rootType: root,
		stack: []PageState{
			{PageType: root},
		},
	}
}

func (n *NavigationStack) rootPageType() PageType {
	if n.rootType == "" {
		return PageOverview
	}
	return n.rootType
}

// Push adds a new page to the stack
func (n *NavigationStack) Push(state PageState) {
	n.stack = append(n.stack, state)
}

// Pop removes and returns the top page from the stack
// Returns nil if only the root page (overview) remains
func (n *NavigationStack) Pop() *PageState {
	if len(n.stack) <= 1 {
		return nil // Can't pop the root page
	}
	// Remove and return the last item
	lastIdx := len(n.stack) - 1
	popped := n.stack[lastIdx]
	n.stack = n.stack[:lastIdx]
	return &popped
}

// Current returns the current (top) page state
func (n *NavigationStack) Current() *PageState {
	if len(n.stack) == 0 {
		return nil
	}
	return &n.stack[len(n.stack)-1]
}

// Previous returns the previous page state (one below current)
// Returns nil if at the root page
func (n *NavigationStack) Previous() *PageState {
	if len(n.stack) <= 1 {
		return nil
	}
	return &n.stack[len(n.stack)-2]
}

// CanGoBack returns true if there's a page to go back to
func (n *NavigationStack) CanGoBack() bool {
	return len(n.stack) > 1
}

// Depth returns the current stack depth
func (n *NavigationStack) Depth() int {
	return len(n.stack)
}

// Clear resets the stack to its root page
func (n *NavigationStack) Clear() {
	n.stack = []PageState{
		{PageType: n.rootPageType()},
	}
}

// UpdateCurrentScrollPos updates the scroll position of the current page
func (n *NavigationStack) UpdateCurrentScrollPos(scrollPos int) {
	if len(n.stack) > 0 {
		n.stack[len(n.stack)-1].ScrollPos = scrollPos
	}
}

// Breadcrumb returns a slice of page states representing the navigation path
func (n *NavigationStack) Breadcrumb() []PageState {
	result := make([]PageState, len(n.stack))
	copy(result, n.stack)
	return result
}

// FormatBreadcrumb formats a drill-down navigation stack for display in the tab bar.
// Returns an empty string when only the root page is on the stack.
func FormatBreadcrumb(pageTitle string, states []PageState) string {
	if len(states) <= 1 {
		return ""
	}

	rootLabel := strings.TrimSpace(pageTitle)
	if rootLabel == "" {
		rootLabel = string(states[0].PageType)
	}

	segments := make([]string, 0, len(states))
	segments = append(segments, rootLabel)
	for _, s := range states[1:] {
		switch s.PageType {
		case PageNodeDetail:
			segments = append(segments, "node/"+s.ResourceID)
		case PagePodDetail:
			segments = append(segments, "pod/"+s.ResourceID)
		case PageWorkloadDetail:
			segments = append(segments, formatWorkloadBreadcrumb(s.ResourceID))
		case PageContainerLogs:
			segments = append(segments, "logs/"+lastResourceSegment(s.ResourceID))
		case PageContainerSpec:
			segments = append(segments, "spec/"+lastResourceSegment(s.ResourceID))
		default:
			if s.ResourceID != "" {
				segments = append(segments, string(s.PageType)+"/"+s.ResourceID)
			} else {
				segments = append(segments, string(s.PageType))
			}
		}
	}
	return strings.Join(segments, " › ")
}

func lastResourceSegment(resourceID string) string {
	idx := strings.LastIndex(resourceID, "/")
	if idx < 0 || idx+1 >= len(resourceID) {
		return resourceID
	}
	return resourceID[idx+1:]
}

func formatWorkloadBreadcrumb(resourceID string) string {
	parts := strings.SplitN(resourceID, "/", 3)
	if len(parts) != 3 {
		return "workload/" + resourceID
	}
	prefix := strings.ToLower(strings.TrimSpace(parts[0]))
	switch prefix {
	case "deployment":
		prefix = "deploy"
	case "statefulset":
		prefix = "sts"
	case "daemonset":
		prefix = "ds"
	case "job":
		prefix = "job"
	}
	return prefix + "/" + parts[1] + "/" + parts[2]
}
