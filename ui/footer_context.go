package ui

import "fmt"

// FooterContext defines the interface for footer content providers
type FooterContext interface {
	GetItems() []FooterItem
}

func appendPageSwitchFooterItem(items []FooterItem, pageCount int) []FooterItem {
	if pageCount > 1 {
		return append(items, FooterItem{Key: fmt.Sprintf("[1-%d]", pageCount), Action: "page"})
	}
	return items
}

// OverviewContext provides footer items for Overview page panels
type OverviewContext struct {
	FocusedPanel string // "header", "summary", "nodes", "pods"
	PageCount    int
}

// GetItems returns footer items based on focused panel
func (c OverviewContext) GetItems() []FooterItem {
	switch c.FocusedPanel {
	case "header":
		items := appendPageSwitchFooterItem([]FooterItem{{Key: "[Tab]", Action: "next"}}, c.PageCount)
		return append(items,
			FooterItem{Key: "[/]", Action: "filter"},
			FooterItem{Key: "[?]", Action: "help"},
			FooterItem{Key: "[ESC | Ctrl+C]", Action: "quit"},
		)
	case "nodes", "pods":
		items := []FooterItem{
			{Key: "[↑/↓]", Action: "navigate"},
			{Key: "[Enter]", Action: "detail"},
			{Key: "[Tab]", Action: "next"},
		}
		items = appendPageSwitchFooterItem(items, c.PageCount)
		return append(items,
			FooterItem{Key: "[=]", Action: "cols"},
			FooterItem{Key: "[/]", Action: "filter"},
			FooterItem{Key: "[y]", Action: "copy"},
			FooterItem{Key: "[?]", Action: "help"},
			FooterItem{Key: "[ESC | Ctrl+C]", Action: "quit"},
		)
	default: // summary or unknown
		items := appendPageSwitchFooterItem([]FooterItem{{Key: "[Tab]", Action: "next"}}, c.PageCount)
		return append(items,
			FooterItem{Key: "[?]", Action: "help"},
			FooterItem{Key: "[ESC | Ctrl+C]", Action: "quit"},
		)
	}
}

// WorkloadsContext provides footer items for the Workloads top-level page.
type WorkloadsContext struct {
	FocusedPanel string // "header", "summary", "deployments", "statefulsets", "daemonsets", "jobs"
	PageCount    int
}

// GetItems returns footer items based on the focused Workloads section.
func (c WorkloadsContext) GetItems() []FooterItem {
	switch c.FocusedPanel {
	case "header":
		items := appendPageSwitchFooterItem([]FooterItem{{Key: "[Tab]", Action: "next"}}, c.PageCount)
		return append(items,
			FooterItem{Key: "[/]", Action: "filter"},
			FooterItem{Key: "[?]", Action: "help"},
			FooterItem{Key: "[ESC | Ctrl+C]", Action: "quit"},
		)
	case "deployments", "statefulsets", "daemonsets", "jobs":
		items := []FooterItem{
			{Key: "[↑/↓]", Action: "navigate"},
			{Key: "[Enter]", Action: "detail"},
			{Key: "[Tab]", Action: "section"},
		}
		items = appendPageSwitchFooterItem(items, c.PageCount)
		return append(items,
			FooterItem{Key: "[/]", Action: "filter"},
			FooterItem{Key: "[y]", Action: "copy"},
			FooterItem{Key: "[?]", Action: "help"},
			FooterItem{Key: "[ESC | Ctrl+C]", Action: "quit"},
		)
	default:
		items := appendPageSwitchFooterItem([]FooterItem{{Key: "[Tab]", Action: "section"}}, c.PageCount)
		return append(items,
			FooterItem{Key: "[?]", Action: "help"},
			FooterItem{Key: "[ESC | Ctrl+C]", Action: "quit"},
		)
	}
}

// WorkloadDetailContext provides footer items for Workload Detail view.
type WorkloadDetailContext struct {
	FocusedPanel string // "detail", "pods", or "events"
}

// GetItems returns footer items based on the focused Workload Detail subpanel.
func (c WorkloadDetailContext) GetItems() []FooterItem {
	switch c.FocusedPanel {
	case "detail":
		return []FooterItem{
			{Key: "[Enter]", Action: "yaml"},
			{Key: "[Tab]", Action: "next"},
			{Key: "[?]", Action: "help"},
			{Key: "[ESC]", Action: "back"},
			{Key: "[Ctrl+C]", Action: "quit"},
		}
	case "events":
		return []FooterItem{
			{Key: "[↑/↓]", Action: "scroll"},
			{Key: "[Tab]", Action: "next"},
			{Key: "[?]", Action: "help"},
			{Key: "[ESC]", Action: "back"},
			{Key: "[Ctrl+C]", Action: "quit"},
		}
	default: // pods
		return []FooterItem{
			{Key: "[↑/↓]", Action: "navigate"},
			{Key: "[Enter]", Action: "pod detail"},
			{Key: "[Tab]", Action: "next"},
			{Key: "[y]", Action: "copy"},
			{Key: "[?]", Action: "help"},
			{Key: "[ESC]", Action: "back"},
			{Key: "[Ctrl+C]", Action: "quit"},
		}
	}
}

