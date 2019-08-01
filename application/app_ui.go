package application

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

var (
	PageNames = []string{
		"Overview",
		"Deployments",
	}

	buttonUnselectedBgColor = tcell.ColorPaleGreen
	buttonUnselectedFgColor = tcell.ColorDarkBlue
	buttonSelectedBgColor   = tcell.ColorBlue
	buttonSelectedFgColor   = tcell.ColorWhite
)

type appPanel struct {
	tviewApp   *tview.Application
	title      string
	header     *tview.TextView
	pages      *tview.Pages
	pageTitles []string
	buttons    *tview.Table
}

func newAppPanel(app *tview.Application) *appPanel {
	p := &appPanel{title: "ktop", tviewApp: app}
	p.Layout()
	return p
}

func (p *appPanel) GetTitle() string {
	return p.title
}

func (p *appPanel) Layout() {
	p.header = tview.NewTextView().SetDynamicColors(true)
	p.header.SetBorder(true)
	p.pages = tview.NewPages()
	p.DrawFooter(PageNames...)

	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.header, 3, 1, false). // header
		AddItem(p.pages, 0, 1, true).   // body
		AddItem(p.buttons, 3, 1, false) // footer

	p.tviewApp.SetRoot(content, true)
}

func (p *appPanel) DrawHeader(headers ...string) {
	title := strings.Join(headers, " ")
	fmt.Fprintf(p.header, title)
}

func (p *appPanel) DrawBody(data interface{}) {
	p.tviewApp.Draw()
}

func (p *appPanel) DrawFooter(buttonNames ...string) {
	buttons := tview.NewTable()
	buttons.SetBorder(true)

	for i := 0; i < len(buttonNames); i++ {
		buttons.SetCell(0, i,
			&tview.TableCell{
				Text:            fmt.Sprintf("  %s (F%d)  ", PageNames[i], i+1),
				Color:           buttonUnselectedFgColor,
				Align:           tview.AlignCenter,
				BackgroundColor: buttonUnselectedBgColor,
				Expansion:       0,
			},
		)
	}

	p.buttons = buttons
}

func (p *appPanel) Clear() {

}

func (p *appPanel) GetView() tview.Primitive {
	return p.pages
}

func (p *appPanel) addPage(pageTitle string, page tview.Primitive) {
	if p.pages == nil {
		panic("Application panel UI missing pages")
	}
	p.pageTitles = append(p.pageTitles, pageTitle)
	p.pages.AddPage(pageTitle, page, true, false)
}

func (p *appPanel) switchToPage(pgIdx int) {
	if !p.pages.HasPage(PageNames[pgIdx]) {
		panic(fmt.Sprintf("Screen page %s not found", PageNames[pgIdx]))
	}

	row := 0
	cols := p.buttons.GetColumnCount()

	for i := 0; i < cols; i++ {
		cell := p.buttons.GetCell(row, i)
		if i == pgIdx {
			cell.SetTextColor(buttonSelectedFgColor)
			cell.SetBackgroundColor(buttonSelectedBgColor)
		} else {
			cell.SetTextColor(buttonUnselectedFgColor)
			cell.SetBackgroundColor(buttonUnselectedBgColor)
		}
	}

	p.pages.SwitchToPage(PageNames[pgIdx])
	p.tviewApp.Draw()
}
