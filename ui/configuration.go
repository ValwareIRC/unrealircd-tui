package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func ConfigurationMenuPage(app *tview.Application, pages *tview.Pages, buildDir string) {
	confDir := filepath.Join(buildDir, "conf")

	// Check if conf directory exists
	if _, err := os.Stat(confDir); os.IsNotExist(err) {
		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Configuration directory does not exist: %s", confDir)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("config_error_modal")
			})
		pages.AddPage("config_error_modal", errorModal, true, true)
		return
	}

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	showConfigurationOptions(app, pages, confDir, flex)
	pages.AddPage("configuration_menu", flex, true, true)
}

func showConfigurationOptions(app *tview.Application, pages *tview.Pages, confDir string, flex *tview.Flex) {
	currentPath := confDir

	// Left: File/Folder list
	list := tview.NewList()
	list.SetBorder(true)
	list.SetTitle("Configuration Files")
	list.SetBorderColor(tcell.ColorGreen)

	// Right: File preview
	previewView := tview.NewTextView()
	previewView.SetBorder(true)
	previewView.SetTitle("File Preview")
	previewView.SetDynamicColors(true)
	previewView.SetWordWrap(true)
	previewView.SetScrollable(true)

	// Track current selected path
	currentSelectedPath := ""

	// Function to preview based on selected item
	previewSelected := func(mainText string) {
		if strings.HasPrefix(mainText, "📁") {
			if mainText == "📁 ../" {
				previewView.SetText("Select a file or folder to preview")
				currentSelectedPath = ""
			} else {
				folderName := strings.TrimPrefix(mainText, "📁 ")
				folderName = strings.TrimSuffix(folderName, "/")
				folderPath := filepath.Join(currentPath, folderName)
				previewFolder(previewView, folderPath)
				currentSelectedPath = folderPath
			}
		} else if strings.HasPrefix(mainText, "📄") {
			fileName := strings.TrimPrefix(mainText, "📄 ")
			filePath := filepath.Join(currentPath, fileName)
			previewFile(previewView, filePath)
			currentSelectedPath = filePath
		}
	}

	// Add changed func for preview
	list.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		previewSelected(mainText)
	})

	// Function to reload the list and preview
	var reload func()
	reload = func() {
		relPath, _ := filepath.Rel(confDir, currentPath)
		if relPath == "." {
			relPath = ""
		}
		title := "Configuration Files"
		if relPath != "" {
			title = fmt.Sprintf("Configuration Files - %s", relPath)
		}
		list.SetTitle(title)
		list.Clear()
		loadConfigurationList(list, previewView, currentPath, confDir, func(newPath string) {
			currentPath = newPath
			reload()
		})

		// Set initial preview for the first item if any
		if list.GetItemCount() > 0 {
			mainText, _ := list.GetItemText(0)
			previewSelected(mainText)
		} else {
			previewView.SetText("Select a file or folder to preview")
			currentSelectedPath = ""
		}
	}

	// Initial load
	reload()

	// Buttons
	editBtn := tview.NewButton("Edit").SetSelectedFunc(func() {
		if currentSelectedPath == "" {
			errorModal := tview.NewModal().
				SetText("No file selected to edit.").
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("edit_error_modal")
				})
			pages.AddPage("edit_error_modal", errorModal, true, true)
			return
		}

		// Check if it's a file
		if info, err := os.Stat(currentSelectedPath); err != nil || info.IsDir() {
			errorModal := tview.NewModal().
				SetText("Can only edit files, not directories.").
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("edit_error_modal")
				})
			pages.AddPage("edit_error_modal", errorModal, true, true)
			return
		}

		showEditModal(app, pages, currentSelectedPath, reload)
	})
	deleteBtn := tview.NewButton("Delete").SetSelectedFunc(func() {
		if currentSelectedPath == "" {
			errorModal := tview.NewModal().
				SetText("No item selected to delete.").
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("delete_error_modal")
				})
			pages.AddPage("delete_error_modal", errorModal, true, true)
			return
		}

		// Check if it's a .default.conf file
		if strings.HasSuffix(currentSelectedPath, ".default.conf") {
			errorModal := tview.NewModal().
				SetText("Cannot delete .default.conf files. These are protected system files.").
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("delete_protected_modal")
				})
			pages.AddPage("delete_protected_modal", errorModal, true, true)
			return
		}

		// Show confirmation
		confirmModal := tview.NewModal().
			SetText(fmt.Sprintf("Are you sure you want to delete '%s'?", filepath.Base(currentSelectedPath))).
			AddButtons([]string{"Yes", "No"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				pages.RemovePage("delete_confirm_modal")
				if buttonLabel == "Yes" {
					var err error
					if info, statErr := os.Stat(currentSelectedPath); statErr == nil && info.IsDir() {
						err = os.RemoveAll(currentSelectedPath)
					} else {
						err = os.Remove(currentSelectedPath)
					}

					if err != nil {
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Error deleting item: %v", err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("delete_error_modal")
							})
						pages.AddPage("delete_error_modal", errorModal, true, true)
					} else {
						reload()
					}
				}
			})
		pages.AddPage("delete_confirm_modal", confirmModal, true, true)
	})
	newFileBtn := tview.NewButton("New File").SetSelectedFunc(func() {
		showNewItemModal(app, pages, currentPath, true, reload)
	})
	newFolderBtn := tview.NewButton("New Folder").SetSelectedFunc(func() {
		showNewItemModal(app, pages, currentPath, false, reload)
	})
	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.RemovePage("configuration_menu")
	})

	buttonBar := tview.NewFlex()
	buttonBar.AddItem(backBtn, 0, 1, false)
	buttonBar.AddItem(tview.NewTextView().SetText(" "), 2, 0, false)
	buttonBar.AddItem(editBtn, 0, 1, false)
	buttonBar.AddItem(tview.NewTextView().SetText(" "), 2, 0, false)
	buttonBar.AddItem(deleteBtn, 0, 1, false)
	buttonBar.AddItem(tview.NewTextView().SetText(" "), 2, 0, false)
	buttonBar.AddItem(newFileBtn, 0, 1, false)
	buttonBar.AddItem(tview.NewTextView().SetText(" "), 2, 0, false)
	buttonBar.AddItem(newFolderBtn, 0, 1, false)

	// Layout
	contentFlex := tview.NewFlex()
	contentFlex.AddItem(list, 40, 0, true)
	contentFlex.AddItem(previewView, 0, 1, false)

	flex.AddItem(createHeader(), 3, 0, false)
	flex.AddItem(contentFlex, 0, 1, true)
	flex.AddItem(buttonBar, 3, 0, false)
}

