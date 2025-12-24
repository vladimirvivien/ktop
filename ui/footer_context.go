package ui

// FooterContext defines the interface for footer content providers
type FooterContext interface {
	GetItems() []FooterItem
}

// OverviewContext provides footer items for Overview page panels
type OverviewContext struct {
	FocusedPanel string // "header", "summary", "nodes", "pods"
}

// GetItems returns footer items based on focused panel
func (c OverviewContext) GetItems() []FooterItem {
	switch c.FocusedPanel {
	case "header":
		return []FooterItem{
			{Key: "[Tab]", Action: "next"},
			{Key: "[/]", Action: "filter"},
			{Key: "[ESC | Ctrl+C]", Action: "quit"},
		}
	case "nodes":
		return []FooterItem{
			{Key: "[↑/↓]", Action: "navigate"},
			{Key: "[Enter]", Action: "detail"},
			{Key: "[Tab]", Action: "next"},
			{Key: "[/]", Action: "filter"},
			{Key: "[ESC | Ctrl+C]", Action: "quit"},
		}
	case "pods":
		return []FooterItem{
			{Key: "[↑/↓]", Action: "navigate"},
			{Key: "[Enter]", Action: "detail"},
			{Key: "[Tab]", Action: "next"},
			{Key: "[/]", Action: "filter"},
			{Key: "[ESC | Ctrl+C]", Action: "quit"},
		}
	default: // summary or unknown
		return []FooterItem{
			{Key: "[Tab]", Action: "next"},
			{Key: "[ESC | Ctrl+C]", Action: "quit"},
		}
	}
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
			{Key: "[ESC]", Action: "back"},
			{Key: "[Ctrl+C]", Action: "quit"},
		}
	default: // events
		return []FooterItem{
			{Key: "[↑/↓]", Action: "scroll"},
			{Key: "[Tab]", Action: "next"},
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
			{Key: "[ESC]", Action: "back"},
			{Key: "[Ctrl+C]", Action: "quit"},
		}
	default: // events, volumes
		return []FooterItem{
			{Key: "[↑/↓]", Action: "scroll"},
			{Key: "[Tab]", Action: "next"},
			{Key: "[n]", Action: "node"},
			{Key: "[ESC]", Action: "back"},
			{Key: "[Ctrl+C]", Action: "quit"},
		}
	}
}

// ContainerDetailContext provides footer items for Container Detail page
type ContainerDetailContext struct {
	FocusedPanel string // optional, container detail has simpler context
}

// GetItems returns footer items for container detail (static)
func (c ContainerDetailContext) GetItems() []FooterItem {
	return []FooterItem{
		{Key: "[↑/↓]", Action: "scroll"},
		{Key: "[Tab]", Action: "next"},
		{Key: "[l]", Action: "logs"},
		{Key: "[s]", Action: "spec"},
		{Key: "[ESC]", Action: "back"},
		{Key: "[Ctrl+C]", Action: "quit"},
	}
}
