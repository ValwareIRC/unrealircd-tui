package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// FocusableTextView wraps tview.TextView to make it focusable
type FocusableTextView struct {
	*tview.TextView
}

func (f *FocusableTextView) Focus(delegate func(p tview.Primitive)) {
	f.TextView.Focus(delegate)
}

func (f *FocusableTextView) HasFocus() bool {
	return f.TextView.HasFocus()
}

func (f *FocusableTextView) Blur() {
	f.TextView.Blur()
}

// createHeader creates the application header
func createHeader() *tview.TextView {
	header := tview.NewTextView()
	header.SetText("UnrealIRCd Terminal Manager by Valware")
	header.SetTextAlign(tview.AlignCenter)
	header.SetTextColor(tcell.ColorYellow)
	header.SetBorder(true)
	header.SetBorderColor(tcell.ColorBlue)
	return header
}

// createButtonBar creates a horizontal flex container for buttons
func createButtonBar(buttons ...*tview.Button) *tview.Flex {
	flex := tview.NewFlex()
	for i, btn := range buttons {
		flex.AddItem(btn, 0, 1, false)
		if i < len(buttons)-1 {
			// Add spacing
			flex.AddItem(tview.NewTextView().SetText(" "), 2, 0, false)
		}
	}
	return flex
}