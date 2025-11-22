package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rivo/tview"
)

// getLoadedModules returns a map of modules that are loaded in config files
func getLoadedModules(buildDir string) (map[string]bool, error) {
	loaded := make(map[string]bool)
	confDir := filepath.Join(buildDir, "conf")
	re := regexp.MustCompile(`loadmodule\s+(.+?);`)
	err := filepath.Walk(confDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !strings.HasSuffix(path, ".conf") || filepath.Base(path) == "modules.default.conf" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		matches := re.FindAllStringSubmatch(string(content), -1)
		for _, match := range matches {
			if len(match) > 1 {
				mod := strings.Trim(match[1], "\"")
				loaded[mod] = true
			}
		}
		return nil
	})
	return loaded, err
}

// getInstalledModules returns a map of modules that have .so files installed
func getInstalledModules(buildDir string) (map[string]bool, error) {
	installed := make(map[string]bool)
	modulesDir := filepath.Join(buildDir, "modules")
	err := filepath.Walk(modulesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, ".so") {
			rel, err := filepath.Rel(modulesDir, path)
			if err != nil {
				return err
			}
			mod := strings.TrimSuffix(rel, ".so")
			installed[mod] = true
		}
		return nil
	})
	return installed, err
}

// getDefaultModules returns a map of modules that are in modules.default.conf
func getDefaultModules(buildDir string) (map[string]bool, error) {
	defaultMods := make(map[string]bool)
	defaultFile := filepath.Join(buildDir, "conf", "modules.default.conf")
	content, err := os.ReadFile(defaultFile)
	if err != nil {
		return defaultMods, err
	}
	re := regexp.MustCompile(`loadmodule\s+(.+?);`)
	matches := re.FindAllStringSubmatch(string(content), -1)
	for _, match := range matches {
		if len(match) > 1 {
			mod := strings.Trim(match[1], "\"")
			defaultMods[mod] = true
		}
	}
	return defaultMods, nil
}

// removeLoadmodule removes loadmodule entries for a module from config files
func removeLoadmodule(mod string, buildDir string) error {
	confDir := filepath.Join(buildDir, "conf")
	re := regexp.MustCompile(`(?m)^.*loadmodule\s+` + regexp.QuoteMeta(mod) + `\s*;.*$\n?`)
	return filepath.Walk(confDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !strings.HasSuffix(path, ".conf") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		newContent := re.ReplaceAllString(string(content), "")
		if newContent != string(content) {
			return os.WriteFile(path, []byte(newContent), 0644)
		}
		return nil
	})
}

// removeSo removes the .so file for a module
func removeSo(mod string, buildDir string) error {
	soPath := filepath.Join(buildDir, "modules", mod+".so")
	return os.Remove(soPath)
}

// removeFromSource removes the source file for a third-party module
func removeFromSource(mod string, sourceDir string) error {
	parts := strings.Split(mod, "/")
	if len(parts) == 2 && parts[0] == "third" {
		srcPath := filepath.Join(sourceDir, "src", "modules", parts[0], parts[1]+".c")
		os.Remove(srcPath) // ignore error
	}
	return nil
}

// addLoadmodule adds a loadmodule entry to mods.conf
func addLoadmodule(mod string, buildDir string) error {
	modsConf := filepath.Join(buildDir, "conf", "mods.conf")
	content, err := os.ReadFile(modsConf)
	if err != nil {
		return err
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.Contains(line, "loadmodule "+mod+";") {
			return nil // already there
		}
	}
	newContent := string(content) + "\nloadmodule \"" + mod + "\";\n"
	return os.WriteFile(modsConf, []byte(newContent), 0644)
}

// rehashPrompt shows a modal asking if the user wants to rehash the server
func rehashPrompt(app *tview.Application, pages *tview.Pages, buildDir string) {
	rehashModal := tview.NewModal().
		SetText("Operation completed. Rehash the server? (./unrealircd rehash)").
		AddButtons([]string{"No", "Yes"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Yes" {
				go func() {
					cmd := exec.Command("./unrealircd", "rehash")
					cmd.Dir = buildDir
					err := cmd.Run()
					app.QueueUpdateDraw(func() {
						if err != nil {
							errorModal := tview.NewModal().
								SetText(fmt.Sprintf("Rehash failed: %v", err)).
								AddButtons([]string{"OK"}).
								SetDoneFunc(func(int, string) {
									pages.RemovePage("error_modal")
								})
							pages.AddPage("error_modal", errorModal, true, true)
						} else {
							successModal := tview.NewModal().
								SetText("Server rehashed successfully!").
								AddButtons([]string{"OK"}).
								SetDoneFunc(func(int, string) {
									pages.RemovePage("success_modal")
								})
							pages.AddPage("success_modal", successModal, true, true)
						}
					})
				}()
			}
			pages.RemovePage("rehash_modal")
		})
	pages.AddPage("rehash_modal", rehashModal, true, true)
}

