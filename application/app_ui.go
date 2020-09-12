package application

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

var (
	buttonUnselectedBgColor = tcell.ColorPaleGreen
	buttonUnselectedFgColor = tcell.ColorDarkBlue
	buttonSelectedBgColor   = tcell.ColorBlue
	buttonSelectedFgColor   = tcell.ColorWhite
)

type appPanel struct {
	tviewApp *tview.Application
	title    string
	header   *tview.TextView
	pages    *tview.Pages
	footer   *tview.Table
}

func newPanel(app *tview.Application) *appPanel {
	p := &appPanel{title: "ktop", tviewApp: app}
	return p
}

func (p *appPanel) GetTitle() string {
	return p.title
}

func (p *appPanel) Layout(data interface{}) {
	p.header = tview.NewTextView().SetDynamicColors(true)
	p.header.SetBorder(true)
	p.pages = tview.NewPages()
	p.footer = tview.NewTable()
	p.footer.SetBorder(true)

	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.header, 3, 1, false). // header
		AddItem(p.pages, 0, 1, true).   // body
		AddItem(p.footer, 3, 1, false) // footer

	p.tviewApp.SetRoot(content, true)

	// add pages
	pages, ok := data.([]ApplicationPanel)
	if !ok{
		panic(fmt.Sprintf("application.Layout got unexpected data type: %T", data))
	}

	// setup page and page buttons in footer
	for i, page := range pages {
		p.pages.AddPage(page.Title, page.View, true, false)
		p.footer.SetCell(0, i,
			&tview.TableCell{
				Text:            fmt.Sprintf("  %s (F%d)  ", page.Title, i+1),
				Color:           buttonUnselectedFgColor,
				Align:           tview.AlignCenter,
				BackgroundColor: buttonUnselectedBgColor,
				Expansion:       0,
			},
		)
	}
}

func (p *appPanel) DrawHeader(data interface{}) {
	header, ok := data.(string)
	if !ok{
		panic(fmt.Sprintf("application.Drawheader got unexpected type %T", data))
	}

	fmt.Fprintf(p.header, header)
}

func (p *appPanel) DrawBody(data interface{}) {}

func (p *appPanel) DrawFooter(data interface{}) {
	title, ok := data.(string)
	if !ok{
		panic(fmt.Sprintf("application.DrawBody got unexpected data type: %T", data))
	}
	p.switchToPage(title)
}

func (p *appPanel) Clear() {

}

func (p *appPanel) GetView() tview.Primitive {
	return p.pages
}

func (p *appPanel) switchToPage(title string) {

	row := 0
	cols := p.footer.GetColumnCount()

	for i := 0; i < cols; i++ {
		cell := p.footer.GetCell(row, i)
		if strings.HasPrefix(strings.TrimSpace(cell.Text),title) {
			cell.SetTextColor(buttonSelectedFgColor)
			cell.SetBackgroundColor(buttonSelectedBgColor)
		} else {
			cell.SetTextColor(buttonUnselectedFgColor)
			cell.SetBackgroundColor(buttonUnselectedBgColor)
		}
	}
	p.pages.SwitchToPage(title)
}
