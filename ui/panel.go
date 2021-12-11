package ui

import (
	"context"

	"github.com/rivo/tview"
)

type Panel interface {
	Layout(data interface{})
	DrawHeader(data interface{})
	// TODO add context to DrawXXX methods
	DrawBody(data interface{})
	DrawFooter(param interface{})
	Clear()
	GetTitle() string
	GetRootView() tview.Primitive
	GetChildrenViews() []tview.Primitive
}

type PanelController interface {
	Panel
	Run(context.Context) error
}
