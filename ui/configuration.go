package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
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
	showConfigurationOptions(app, pages, confDir, buildDir, flex)
	pages.AddPage("configuration_menu", flex, true, true)
}

func showConfigurationOptions(app *tview.Application, pages *tview.Pages, confDir string, buildDir string, flex *tview.Flex) {
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

	// Add selected func for editing files (after reload is defined)
	list.SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		// This will be called for both Enter and mouse clicks, so we need to distinguish
		// For now, let's disable this and handle Enter via input capture
	})

	// Helper function to check if a file is protected from editing
	isProtectedFile := func(fileName string) bool {
		protectedFiles := []string{
			"modules.default.conf",
			"snomask.default.conf",
			"operclass.default.conf",
			"rpc-class.default.conf",
			"rpc.modules.default.conf",
		}
		for _, protected := range protectedFiles {
			if fileName == protected {
				return true
			}
		}
		return false
	}

	// Handle Enter key specifically for editing files
	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter {
			mainText, _ := list.GetItemText(list.GetCurrentItem())
			if strings.HasPrefix(mainText, "📄") {
				fileName := strings.TrimPrefix(mainText, "📄 ")
				if isProtectedFile(fileName) {
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Cannot edit protected file: %s\n\nThis is a system default configuration file.", fileName)).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("edit_protected_error_modal")
						})
					pages.AddPage("edit_protected_error_modal", errorModal, true, true)
					return nil // Consume the event
				}
				filePath := filepath.Join(currentPath, fileName)
				showEditModal(app, pages, filePath, reload)
				return nil // Consume the event
			}
		}
		return event // Pass through other events
	})

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

		// Check if it's a protected file
		fileName := filepath.Base(currentSelectedPath)
		if isProtectedFile(fileName) {
			errorModal := tview.NewModal().
				SetText(fmt.Sprintf("Cannot edit protected file: %s\n\nThis is a system default configuration file.", fileName)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("edit_protected_error_modal")
				})
			pages.AddPage("edit_protected_error_modal", errorModal, true, true)
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
	testConfigBtn := tview.NewButton("Test Config").SetSelectedFunc(func() {
		// Run ./unrealircd configtest and capture output
		cmd := exec.Command("./unrealircd", "configtest")
		cmd.Dir = buildDir // Run from the build directory where unrealircd binary should be
		output, err := cmd.CombinedOutput()

		var resultText string
		if err != nil {
			resultText = fmt.Sprintf("Config test failed:\n\nError: %v\n\nOutput:\n%s", err, string(output))
		} else {
			resultText = fmt.Sprintf("Config test passed!\n\nOutput:\n%s", string(output))
		}

		// Show result in a modal
		resultModal := tview.NewModal().
			SetText(resultText).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("config_test_result_modal")
			})
		pages.AddPage("config_test_result_modal", resultModal, true, true)
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
	buttonBar.AddItem(tview.NewTextView().SetText(" "), 2, 0, false)
	buttonBar.AddItem(testConfigBtn, 0, 1, false)

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
				}
				// Files don't need special handling here - SetSelectedFunc handles editing
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

