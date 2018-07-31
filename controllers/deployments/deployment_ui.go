package deployments

import (
	"fmt"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

type deploymentPage struct {
	root   *tview.Flex
	header *tview.TextView

	depListFormat string
	depListCols   []string
	depList       *tview.Table
}

func newPage() *deploymentPage {
	p := &deploymentPage{
		depListCols: []string{"NAME", "STATUS", "IP", "NODE", "CPU", "MEMORY"},
	}
	p.layout()
	return p
}

func (p *deploymentPage) layout() {
	p.header = tview.NewTextView().
		SetDynamicColors(true)
	p.header.SetBorder(true)
	fmt.Fprint(p.header, "[green]loading...")

	p.depList = tview.NewTable()
	p.depList.SetBorder(true)
	p.depList.SetBorders(false)
	p.depList.SetTitle(" Deployments ")
	p.depList.SetTitleAlign(tview.AlignLeft)
	p.depList.SetBorderColor(tcell.ColorWhite)

	page := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.header, 3, 1, true).
		AddItem(p.depList, 0, 1, true)

	p.root = page
}
