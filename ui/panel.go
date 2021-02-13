package ui

import (
	"github.com/rivo/tview"
)

type Panel interface {
	Layout(data interface{})
	DrawHeader(data interface{})
	DrawBody(data interface{})
	DrawFooter(param interface{})
	Clear()
	GetTitle() string
	GetRootView() tview.Primitive
	GetChildrenViews() []tview.Primitive
}

type PanelController interface {
	Panel
	Run() error
}
