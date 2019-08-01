package ui

import (
	"github.com/rivo/tview"
)

type Panel interface {
	Layout()
	Clear()
	DrawHeader(...string)
	DrawBody(data interface{})
	DrawFooter(...string)
	GetTitle() string
	GetView() tview.Primitive
}
