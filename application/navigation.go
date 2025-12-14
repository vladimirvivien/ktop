package application

// PageType represents the type of page in the navigation stack
type PageType string

const (
	PageOverview    PageType = "overview"
	PageNodeDetail  PageType = "node_detail"
	PagePodDetail   PageType = "pod_detail"
)

// PageState represents a page in the navigation stack
type PageState struct {
	PageType   PageType
	ResourceID string // e.g., "minikube" for node, "kube-system/coredns-xyz" for pod
	ScrollPos  int    // Scroll position to restore when navigating back
}

// NavigationStack manages page navigation history
type NavigationStack struct {
	stack []PageState
}

// NewNavigationStack creates a new navigation stack with the overview as the initial page
func NewNavigationStack() *NavigationStack {
	return &NavigationStack{
		stack: []PageState{
			{PageType: PageOverview},
		},
	}
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

// Clear resets the stack to just the overview page
func (n *NavigationStack) Clear() {
	n.stack = []PageState{
		{PageType: PageOverview},
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