// PageContext provides generic footer items for top-level pages.
type PageContext struct {
	FocusedPanel string
	PageCount    int
	Extra        []FooterItem
}

// GetItems returns footer items for a generic top-level page.
func (c PageContext) GetItems() []FooterItem {
	items := []FooterItem{{Key: "[Tab]", Action: "next"}}
	if c.PageCount > 1 {
		items = append(items, FooterItem{Key: fmt.Sprintf("[1-%d]", c.PageCount), Action: "page"})
	}
	if len(c.Extra) > 0 {
		items = append(items, c.Extra...)
	} else if c.FocusedPanel == "header" {
		items = append(items, FooterItem{Key: "[/]", Action: "filter"})
	}
	return append(items,
		FooterItem{Key: "[?]", Action: "help"},
		FooterItem{Key: "[ESC | Ctrl+C]", Action: "quit"},
	)
}

// NodeDetailContext provides footer items for Node Detail page
type NodeDetailContext struct {
	FocusedPanel string // "events", "pods"
}

// GetItems returns footer items based on focused panel
func (c NodeDetailContext) GetItems() []FooterItem {
	switch c.FocusedPanel {
	case "pods":
		return []FooterItem{
			{Key: "[↑/↓]", Action: "navigate"},
			{Key: "[Enter]", Action: "pod detail"},
			{Key: "[Tab]", Action: "next"},
			{Key: "[y]", Action: "copy"},
			{Key: "[?]", Action: "help"},
			{Key: "[ESC]", Action: "back"},
			{Key: "[Ctrl+C]", Action: "quit"},
		}
	default: // events
		return []FooterItem{
			{Key: "[↑/↓]", Action: "scroll"},
			{Key: "[Tab]", Action: "next"},
			{Key: "[?]", Action: "help"},
			{Key: "[ESC]", Action: "back"},
			{Key: "[Ctrl+C]", Action: "quit"},
		}
	}
}

// PodDetailContext provides footer items for Pod Detail page
type PodDetailContext struct {
	FocusedPanel string // "events", "containers", "volumes"
}

// GetItems returns footer items based on focused panel
func (c PodDetailContext) GetItems() []FooterItem {
	switch c.FocusedPanel {
	case "containers":
		return []FooterItem{
			{Key: "[↑/↓]", Action: "navigate"},
			{Key: "[Enter]", Action: "container"},
			{Key: "[Tab]", Action: "next"},
			{Key: "[n]", Action: "node"},
			{Key: "[y]", Action: "copy"},
			{Key: "[?]", Action: "help"},
			{Key: "[ESC]", Action: "back"},
			{Key: "[Ctrl+C]", Action: "quit"},
		}
	default: // events, volumes
		return []FooterItem{
			{Key: "[↑/↓]", Action: "scroll"},
			{Key: "[Tab]", Action: "next"},
			{Key: "[n]", Action: "node"},
			{Key: "[?]", Action: "help"},
			{Key: "[ESC]", Action: "back"},
			{Key: "[Ctrl+C]", Action: "quit"},
		}
	}
}

// ContainerDetailContext provides footer items for Container Detail page
type ContainerDetailContext struct {
	FocusedPanel string // "detail" or "logs"
}

// GetItems returns footer items based on focused panel
func (c ContainerDetailContext) GetItems() []FooterItem {
	switch c.FocusedPanel {
	case "logs":
		return []FooterItem{
			{Key: "[↑/↓]", Action: "scroll"},
			{Key: "[s]", Action: "stream"},
			{Key: "[t]", Action: "time"},
			{Key: "[w]", Action: "wrap"},
			{Key: "[m]", Action: "more"},
			{Key: "[/]", Action: "filter"},
			{Key: "[x]", Action: "expand"},
			{Key: "[g/G]", Action: "top/btm"},
			{Key: "[?]", Action: "help"},
			{Key: "[ESC]", Action: "back"},
		}
	default: // detail panel
		return []FooterItem{
			{Key: "[↑/↓]", Action: "scroll"},
			{Key: "[Tab]", Action: "next"},
			{Key: "[s]", Action: "spec"},
			{Key: "[?]", Action: "help"},
			{Key: "[ESC]", Action: "back"},
			{Key: "[Ctrl+C]", Action: "quit"},
		}
	}
}