func loadConfigurationList(list *tview.List, previewView *tview.TextView, currentPath, rootPath string, navigate func(string)) {
	entries, err := os.ReadDir(currentPath)
	if err != nil {
		previewView.SetText(fmt.Sprintf("Error reading directory: %v", err))
		return
	}

	// If not at root, add ".." entry
	if currentPath != rootPath {
		list.AddItem("📁 ../", "  Go back", 0, func() {
			parent := filepath.Dir(currentPath)
			navigate(parent)
		})
	}

	// Sort entries: folders first, then files
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	for _, entry := range entries {
		name := entry.Name()
		var displayName string
		if entry.IsDir() {
			displayName = fmt.Sprintf("📁 %s/", name)
		} else {
			displayName = fmt.Sprintf("📄 %s", name)
		}

		entryPath := filepath.Join(currentPath, name)
		isDir := entry.IsDir()

		list.AddItem(displayName, "", 0, func(ep string, id bool) func() {
			return func() {
				if id {
					navigate(ep)
				} else {
					previewFile(previewView, ep)
				}
			}
		}(entryPath, isDir))
	}
}

func previewFolder(previewView *tview.TextView, path string) {
	entries, err := os.ReadDir(path)
	if err != nil {
		previewView.SetText(fmt.Sprintf("Error reading directory: %v", err))
		return
	}

	var content strings.Builder
	content.WriteString(fmt.Sprintf("Directory: %s\n\nContents:\n", filepath.Base(path)))

	for _, entry := range entries {
		if entry.IsDir() {
			content.WriteString(fmt.Sprintf("📁 %s/\n", entry.Name()))
		} else {
			content.WriteString(fmt.Sprintf("📄 %s\n", entry.Name()))
		}
	}

	previewView.SetText(content.String())
}

func previewFile(previewView *tview.TextView, path string) {
	// For files, show content
	content, err := os.ReadFile(path)
	if err != nil {
		previewView.SetText(fmt.Sprintf("Error reading file: %v", err))
		return
	}

	// Limit preview to first 10000 characters to avoid huge files
	previewContent := string(content)
	if len(previewContent) > 10000 {
		previewContent = previewContent[:10000] + "\n\n[File truncated for preview]"
	}

	previewView.SetText(fmt.Sprintf("File: %s\n\n%s", filepath.Base(path), previewContent))
}