// UninstallObbyScript uninstalls the ObbyScript module and scripts
func UninstallObbyScript(app *tview.Application, pages *tview.Pages, buildDir, sourceDir string) {
	// First confirmation modal
	firstConfirmModal := tview.NewModal().
		SetText("Are you sure you want to uninstall ObbyScript?\n\nThis will remove all scripts and configurations.").
		AddButtons([]string{"No", "Yes"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			pages.RemovePage("first_uninstall_confirm")
			if buttonLabel == "Yes" {
				// Second confirmation modal
				secondConfirmModal := tview.NewModal().
					SetText("Are you REALLY REALLY sure??!?!?\n\nThis action cannot be undone!\nAll your scripts will be permanently deleted!").
					AddButtons([]string{"No", "Yes"}).
					SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						pages.RemovePage("second_uninstall_confirm")
						if buttonLabel == "Yes" {
							// Proceed with uninstallation
							mod := "third/obbyscript"
							removeLoadmodule(mod, buildDir)
							removeSo(mod, buildDir)
							removeFromSource(mod, sourceDir)
							scriptsDir := filepath.Join(buildDir, "scripts")
							os.RemoveAll(scriptsDir)
							rehashPrompt(app, pages, buildDir)
						}
					})
				pages.AddPage("second_uninstall_confirm", secondConfirmModal, true, true)
			}
		})
	pages.AddPage("first_uninstall_confirm", firstConfirmModal, true, true)
}