func highlightUnrealIRCdConfig(content string) string {
	// Simple syntax highlighting for UnrealIRCd config files
	// Use tview color tags for proper rendering

	lines := strings.Split(content, "\n")
	var result []string

	// Common UnrealIRCd configuration keywords
	keywords := []string{
		"loadmodule", "include", "rpc-user", "set", "oper", "password", "vhost",
		"me", "admin", "class", "allow", "listen", "link", "ulines", "ban",
		"except", "tld", "log", "alias", "channel", "badword", "spamfilter",
		"deny", "connect", "drpass", "restartpass", "diepass",
		"motd", "rules", "services", "cloak", "cgiirc", "webirc", "geoip",
		"blacklist", "dnsbl", "throttle", "antirandom", "antimixedutf8",
	}

	for _, line := range lines {
		originalLine := line
		highlighted := line

		// Color braces and parentheses first
		highlighted = strings.ReplaceAll(highlighted, "{", "[red]{[-]")
		highlighted = strings.ReplaceAll(highlighted, "}", "[red]}[-]")
		highlighted = strings.ReplaceAll(highlighted, "(", "[white]([-]")
		highlighted = strings.ReplaceAll(highlighted, ")", "[white])[-]")

		// Color strings first
		inString := false
		stringStart := -1
		quoteChar := byte(0)
		for i, char := range highlighted {
			if (char == '"' || char == '\'') && (i == 0 || highlighted[i-1] != '\\') {
				if !inString {
					inString = true
					stringStart = i
					quoteChar = byte(char)
				} else if byte(char) == quoteChar {
					inString = false
					// Color the string
					before := highlighted[:stringStart]
					stringPart := highlighted[stringStart : i+1]
					after := highlighted[i+1:]
					highlighted = before + "[blue]" + stringPart + "[-]" + after
					break // Only color first string per line for simplicity
				}
			}
		}

		// Color keywords - but skip those that are inside quotes in the original line
		// We need to process matches in order and track position
		for _, keyword := range keywords {
			// Use word boundaries to avoid partial matches
			pattern := `\b` + keyword + `\b`
			re := regexp.MustCompile(pattern)
			// Find all matches with their positions
			matches := re.FindAllStringIndex(originalLine, -1)
			// Process them in reverse order to avoid position shifting
			for i := len(matches) - 1; i >= 0; i-- {
				match := originalLine[matches[i][0]:matches[i][1]]
				matchStart := matches[i][0]

				// Count quotes before this match position
				quoteCount := 0
				for j := 0; j < matchStart; j++ {
					if (originalLine[j] == '"' || originalLine[j] == '\'') && (j == 0 || originalLine[j-1] != '\\') {
						quoteCount++
					}
				}

				// If odd number of quotes, we're inside a string, skip
				if quoteCount%2 == 1 {
					continue
				}

				// Replace this specific occurrence
				before := highlighted[:matches[i][0]]
				after := highlighted[matches[i][1]:]
				highlighted = before + "[green]" + match + "[-]" + after
			}
		}

		result = append(result, highlighted)
	}

	return strings.Join(result, "\n")
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

	// Check if this is a .conf file for syntax highlighting
	fileName := filepath.Base(path)
	if strings.HasSuffix(fileName, ".conf") {
		previewContent = highlightUnrealIRCdConfig(previewContent)
	}

	previewView.SetText(fmt.Sprintf("File: %s\n\n%s", fileName, previewContent))
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

func showEditorWithChoice(app *tview.Application, pages *tview.Pages, filePath string, reload func(), editor string) {
	// Create a temporary file for editing
	tempFile, err := os.CreateTemp("", "unrealircd-edit-*.tmp")
	if err != nil {
		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Error creating temporary file: %v", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("edit_temp_error_modal")
			})
		pages.AddPage("edit_temp_error_modal", errorModal, true, true)
		return
	}
	tempPath := tempFile.Name()
	tempFile.Close()

	// Copy original file to temp file
	originalContent, err := os.ReadFile(filePath)
	if err != nil {
		os.Remove(tempPath) // cleanup
		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Error reading file: %v", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("edit_read_error_modal")
			})
		pages.AddPage("edit_read_error_modal", errorModal, true, true)
		return
	}

	err = os.WriteFile(tempPath, originalContent, 0644)
	if err != nil {
		os.Remove(tempPath) // cleanup
		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Error writing temporary file: %v", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("edit_write_error_modal")
			})
		pages.AddPage("edit_write_error_modal", errorModal, true, true)
		return
	}

	// Show progress modal
	progressModal := tview.NewModal().
		SetText("Editor launched. Press OK when you've finished to apply the changes.").
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(int, string) {
			pages.RemovePage("edit_progress_modal")
			// Check if file was modified
			editedContent, err := os.ReadFile(tempPath)
			if err != nil {
				os.Remove(tempPath) // cleanup
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error reading edited file: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("edit_read_edited_error_modal")
					})
				pages.AddPage("edit_read_edited_error_modal", errorModal, true, true)
				return
			}

			// Check if content changed
			if string(editedContent) != string(originalContent) {
				// Save changes back to original file
				err = os.WriteFile(filePath, editedContent, 0644)
				if err != nil {
					os.Remove(tempPath) // cleanup
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Error saving changes: %v", err)).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("edit_save_error_modal")
						})
					pages.AddPage("edit_save_error_modal", errorModal, true, true)
					return
				}
				reload() // Refresh the file list
			}

			// Cleanup temp file
			os.Remove(tempPath)
		})

	pages.AddPage("edit_progress_modal", progressModal, true, true)

	// Suspend the TUI and launch the external editor
	app.Suspend(func() {
		cmd := exec.Command(editor, tempPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			// Editor failed to launch or exited with error
			app.QueueUpdateDraw(func() {
				pages.RemovePage("edit_progress_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Editor failed: %v\n\nYou can try setting the EDITOR environment variable.", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("edit_launch_error_modal")
						os.Remove(tempPath) // cleanup
					})
				pages.AddPage("edit_launch_error_modal", errorModal, true, true)
			})
		}
	})
}

func showEditModal(app *tview.Application, pages *tview.Pages, filePath string, reload func()) {
	// Get available editors
	availableEditors := getAvailableEditors()

	// If only one editor is available, use it directly
	if len(availableEditors) == 1 {
		showEditorWithChoice(app, pages, filePath, reload, availableEditors[0])
		return
	}

	// Show editor selection modal
	var buttons []string
	for _, editor := range availableEditors {
		buttons = append(buttons, editor)
	}
	buttons = append(buttons, "Cancel")

	editorModal := tview.NewModal().
		SetText("Multiple editors found. Choose one:").
		AddButtons(buttons).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			pages.RemovePage("editor_selection_modal")
			if buttonLabel != "Cancel" && buttonIndex < len(availableEditors) {
				showEditorWithChoice(app, pages, filePath, reload, buttonLabel)
			}
		})

	pages.AddPage("editor_selection_modal", editorModal, true, true)
}

// getAvailableEditors returns a list of available editors
func getAvailableEditors() []string {
	var available []string

	// Check EDITOR environment variable first
	if editor := os.Getenv("EDITOR"); editor != "" {
		if _, err := exec.LookPath(editor); err == nil {
			available = append(available, editor)
		}
	}

	// Try common editors
	editors := []string{"nano", "micro", "vim", "vi", "emacs", "gedit", "kate", "code"}

	for _, editor := range editors {
		if _, err := exec.LookPath(editor); err == nil {
			// Avoid duplicates
			found := false
			for _, avail := range available {
				if avail == editor {
					found = true
					break
				}
			}
			if !found {
				available = append(available, editor)
			}
		}
	}

	// Fallback to vi if nothing else is available
	if len(available) == 0 {
		available = append(available, "vi")
	}

	return available
}