func showNewItemModal(app *tview.Application, pages *tview.Pages, currentPath string, isFile bool, reload func()) {
	form := tview.NewForm()
	form.SetBorder(true)
	if isFile {
		form.SetTitle("Create New File")
	} else {
		form.SetTitle("Create New Folder")
	}
	form.SetBackgroundColor(tcell.ColorDefault)

	form.AddInputField("Name:", "", 30, nil, nil)

	form.AddButton("Create", func() {
		name := form.GetFormItem(0).(*tview.InputField).GetText()
		if name == "" {
			errorModal := tview.NewModal().
				SetText("Name cannot be empty.").
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("new_item_error_modal")
				})
			pages.AddPage("new_item_error_modal", errorModal, true, true)
			return
		}

		fullPath := filepath.Join(currentPath, name)
		var err error
		if isFile {
			var file *os.File
			file, err = os.Create(fullPath)
			if err == nil {
				file.Close()
			}
		} else {
			err = os.Mkdir(fullPath, 0755)
		}

		if err != nil {
			errorModal := tview.NewModal().
				SetText(fmt.Sprintf("Error creating item: %v", err)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("new_item_error_modal")
				})
			pages.AddPage("new_item_error_modal", errorModal, true, true)
		} else {
			reload()
			pages.RemovePage("new_item_modal")
		}
	})

	form.AddButton("Cancel", func() {
		pages.RemovePage("new_item_modal")
	})

	pages.AddPage("new_item_modal", form, true, true)
}