// CheckModulesPage creates the check modules page
func CheckModulesPage(app *tview.Application, pages *tview.Pages, buildDir, sourceDir string) {
	loaded, err := getLoadedModules(buildDir)
	if err != nil {
		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Error scanning loaded modules: %v", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("error_modal")
			})
		pages.AddPage("error_modal", errorModal, true, true)
		return
	}

	installed, err := getInstalledModules(buildDir)
	if err != nil {
		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Error scanning installed modules: %v", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("error_modal")
			})
		pages.AddPage("error_modal", errorModal, true, true)
		return
	}

	defaultMods, err := getDefaultModules(buildDir)
	if err != nil {
		// Ignore error if default file doesn't exist
		defaultMods = make(map[string]bool)
	}

	allMods := make(map[string]bool)
	for mod := range loaded {
		allMods[mod] = true
	}
	for mod := range installed {
		allMods[mod] = true
	}

	list := tview.NewList()
	list.SetBorder(true).SetTitle("Module Status")
	for mod := range allMods {
		if defaultMods[mod] {
			continue
		}
		inst := "No"
		if installed[mod] {
			inst = "Yes"
		}
		load := "No"
		if loaded[mod] {
			load = "Yes"
		}
		list.AddItem(mod, fmt.Sprintf("Installed: %s | Loaded: %s", inst, load), 0, nil)
	}
	// Note: currentList is a global variable that needs to be handled

	textView := &FocusableTextView{tview.NewTextView()}
	textView.SetBorder(true).SetTitle("Module Details")
	textView.SetDynamicColors(true)
	textView.SetWordWrap(true)
	textView.SetScrollable(true)
	textView.SetText("Select a module to view its status details.")

	loadUnloadBtn := tview.NewButton("Load/Unload Module").SetSelectedFunc(func() {
		index := list.GetCurrentItem()
		if index < 0 {
			return
		}
		mod, _ := list.GetItemText(index)
		if loaded[mod] {
			// Unload
			confirmModal := tview.NewModal().
				SetText(fmt.Sprintf("Unload module '%s'? This will remove loadmodule entries from config files.", mod)).
				AddButtons([]string{"No", "Yes"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					if buttonLabel == "Yes" {
						err := removeLoadmodule(mod, buildDir)
						if err != nil {
							errorModal := tview.NewModal().
								SetText(fmt.Sprintf("Error unloading: %v", err)).
								AddButtons([]string{"OK"}).
								SetDoneFunc(func(int, string) {
									pages.RemovePage("error_modal")
								})
							pages.AddPage("error_modal", errorModal, true, true)
						} else {
							rehashPrompt(app, pages, buildDir)
						}
					}
					pages.RemovePage("confirm_modal")
				})
			pages.AddPage("confirm_modal", confirmModal, true, true)
		} else {
			// Load
			confirmModal := tview.NewModal().
				SetText(fmt.Sprintf("Load module '%s'? This will add loadmodule to mods.conf.", mod)).
				AddButtons([]string{"No", "Yes"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					if buttonLabel == "Yes" {
						err := addLoadmodule(mod, buildDir)
						if err != nil {
							errorModal := tview.NewModal().
								SetText(fmt.Sprintf("Error loading: %v", err)).
								AddButtons([]string{"OK"}).
								SetDoneFunc(func(int, string) {
									pages.RemovePage("error_modal")
								})
							pages.AddPage("error_modal", errorModal, true, true)
						} else {
							rehashPrompt(app, pages, buildDir)
						}
					}
					pages.RemovePage("confirm_modal")
				})
			pages.AddPage("confirm_modal", confirmModal, true, true)
		}
	})

	list.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		inst := "No"
		if installed[mainText] {
			inst = "Yes"
		}
		load := "No"
		if loaded[mainText] {
			load = "Yes"
		}
		textView.SetText(fmt.Sprintf("Module: %s\n\nInstalled: %s\nLoaded: %s", mainText, inst, load))
		if loaded[mainText] {
			loadUnloadBtn.SetLabel("Unload Module")
		} else {
			loadUnloadBtn.SetLabel("Load Module")
		}
	})

	uninstallBtn := tview.NewButton("Uninstall Module").SetSelectedFunc(func() {
		index := list.GetCurrentItem()
		if index < 0 {
			return
		}
		mod, _ := list.GetItemText(index)
		confirmModal := tview.NewModal().
			SetText(fmt.Sprintf("Uninstall module '%s'? This will remove the .so file and loadmodule entries.", mod)).
			AddButtons([]string{"No", "Yes"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				if buttonLabel == "Yes" {
					err1 := removeSo(mod, buildDir)
					err2 := removeLoadmodule(mod, buildDir)
					if err1 != nil || err2 != nil {
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Error uninstalling: removeSo: %v, removeLoadmodule: %v", err1, err2)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("error_modal")
							})
						pages.AddPage("error_modal", errorModal, true, true)
					} else {
						rehashPrompt(app, pages, buildDir)
					}
				}
				pages.RemovePage("confirm_modal")
			})
		pages.AddPage("confirm_modal", confirmModal, true, true)
	})

	deleteBtn := tview.NewButton("Delete Module").SetSelectedFunc(func() {
		index := list.GetCurrentItem()
		if index < 0 {
			return
		}
		mod, _ := list.GetItemText(index)
		if !strings.HasPrefix(mod, "third/") || mod == "third/obbyscript" {
			errorModal := tview.NewModal().
				SetText("Cannot delete this module.").
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("error_modal")
				})
			pages.AddPage("error_modal", errorModal, true, true)
			return
		}
		confirmModal := tview.NewModal().
			SetText(fmt.Sprintf("Delete module '%s'? This will remove the .so file, loadmodule entries, and source file.", mod)).
			AddButtons([]string{"No", "Yes"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				if buttonLabel == "Yes" {
					err1 := removeSo(mod, buildDir)
					err2 := removeLoadmodule(mod, buildDir)
					err3 := removeFromSource(mod, sourceDir)
					if err1 != nil || err2 != nil || err3 != nil {
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Error deleting: removeSo: %v, removeLoadmodule: %v, removeFromSource: %v", err1, err2, err3)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("error_modal")
							})
						pages.AddPage("error_modal", errorModal, true, true)
					} else {
						rehashPrompt(app, pages, buildDir)
					}
				}
				pages.RemovePage("confirm_modal")
			})
		pages.AddPage("confirm_modal", confirmModal, true, true)
	})

	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.SwitchToPage("main_menu")
	})
	buttonBar := createButtonBar(loadUnloadBtn, uninstallBtn, deleteBtn, backBtn)

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	browserFlex := tview.NewFlex().
		AddItem(list, 40, 0, true).
		AddItem(textView, 0, 1, false)
	flex.AddItem(createHeader(), 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(CreateFooter("ESC: Main Menu | q: Quit"), 3, 0, false)
	pages.AddPage("check_modules", flex, true, true)
	// checkModulesFocusables = []tview.Primitive{list, textView, loadUnloadBtn, uninstallBtn, deleteBtn, backBtn} // TODO: handle globals
}