func showEditModal(app *tview.Application, pages *tview.Pages, filePath string, reload func()) {
	// Load file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Error reading file: %v", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("edit_read_error_modal")
			})
		pages.AddPage("edit_read_error_modal", errorModal, true, true)
		return
	}

	// Create text area
	textArea := tview.NewTextArea()
	textArea.SetBorder(true)
	textArea.SetTitle(fmt.Sprintf("Editing: %s", filepath.Base(filePath)))
	textArea.SetText(string(content), false)
	textArea.SetWordWrap(false)

	// Input field for find/goto
	inputField := tview.NewInputField()
	inputField.SetBorder(true)
	inputField.SetTitle("Command Input")
	inputField.SetFieldBackgroundColor(tcell.ColorWhite)
	inputField.SetFieldTextColor(tcell.ColorBlack)
	inputField.SetBorderColor(tcell.ColorYellow)

	// Add shortcuts banner
	shortcutsView := tview.NewTextView()
	shortcutsView.SetTextAlign(tview.AlignCenter)
	shortcutsView.SetTextColor(tcell.ColorYellow)
	shortcutsView.SetBorder(true)
	shortcutsView.SetBorderColor(tcell.ColorBlue)

	saveBtn := tview.NewButton("Save").SetSelectedFunc(func() {
		newContent := textArea.GetText()
		err := os.WriteFile(filePath, []byte(newContent), 0644)
		if err != nil {
			errorModal := tview.NewModal().
				SetText(fmt.Sprintf("Error saving file: %v", err)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("edit_save_error_modal")
				})
			pages.AddPage("edit_save_error_modal", errorModal, true, true)
		} else {
			reload()
			pages.RemovePage("edit_modal")
		}
	})
	cancelBtn := tview.NewButton("Cancel").SetSelectedFunc(func() {
		pages.RemovePage("edit_modal")
	})

	buttonBar := tview.NewFlex()
	buttonBar.AddItem(saveBtn, 0, 1, false)
	buttonBar.AddItem(tview.NewTextView().SetText(" "), 2, 0, false)
	buttonBar.AddItem(cancelBtn, 0, 1, false)

	// Mode: "normal", "find", "goto"
	mode := "normal"

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	flex.AddItem(createHeader(), 3, 0, false)
	flex.AddItem(textArea, 0, 1, true)
	flex.AddItem(buttonBar, 3, 0, false)
	flex.AddItem(inputField, 0, 0, false) // Initially hidden with height 0
	flex.AddItem(shortcutsView, 3, 0, false)

	// Function to show input field
	showInputField := func() {
		// Change input field height to 3 to make it visible
		flex.RemoveItem(inputField)
		flex.RemoveItem(shortcutsView)
		flex.AddItem(inputField, 3, 0, false)
		flex.AddItem(shortcutsView, 3, 0, false)
	}

	// Function to hide input field
	hideInputField := func() {
		// Change input field height to 0 to hide it
		flex.RemoveItem(inputField)
		flex.RemoveItem(shortcutsView)
		flex.AddItem(inputField, 0, 0, false)
		flex.AddItem(shortcutsView, 3, 0, false)
	}

	// Function to scroll TextArea to a specific line
	scrollToLine := func(textArea *tview.TextArea, line int) {
		// Use reflection to access the private scrollTo method
		val := reflect.ValueOf(textArea)
		method := val.MethodByName("scrollTo")
		if method.IsValid() {
			// Call scrollTo(line, true) to center the line
			args := []reflect.Value{reflect.ValueOf(line), reflect.ValueOf(true)}
			method.Call(args)
		}
	}

	// Function to update the banner
	updateBanner := func() {
		switch mode {
		case "normal":
			shortcutsView.SetText("Ctrl+S: Save | Ctrl+X: Cancel | Ctrl+Q: Quit | Ctrl+F: Find | Ctrl+G: Go to Line")
		case "find":
			shortcutsView.SetText("Find: (Enter to search, Esc to cancel)")
		case "goto":
			shortcutsView.SetText("Go to line: (Enter to go, Esc to cancel)")
		}
	}

	// Keyboard shortcuts for textArea
	textArea.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if mode == "normal" {
			if event.Key() == tcell.KeyCtrlS {
				// Save
				newContent := textArea.GetText()
				err := os.WriteFile(filePath, []byte(newContent), 0644)
				if err != nil {
					// Could show error
				} else {
					reload()
					pages.RemovePage("edit_modal")
				}
				return nil
			}
			if event.Key() == tcell.KeyCtrlX || event.Key() == tcell.KeyCtrlQ {
				// Cancel
				pages.RemovePage("edit_modal")
				return nil
			}
			if event.Key() == tcell.KeyCtrlF {
				// Find
				mode = "find"
				inputField.SetLabel("Find: ")
				inputField.SetText("")
				updateBanner()
				showInputField()
				app.SetFocus(inputField)
				return nil
			}
			if event.Key() == tcell.KeyCtrlG {
				// Go to line
				mode = "goto"
				inputField.SetLabel("Go to line: ")
				inputField.SetText("")
				updateBanner()
				showInputField()
				app.SetFocus(inputField)
				return nil
			}
		}
		return event
	})

	// Input field capture
	inputField.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter {
			if mode == "find" {
				searchText := inputField.GetText()
				if searchText == "" {
					mode = "normal"
					updateBanner()
					app.SetFocus(textArea)
					return nil
				}

				content := textArea.GetText()
				cursorRow, cursorCol, _, _ := textArea.GetCursor()

				// Find from current position
				lines := strings.Split(content, "\n")
				for i := cursorRow; i < len(lines); i++ {
					line := lines[i]
					if i == cursorRow {
						idx := strings.Index(line[cursorCol:], searchText)
						if idx >= 0 {
							scrollToLine(textArea, i)
							mode = "normal"
							updateBanner()
							hideInputField()
							app.SetFocus(textArea)
							return nil
						}
					} else {
						idx := strings.Index(line, searchText)
						if idx >= 0 {
							scrollToLine(textArea, i)
							mode = "normal"
							updateBanner()
							hideInputField()
							app.SetFocus(textArea)
							return nil
						}
					}
				}

				// Wrap around from beginning
				for i := 0; i <= cursorRow; i++ {
					line := lines[i]
					idx := strings.Index(line, searchText)
					if idx >= 0 {
						scrollToLine(textArea, i)
						mode = "normal"
						updateBanner()
						hideInputField()
						app.SetFocus(textArea)
						return nil
					}
				}

				// Not found
				notFoundModal := tview.NewModal().
					SetText("Text not found.").
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("find_not_found_modal")
					})
				pages.AddPage("find_not_found_modal", notFoundModal, true, true)
			} else if mode == "goto" {
				lineStr := inputField.GetText()
				lineNum, err := strconv.Atoi(lineStr)
				if err != nil || lineNum < 1 {
					errorModal := tview.NewModal().
						SetText("Invalid line number.").
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("goto_error_modal")
						})
					pages.AddPage("goto_error_modal", errorModal, true, true)
				} else {
					content := textArea.GetText()
					lines := strings.Split(content, "\n")
					if lineNum > len(lines) {
						lineNum = len(lines)
					}
					scrollToLine(textArea, lineNum-1) // 0-based
				}
				mode = "normal"
				updateBanner()
				hideInputField()
				app.SetFocus(textArea)
			}
			return nil
		}
		if event.Key() == tcell.KeyEsc {
			mode = "normal"
			updateBanner()
			hideInputField()
			app.SetFocus(textArea)
			return nil
		}
		return event
	})

	// Initially normal
	updateBanner()

	pages.AddPage("edit_modal", flex, true, true)
	app.SetFocus(textArea)
}
