package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"utui/ui"

	"github.com/rivo/tview"
	"github.com/gdamore/tcell/v2"
)

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

func removeSo(mod string, buildDir string) error {
	soPath := filepath.Join(buildDir, "modules", mod+".so")
	return os.Remove(soPath)
}

func removeFromSource(mod string, sourceDir string) error {
	parts := strings.Split(mod, "/")
	if len(parts) == 2 && parts[0] == "third" {
		srcPath := filepath.Join(sourceDir, "src", "modules", parts[0], parts[1]+".c")
		os.Remove(srcPath) // ignore error
	}
	return nil
}

func uninstallObbyScript(app *tview.Application, pages *tview.Pages, buildDir, sourceDir string) {
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

func checkModulesPage(app *tview.Application, pages *tview.Pages, buildDir, sourceDir string) {
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
	currentList = list

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
	flex.AddItem(createHeader(), 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(createFooter("ESC: Main Menu | q: Quit"), 3, 0, false)
	pages.AddPage("check_modules", flex, true, true)
	checkModulesFocusables = []tview.Primitive{list, textView, loadUnloadBtn, uninstallBtn, deleteBtn, backBtn}
}

const (
	configFile = ".unrealircd_tui_config"

	// Menu item constants
	MenuBrowseScripts = "Browse GitHub Scripts (ObbyScript)"
	MenuViewInstalled = "View Installed Scripts (ObbyScript)"
	MenuBrowseModules = "Browse UnrealIRCd Third-Party Modules (C)"
	MenuCheckModules = "Check Installed Modules"
	MenuUninstallObbyScript = "Uninstall ObbyScript"
	MenuRemoteControl = "Remote Control (RPC)"
)

type Config struct {
	SourceDir string `json:"source_dir"`
	BuildDir  string `json:"build_dir"`
	Version   string `json:"version"`
}

type Downloads struct {
	Src    string `json:"src"`
	Winssl string `json:"winssl"`
}

type VersionInfo struct {
	Type      string    `json:"type"`
	Version   string    `json:"version"`
	Downloads Downloads `json:"downloads"`
}

type UpdateResponse map[string]map[string]VersionInfo

func loadConfig() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(home, configFile)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, nil // No config file
	}
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var config Config
	err = json.NewDecoder(file).Decode(&config)
	return &config, err
}

func saveConfig(config *Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(home, configFile)
	file, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(config)
}

func scanSourceDirs() ([]string, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, err
	}
	homeDir := usr.HomeDir
	var sourceDirs []string
	entries, err := os.ReadDir(homeDir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			dirPath := filepath.Join(homeDir, entry.Name())
			if hasRequiredFiles(dirPath) {
				sourceDirs = append(sourceDirs, dirPath)
			}
		}
	}
	return sourceDirs, nil
}

func hasRequiredFiles(dir string) bool {
	configSettings := filepath.Join(dir, "config.settings")
	unrealircd := filepath.Join(dir, "unrealircd")
	_, err1 := os.Stat(configSettings)
	_, err2 := os.Stat(unrealircd)
	return !os.IsNotExist(err1) && !os.IsNotExist(err2)
}

func getUnrealIRCdVersion(sourceDir string) (string, error) {
	configurePath := filepath.Join(sourceDir, "configure")
	content, err := os.ReadFile(configurePath)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`PACKAGE_VERSION='([^']+)'`)
	matches := re.FindStringSubmatch(string(content))
	if len(matches) < 2 {
		return "", fmt.Errorf("PACKAGE_VERSION not found in configure file")
	}
	return matches[1], nil
}

func getBasePathFromConfig(sourceDir string) (string, error) {
	configPath := filepath.Join(sourceDir, "config.settings")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`BASEPATH="([^"]+)"`)
	matches := re.FindStringSubmatch(string(content))
	if len(matches) < 2 {
		return "", fmt.Errorf("BASEPATH not found in config.settings")
	}
	return matches[1], nil
}


func buildAndInstall(sourceDir string) error {
	cmd := exec.Command("make")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.Command("make", "install")
	cmd.Dir = sourceDir
	return cmd.Run()
}

func setupConfigs(buildDir string) error {
	// Edit unrealircd.conf
	confFile := filepath.Join(buildDir, "conf", "unrealircd.conf")
	content, err := os.ReadFile(confFile)
	if err != nil {
		return err
	}
	newContent := "include \"scripts.conf\";\n" + string(content)
	err = os.WriteFile(confFile, []byte(newContent), 0644)
	if err != nil {
		return err
	}

	// Create scripts.conf
	scriptsConf := filepath.Join(buildDir, "conf", "scripts.conf")
	scriptsContent := `/* DO NOT EDIT THIS FILE MANUALLY */
scripts {
    // Script files go here
}
`
	return os.WriteFile(scriptsConf, []byte(scriptsContent), 0644)
}

func createScriptsDir(buildDir string) error {
	scriptsDir := filepath.Join(buildDir, "scripts")
	return os.MkdirAll(scriptsDir, 0755)
}

func installScript(buildDir, downloadURL, filename string) error {
	// Download script
	resp, err := http.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: %d", resp.StatusCode)
	}
	scriptsDir := filepath.Join(buildDir, "scripts")
	filePath := filepath.Join(scriptsDir, filename)
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return err
	}

	// Update scripts.conf
	return updateScriptsConf(buildDir, filePath)
}

func installModule(sourceDir, buildDir, downloadURL, filename string) error {
	// Download module to sourceDir/src/modules/third/
	thirdDir := filepath.Join(sourceDir, "src", "modules", "third")
	os.MkdirAll(thirdDir, 0755)
	filePath := filepath.Join(thirdDir, filename)
	resp, err := http.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: %d", resp.StatusCode)
	}
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return err
	}

	// Run make && make install from sourceDir
	cmd := exec.Command("make")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.Command("make", "install")
	cmd.Dir = sourceDir
	if err := cmd.Run(); err != nil {
		return err
	}

	// Update mods.conf
	moduleName := strings.TrimSuffix(filename, ".c")
	return updateModsConf(buildDir, moduleName)
}

func updateScriptsConf(buildDir, scriptPath string) error {
	scriptsConf := filepath.Join(buildDir, "conf", "scripts.conf")
	content, err := os.ReadFile(scriptsConf)
	if err != nil {
		// Create the file if it doesn't exist
		content = []byte("/* DO NOT EDIT THIS FILE MANUALLY */\nscripts {\n    // Script files go here\n}\n")
	}
	// Add the script to the scripts block
	lines := strings.Split(string(content), "\n")
	var newLines []string
	inScripts := false
	added := false
	for _, line := range lines {
		if strings.Contains(line, "scripts {") {
			inScripts = true
			newLines = append(newLines, line)
		} else if strings.Contains(line, "}") && inScripts {
			if !added {
				newLines = append(newLines, fmt.Sprintf("    '%s';", scriptPath))
				added = true
			}
			newLines = append(newLines, line)
			inScripts = false
		} else if !inScripts {
			newLines = append(newLines, line)
		}
	}
	return os.WriteFile(scriptsConf, []byte(strings.Join(newLines, "\n")), 0644)
}

func updateModsConf(buildDir, moduleName string) error {
	modsConf := filepath.Join(buildDir, "conf", "mods.conf")
	content, err := os.ReadFile(modsConf)
	if err != nil {
		// Create the file if it doesn't exist
		content = []byte("/* DO NOT EDIT THIS FILE MANUALLY */\n\n")
	}
	lines := strings.Split(string(content), "\n")
	// Check if already loaded
	for _, line := range lines {
		if strings.Contains(line, "third/"+moduleName) {
			return nil // Already loaded
		}
	}
	// Add at end
	newContent := string(content) + fmt.Sprintf("loadmodule \"third/%s\";\n", moduleName)
	return os.WriteFile(modsConf, []byte(newContent), 0644)
}

func rehash(buildDir string) error {
	cmd := exec.Command("./unrealircd", "rehash")
	cmd.Dir = buildDir
	return cmd.Run()
}

func removeScriptFromConf(buildDir, scriptPath string) error {
	scriptsConf := filepath.Join(buildDir, "conf", "scripts.conf")
	content, err := os.ReadFile(scriptsConf)
	if err != nil {
		return err
	}
	lines := strings.Split(string(content), "\n")
	var newLines []string
	for _, line := range lines {
		if !strings.Contains(line, scriptPath) {
			newLines = append(newLines, line)
		}
	}
	return os.WriteFile(scriptsConf, []byte(strings.Join(newLines, "\n")), 0644)
}

func uninstallScript(buildDir, filename string) error {
	scriptPath := filepath.Join(buildDir, "scripts", filename)
	if err := os.Remove(scriptPath); err != nil {
		return err
	}
	return removeScriptFromConf(buildDir, scriptPath)
}

func getInstalledScripts(buildDir string) ([]string, error) {
	scriptsDir := filepath.Join(buildDir, "scripts")
	entries, err := os.ReadDir(scriptsDir)
	if err != nil {
		return nil, err
	}
	var scripts []string
	for _, entry := range entries {
		if !entry.IsDir() {
			scripts = append(scripts, entry.Name())
		}
	}
	return scripts, nil
}

func createFooter(shortcuts string) *tview.TextView {
	footer := tview.NewTextView()
	footer.SetText(shortcuts)
	footer.SetTextAlign(tview.AlignCenter)
	footer.SetBorder(true)
	return footer
}

func createHeader() *tview.TextView {
	header := tview.NewTextView()
	header.SetText("UnrealIRCd Terminal Manager by Valware")
	header.SetTextAlign(tview.AlignCenter)
	header.SetTextColor(tcell.ColorYellow)
	header.SetBorder(true)
	header.SetBorderColor(tcell.ColorBlue)
	return header
}

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

type SyntaxTextArea struct {
	*tview.TextView
	text      []rune
	cursor    int
	title     string
	changed   func()
	saveFunc  func()
	cancelFunc func()
}

func NewSyntaxTextArea() *SyntaxTextArea {
	sta := &SyntaxTextArea{
		TextView: tview.NewTextView(),
		text:     []rune{},
		cursor:   0,
	}
	sta.TextView.SetDynamicColors(true)
	sta.TextView.SetScrollable(true)
	sta.TextView.SetWordWrap(false) // For editing, no wrap
	return sta
}

func (sta *SyntaxTextArea) SetText(text string) {
	sta.text = []rune(text)
	sta.cursor = len(sta.text)
	sta.updateDisplay()
}

func (sta *SyntaxTextArea) GetText() string {
	return string(sta.text)
}

func (sta *SyntaxTextArea) SetTitle(title string) {
	sta.title = title
	sta.TextView.SetTitle(title)
}

func (sta *SyntaxTextArea) SetChangedFunc(changed func()) {
	sta.changed = changed
}

func (sta *SyntaxTextArea) SetSaveFunc(saveFunc func()) {
	sta.saveFunc = saveFunc
}

func (sta *SyntaxTextArea) SetCancelFunc(cancelFunc func()) {
	sta.cancelFunc = cancelFunc
}

func (sta *SyntaxTextArea) updateDisplay() {
	textWithCursor := string(sta.text[:sta.cursor]) + "█" + string(sta.text[sta.cursor:])
	sta.TextView.SetText(textWithCursor)
}

func (sta *SyntaxTextArea) insertChar(ch rune) {
	if sta.cursor < 0 {
		sta.cursor = 0
	}
	if sta.cursor > len(sta.text) {
		sta.cursor = len(sta.text)
	}
	sta.text = append(sta.text[:sta.cursor], append([]rune{ch}, sta.text[sta.cursor:]...)...)
	sta.cursor++
	sta.updateDisplay()
	if sta.changed != nil {
		sta.changed()
	}
}

func (sta *SyntaxTextArea) deleteChar() {
	if sta.cursor > 0 {
		sta.text = append(sta.text[:sta.cursor-1], sta.text[sta.cursor:]...)
		sta.cursor--
		sta.updateDisplay()
		if sta.changed != nil {
			sta.changed()
		}
	}
}

func (sta *SyntaxTextArea) moveCursorLeft() {
	if sta.cursor > 0 {
		sta.cursor--
		sta.updateDisplay()
	}
}

func (sta *SyntaxTextArea) moveCursorRight() {
	if sta.cursor < len(sta.text) {
		sta.cursor++
		sta.updateDisplay()
	}
}

func (sta *SyntaxTextArea) moveCursorUp() {
	// Simple: find previous \n
	text := string(sta.text)
	lines := strings.Split(text, "\n")
	cursorLine := 0
	pos := 0
	for i, line := range lines {
		if pos+len(line) >= sta.cursor {
			cursorLine = i
			break
		}
		pos += len(line) + 1
	}
	if cursorLine > 0 {
		prevLineLen := len(lines[cursorLine-1])
		sta.cursor = pos - len(lines[cursorLine-1]) - 1 + prevLineLen
		if sta.cursor < 0 {
			sta.cursor = 0
		}
		sta.updateDisplay()
	}
}

func (sta *SyntaxTextArea) moveCursorDown() {
	text := string(sta.text)
	lines := strings.Split(text, "\n")
	cursorLine := 0
	pos := 0
	for i, line := range lines {
		if pos+len(line) >= sta.cursor {
			cursorLine = i
			break
		}
		pos += len(line) + 1
	}
	if cursorLine < len(lines)-1 {
		currentCol := sta.cursor - (pos - len(lines[cursorLine]) - 1)
		sta.cursor = pos + len(lines[cursorLine]) + 1 + currentCol
		if sta.cursor > len(sta.text) {
			sta.cursor = len(sta.text)
		}
		sta.updateDisplay()
	}
}

func (sta *SyntaxTextArea) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return sta.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if event.Key() == tcell.KeyCtrlS {
			if sta.saveFunc != nil {
				sta.saveFunc()
			}
			return
		}
		if event.Key() == tcell.KeyCtrlX {
			if sta.cancelFunc != nil {
				sta.cancelFunc()
			}
			return
		}
		switch event.Key() {
		case tcell.KeyLeft:
			sta.moveCursorLeft()
		case tcell.KeyRight:
			sta.moveCursorRight()
		case tcell.KeyUp:
			sta.moveCursorUp()
		case tcell.KeyDown:
			sta.moveCursorDown()
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			sta.deleteChar()
		case tcell.KeyEnter:
			sta.insertChar('\n')
		case tcell.KeyTab:
			sta.insertChar('\t')
		default:
			if event.Rune() != 0 {
				sta.insertChar(event.Rune())
			}
		}
	})
}

type GitHubItem struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
}

type Module struct {
	Name                   string
	Description            string
	Version                string
	Author                 string
	Documentation          string
	Troubleshooting        string
	Source                 string
	Sha256sum              string
	LastUpdated            string
	MinUnrealircdVersion   string
	MaxUnrealircdVersion   string
	PostInstallText        []string
}

func fetchRepoContents(owner, repo, path, ref string) ([]GitHubItem, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", owner, repo, path, ref)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var items []GitHubItem
	err = json.Unmarshal(body, &items)
	return items, err
}

func fetchFileContent(downloadURL string) (string, error) {
	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Download failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func parseModulesList(content string) ([]Module, error) {
	var modules []Module
	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "module \"") && strings.Contains(line, "\"") {
			// Start of module
			nameStart := strings.Index(line, "\"") + 1
			nameEnd := strings.LastIndex(line, "\"")
			name := line[nameStart:nameEnd]
			mod := Module{Name: name}
			i++
			// Expect {
			if i < len(lines) && strings.TrimSpace(lines[i]) == "{" {
				i++
			}
			for i < len(lines) && strings.TrimSpace(lines[i]) != "}" {
				line = strings.TrimSpace(lines[i])
				if line == "" || strings.HasPrefix(line, "/*") {
					i++
					continue
				}
				if strings.TrimSpace(line) == "post-install-text" {
					i++
					if i < len(lines) && strings.TrimSpace(lines[i]) == "{" {
						i++
					}
					var postText []string
					for i < len(lines) && strings.TrimSpace(lines[i]) != "}" {
						textLine := strings.TrimSpace(lines[i])
						if strings.HasSuffix(textLine, ";") {
							textLine = strings.TrimSuffix(textLine, ";")
						}
						if len(textLine) > 1 && textLine[0] == '"' && textLine[len(textLine)-1] == '"' {
							textLine = textLine[1 : len(textLine)-1]
						}
						postText = append(postText, textLine)
						i++
					}
					if i < len(lines) && strings.TrimSpace(lines[i]) == "}" {
						i++
					}
					mod.PostInstallText = postText
				} else if strings.Contains(line, ";") {
					parts := strings.SplitN(line, " ", 2)
					if len(parts) == 2 {
						key := parts[0]
						value := strings.TrimSuffix(parts[1], ";")
						if len(value) > 1 && value[0] == '"' && value[len(value)-1] == '"' {
							value = value[1 : len(value)-1]
						}
						switch key {
						case "description":
							mod.Description = value
						case "version":
							mod.Version = value
						case "author":
							mod.Author = value
						case "documentation":
							mod.Documentation = value
						case "troubleshooting":
							mod.Troubleshooting = value
						case "source":
							mod.Source = value
						case "sha256sum":
							mod.Sha256sum = value
						case "last-updated":
							mod.LastUpdated = value
						case "min-unrealircd-version":
							mod.MinUnrealircdVersion = value
						case "max-unrealircd-version":
							mod.MaxUnrealircdVersion = value
						}
					}
					i++
				}
				// remove the i++ here
			}
			if i < len(lines) && strings.TrimSpace(lines[i]) == "}" {
				i++
			}
			modules = append(modules, mod)
			i-- // Cancel the for loop's i++
		}
	}
	return modules, nil
}

func parseModulesSources() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	sourcesPath := filepath.Join(home, "unrealircd", "conf", "modules.sources.list")
	file, err := os.Open(sourcesPath)
	if err != nil {
		// If file doesn't exist, return default
		return []string{"https://modules.unrealircd.org/modules.list"}, nil
	}
	defer file.Close()

	var urls []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		urls = append(urls, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return urls, nil
}

func formatModuleDetails(mod Module) string {
	details := fmt.Sprintf("[blue]Module:[white] %s\n", mod.Name)
	if mod.Description != "" {
		details += fmt.Sprintf("[blue]Description:[white] %s\n", mod.Description)
	}
	if mod.Version != "" {
		details += fmt.Sprintf("[blue]Version:[white] %s\n", mod.Version)
	}
	if mod.Author != "" {
		details += fmt.Sprintf("[blue]Author:[white] %s\n", mod.Author)
	}
	if mod.Documentation != "" {
		details += fmt.Sprintf("[blue]Documentation:[white] %s\n", mod.Documentation)
	}
	if mod.Troubleshooting != "" {
		details += fmt.Sprintf("[blue]Troubleshooting:[white] %s\n", mod.Troubleshooting)
	}
	if mod.MinUnrealircdVersion != "" {
		details += fmt.Sprintf("[blue]Min UnrealIRCd Version:[white] %s\n", mod.MinUnrealircdVersion)
	}
	if mod.MaxUnrealircdVersion != "" {
		details += fmt.Sprintf("[blue]Max UnrealIRCd Version:[white] %s\n", mod.MaxUnrealircdVersion)
	}
	if mod.LastUpdated != "" {
		details += fmt.Sprintf("[blue]Last Updated:[white] %s\n", mod.LastUpdated)
	}
	if mod.Source != "" {
		details += fmt.Sprintf("[blue]Source:[white] %s\n", mod.Source)
	}
	if mod.Sha256sum != "" {
		details += fmt.Sprintf("[blue]SHA256 Sum:[white] %s\n", mod.Sha256sum)
	}
	return details
}

var currentList *tview.List

var mainMenuFocusables []tview.Primitive
var githubBrowserFocusables []tview.Primitive
var installedScriptsFocusables []tview.Primitive
var thirdPartyBrowserFocusables []tview.Primitive
var editScriptFocusables []tview.Primitive
var checkModulesFocusables []tview.Primitive
var obbyScriptSubmenuFocusables []tview.Primitive
var moduleManagerSubmenuFocusables []tview.Primitive

func main() {
	app := tview.NewApplication().EnableMouse(true)
	pages := tview.NewPages()

	app.SetMouseCapture(func(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
		if currentList != nil {
			currentItem := currentList.GetCurrentItem()
			if action == tview.MouseScrollUp && currentItem > 0 {
				currentList.SetCurrentItem(currentItem - 1)
				return nil, tview.MouseConsumed
			} else if action == tview.MouseScrollDown && currentItem < currentList.GetItemCount()-1 {
				currentList.SetCurrentItem(currentItem + 1)
				return nil, tview.MouseConsumed
			}
		}
		return event, action
	})

	config, err := loadConfig()
	if err != nil {
		// Handle error, perhaps show message
	}
	var sourceDir, buildDir string
	if config != nil && config.SourceDir != "" && config.BuildDir != "" {
		sourceDir = config.SourceDir
		buildDir = config.BuildDir
		// Check if dirs exist
		if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
			config = nil
		}
		if _, err := os.Stat(buildDir); os.IsNotExist(err) {
			config = nil
		}
	}

	if config == nil {
		// Scan for source dirs
		sourceDirs, err := scanSourceDirs()
		if err != nil {
			// Show error
			return
		}
		if len(sourceDirs) == 0 {
			// No source dirs found
			return
		} else if len(sourceDirs) == 1 {
			sourceDir = sourceDirs[0]
			version, err := getUnrealIRCdVersion(sourceDir)
			if err != nil {
				// Handle error, perhaps show message or set empty
				version = ""
			}
			usr, _ := user.Current()
			buildDir = filepath.Join(usr.HomeDir, "unrealircd")
			installPage(app, pages, sourceDir, buildDir)
			config = &Config{SourceDir: sourceDir, BuildDir: buildDir, Version: version}
			saveConfig(config)
		} else {
			// Show selection UI
			selectSourcePage(app, pages, sourceDirs, func(selected string) {
				sourceDir = selected
				version, err := getUnrealIRCdVersion(sourceDir)
				if err != nil {
					// Handle error
					version = ""
				}
				usr, _ := user.Current()
				buildDir = filepath.Join(usr.HomeDir, "unrealircd")
				installPage(app, pages, sourceDir, buildDir)
				config = &Config{SourceDir: sourceDir, BuildDir: buildDir, Version: version}
				saveConfig(config)
			})
		}
	} else {
		sourceDir = config.SourceDir
		buildDir = config.BuildDir
		mainMenuPage(app, pages, sourceDir, buildDir)
	}

	// Run
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyESC {
			pages.SwitchToPage("main_menu")
			return nil
		}
		if event.Rune() == 'q' {
			// Don't quit if we're on pages with input fields
			pageName, _ := pages.GetFrontPage()
			if pageName == "remote_log_streaming" || pageName == "rpc_setup_modal" {
				return event // Let the input field handle it
			}
			app.Stop()
			return nil
		}
		if event.Key() == tcell.KeyTab {
			pageName, _ := pages.GetFrontPage()
			var focusables []tview.Primitive
			switch pageName {
			case "main_menu":
				focusables = mainMenuFocusables
			case "github_browser":
				focusables = githubBrowserFocusables
			case "installed_scripts":
				focusables = installedScriptsFocusables
			case "third_party_browser":
				focusables = thirdPartyBrowserFocusables
			case "edit_script":
				focusables = editScriptFocusables
			case "obby_script_submenu":
				focusables = obbyScriptSubmenuFocusables
			case "module_manager_submenu":
				focusables = moduleManagerSubmenuFocusables
			}
			if len(focusables) > 0 {
				current := app.GetFocus()
				index := -1
				for i, p := range focusables {
					if p == current {
						index = i
						break
					}
				}
				var next int
				if index == -1 {
					next = 0
				} else {
					next = (index + 1) % len(focusables)
				}
				app.SetFocus(focusables[next])
			}
			return nil
		}
		return event
	})
	if err := app.SetRoot(pages, true).Run(); err != nil {
		panic(err)
	}
}

func selectSourcePage(app *tview.Application, pages *tview.Pages, sourceDirs []string, onSelect func(string)) {
	list := tview.NewList()
	list.SetTitle("Select UnrealIRCd Source Directory").SetTitleAlign(tview.AlignCenter).SetTitleColor(tcell.ColorBlue)
	list.SetBorder(true).SetBorderColor(tcell.ColorBlue)
	for _, dir := range sourceDirs {
		list.AddItem(dir, "", 0, nil)
	}
	currentList = list
	list.SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		onSelect(mainText)
		pages.RemovePage("select_source")
	})
	selectBtn := tview.NewButton("Select").SetSelectedFunc(func() {
		index := list.GetCurrentItem()
		if index >= 0 {
			mainText, _ := list.GetItemText(index)
			onSelect(mainText)
			pages.RemovePage("select_source")
		}
	})
	buttonBar := createButtonBar(selectBtn)

	contentFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	contentFlex.AddItem(createHeader(), 3, 0, false).AddItem(list, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(createFooter("Enter: Select | q: Quit"), 3, 0, false)

	// Auto-size height based on content
	contentHeight := len(sourceDirs) + 8  // items + title + buttons + footer + padding
	if contentHeight < 15 {
		contentHeight = 15
	}
	if contentHeight > 20 {
		contentHeight = 20
	}

	centeredFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView(), 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(tview.NewTextView(), 0, 1, false).
			AddItem(contentFlex, 60, 0, true).
			AddItem(tview.NewTextView(), 0, 1, false), contentHeight, 0, true).
		AddItem(tview.NewTextView(), 0, 1, false)

	pages.AddPage("select_source", centeredFlex, true, true)
}

func installPage(app *tview.Application, pages *tview.Pages, sourceDir, buildDir string) {
	currentList = nil
	textView := tview.NewTextView()
	textView.SetBorder(true).SetTitle("Installation Progress")
	textView.SetDynamicColors(true)
	textView.SetWordWrap(true)

	go func() {
		update := func(msg string) {
			app.QueueUpdateDraw(func() {
				fmt.Fprintf(textView, "%s\n", msg)
			})
		}

		update("Building and installing...")
		if err := buildAndInstall(sourceDir); err != nil {
			update(fmt.Sprintf("Error: %v", err))
			return
		}
		update("Setting up configs...")
		if err := setupConfigs(buildDir); err != nil {
			update(fmt.Sprintf("Error: %v", err))
			return
		}
		update("Creating scripts directory...")
		if err := createScriptsDir(buildDir); err != nil {
			update(fmt.Sprintf("Error: %v", err))
			return
		}
		update("Installation complete!")
		app.QueueUpdateDraw(func() {
			pages.RemovePage("install")
			mainMenuPage(app, pages, sourceDir, buildDir)
		})
	}()

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	flex.AddItem(createHeader(), 3, 0, false).AddItem(textView, 0, 1, true).AddItem(createFooter("q: Quit"), 3, 0, false)
	pages.AddPage("install", flex, true, true)
}

func highlightLine(line string) string {
	// Apply highlighting to non-comment lines
	// Operators first (longer first, non-overlapping)
	operators := []string{"==", "!=", ">=", "<=", "has", "!has", "++", "--", "+=", "-=", "*=", "/=", "+", "-", "*", "/", "="}
	type match struct {
		start, end int
		repl       string
	}
	var matches []match
	for _, op := range operators {
		re := regexp.MustCompile(regexp.QuoteMeta(op))
		for _, loc := range re.FindAllStringIndex(line, -1) {
			matches = append(matches, match{loc[0], loc[1], "[cyan]" + op + "[-]"})
		}
	}
	// Sort by length descending, then by start ascending
	sort.Slice(matches, func(i, j int) bool {
		leni := matches[i].end - matches[i].start
		lenj := matches[j].end - matches[j].start
		if leni != lenj {
			return leni > lenj
		}
		return matches[i].start < matches[j].start
	})
	// Remove overlapping matches, keep longer ones
	var filtered []match
	lastEnd := -1
	for _, m := range matches {
		if m.start >= lastEnd {
			filtered = append(filtered, m)
			lastEnd = m.end
		}
	}
	// Apply from end to start
	for i := len(filtered) - 1; i >= 0; i-- {
		m := filtered[i]
		line = line[:m.start] + m.repl + line[m.end:]
	}

	// Variables
	re := regexp.MustCompile(`\$[a-zA-Z_][a-zA-Z0-9_.]*`)
	line = re.ReplaceAllStringFunc(line, func(s string) string { return "[yellow]" + s + "[-]" })
	re = regexp.MustCompile(`%[a-zA-Z_][a-zA-Z0-9_]*`)
	line = re.ReplaceAllStringFunc(line, func(s string) string { return "[yellow]" + s + "[-]" })

	// Keywords
	keywords := []string{"on", "if", "var", "const", "return"}
	for _, kw := range keywords {
		re := regexp.MustCompile("\\b" + kw + "\\b")
		line = re.ReplaceAllStringFunc(line, func(s string) string { return "[blue]" + s + "[-]" })
	}

	// Functions
	functions := []string{"sendnotice", "privmsg", "globops", "kick", "ban", "unban", "invite", "topic", "mode", "umode", "kill", "gline", "shun", "isupport", "cap", "ischanop", "isvoice", "ishalfop", "isadmin", "isowner", "issg", "isoper", "issecure", "ishidden", "hascap"}
	for _, fn := range functions {
		re := regexp.MustCompile("\\b" + fn + "\\b")
		line = re.ReplaceAllStringFunc(line, func(s string) string { return "[green]" + s + "[-]" })
	}

	// Events
	events := []string{"START", "CONNECT", "QUIT", "CAN_JOIN", "JOIN", "PART", "KICK", "CHANNEL_CREATE", "CHANNEL_DESTROY", "PRIVMSG", "NOTICE", "TOPIC", "NICK", "AWAY", "OPER", "UMODE_CHANGE", "MODE", "CHANMODE", "INVITE", "KNOCK", "KILL"}
	for _, ev := range events {
		re := regexp.MustCompile("\\b" + ev + "\\b")
		line = re.ReplaceAllStringFunc(line, func(s string) string { return "[magenta]" + ev + "[-]" })
	}

	// Strings (applied last to avoid highlighting inside strings)
	re = regexp.MustCompile(`"([^"]*)"`)
	line = re.ReplaceAllStringFunc(line, func(s string) string { return "[red]" + s + "[-]" })

	return line
}

func highlightUSL(text string) string {
	lines := strings.Split(text, "\n")
	inMultilineComment := false
	for i, line := range lines {
		if inMultilineComment {
			if strings.Contains(line, "*/") {
				idx := strings.Index(line, "*/")
				lines[i] = "[gray]" + line[:idx+2] + "[-]" + highlightLine(line[idx+2:])
				inMultilineComment = false
			} else {
				lines[i] = "[gray]" + line + "[-]"
			}
		} else if strings.HasPrefix(strings.TrimSpace(line), "//") {
			// Comment line, color entire line
			lines[i] = "[gray]" + line + "[-]"
		} else if strings.Contains(line, "/*") {
			idx := strings.Index(line, "/*")
			prefix := highlightLine(line[:idx])
			rest := line[idx:]
			if strings.Contains(rest, "*/") {
				endIdx := strings.Index(rest, "*/") + 2
				comment := rest[:endIdx]
				suffix := highlightLine(rest[endIdx:])
				lines[i] = prefix + "[gray]" + comment + "[-]" + suffix
			} else {
				lines[i] = prefix + "[gray]" + rest + "[-]"
				inMultilineComment = true
			}
		} else {
			lines[i] = highlightLine(line)
		}
	}
	return strings.Join(lines, "\n")
}

func mainMenuPage(app *tview.Application, pages *tview.Pages, sourceDir, buildDir string) {
	// Text view on right for descriptions
	textView := &FocusableTextView{tview.NewTextView()}
	textView.SetBorder(true).SetTitle("Description")
	textView.SetDynamicColors(true)
	textView.SetWordWrap(true)
	textView.SetScrollable(true)

	// Descriptions
	descriptions := map[string]string{
		"• Module Manager": `Manage UnrealIRCd C modules.

Features:
• Browse and install third-party C modules
• Check status of installed and loaded modules
• Upload and install custom modules
• Automatic compilation and installation
• Module management and troubleshooting

Comprehensive module management for your IRC server.`,
		"• Check for Updates": `Check for available UnrealIRCd updates.

Features:
• Fetch latest stable version from official website
• Compare with your current installed version
• Automatic upgrade option with ./unrealircd upgrade
• Update build directory configuration after successful upgrade

Keep your UnrealIRCd installation up to date with the latest stable release.`,
		"• Remote Control (RPC)": `Control UnrealIRCd server via JSON-RPC API.

Features:
• View and manage channels in real-time
• Monitor connected users and their details
• View server information and statistics
• Manage server bans (G-lines, K-lines, Z-lines, etc)
• Remote server administration without direct access

Connect to your UnrealIRCd server's RPC interface for live control.`,
		"• Set up new UnrealIRCd install": `Set up a new UnrealIRCd installation.

Features:
• Download the latest stable UnrealIRCd source code
• Extract and configure in a new directory
• Automatic build directory setup
• Prepare for compilation and installation

Create a fresh UnrealIRCd installation alongside existing ones.`,
		"• Switch UnrealIRCd install": `Switch between installed UnrealIRCd versions.

Features:
• List all detected UnrealIRCd source directories
• View version information for each install
• Switch configuration to use a different version
• Automatic build directory adjustment

Easily switch between multiple UnrealIRCd installations.`,
		"• ObbyScript": `Manage ObbyScript installation and scripts.

Features:
• Browse and install scripts from GitHub
• View and edit installed scripts
• Uninstall ObbyScript completely
• Automatic configuration management
• Syntax highlighting and code preview

Extend your IRC server functionality with custom scripts and automation.`}

	list := tview.NewList()
	list.SetBorder(true).SetBorderColor(tcell.ColorGreen)
	list.AddItem("• Module Manager", "  Manage UnrealIRCd C modules", 0, nil)
	list.AddItem("• Check for Updates", "  Check for available UnrealIRCd updates", 0, nil)
	list.AddItem("• Set up new UnrealIRCd install", "  Set up a new UnrealIRCd installation", 0, nil)
	list.AddItem("• Switch UnrealIRCd install", "  Switch between installed UnrealIRCd versions", 0, nil)
	list.AddItem("• Remote Control (RPC)", "  Control UnrealIRCd server via JSON-RPC API", 0, nil)
	list.AddItem("• ObbyScript", "  Manage ObbyScript installation and scripts", 0, nil)

	currentList = list

	header := createHeader()

	var lastClickTime time.Time
	var lastClickIndex = -1

	list.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		if desc, ok := descriptions[mainText]; ok {
			textView.SetText(desc)
		}
		now := time.Now()
		if index == lastClickIndex && now.Sub(lastClickTime) < 300*time.Millisecond {
			// Double-click detected
			switch mainText {
			case "• Module Manager":
				moduleManagerSubmenuPage(app, pages, sourceDir, buildDir)
			case "• Check for Updates":
				checkForUpdatesPage(app, pages, sourceDir, buildDir)
			case "• Set up new UnrealIRCd install":
				setupNewInstallPage(app, pages)
			case "• Switch UnrealIRCd install":
				switchInstallPage(app, pages)
			case "• Remote Control (RPC)":
				ui.RemoteControlMenuPage(app, pages, buildDir)
			case "• ObbyScript":
				obbyScriptSubmenuPage(app, pages, sourceDir, buildDir)
			}
		}
		lastClickIndex = index
		lastClickTime = now
	})

	list.SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		// For Enter key
		switch mainText {
		case "• Module Manager":
			moduleManagerSubmenuPage(app, pages, sourceDir, buildDir)
		case "• Check for Updates":
			checkForUpdatesPage(app, pages, sourceDir, buildDir)
		case "• Set up new UnrealIRCd install":
			setupNewInstallPage(app, pages)
		case "• Switch UnrealIRCd install":
			switchInstallPage(app, pages)
		case "• Remote Control (RPC)":
			ui.RemoteControlMenuPage(app, pages, buildDir)
		case "• ObbyScript":
			obbyScriptSubmenuPage(app, pages, sourceDir, buildDir)
		}
	})

	list.SetInputCapture(nil) // Remove custom input capture

	// Set initial description
	if len(descriptions) > 0 {
		textView.SetText(descriptions["• Module Manager"])
	}

	quitBtn := tview.NewButton("Quit").SetSelectedFunc(func() {
		app.Stop()
	})

	buttonBar := createButtonBar(quitBtn)

	// Layout
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	browserFlex := tview.NewFlex().
		AddItem(list, 40, 0, true).
		AddItem(textView, 0, 1, false)
	flex.AddItem(header, 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(createFooter("ESC: Back | Enter: Select | q: Quit"), 3, 0, false)
	pages.AddPage("main_menu", flex, true, true)
	mainMenuFocusables = []tview.Primitive{list, textView, quitBtn}
}

func checkForUpdatesPage(app *tview.Application, pages *tview.Pages, sourceDir, buildDir string) {
	// Load current config
	config, err := loadConfig()
	if err != nil {
		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Error loading config: %v", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("config_error_modal")
			})
		pages.AddPage("config_error_modal", errorModal, true, true)
		return
	}
	if config == nil || config.Version == "" {
		errorModal := tview.NewModal().
			SetText("No version information in config. Please reconfigure.").
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("no_version_modal")
			})
		pages.AddPage("no_version_modal", errorModal, true, true)
		return
	}

	// Fetch update info
	resp, err := http.Get("https://www.unrealircd.org/downloads/list.json")
	if err != nil {
		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Error fetching updates: %v", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("fetch_error_modal")
			})
		pages.AddPage("fetch_error_modal", errorModal, true, true)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Failed to fetch updates: HTTP %d", resp.StatusCode)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("http_error_modal")
			})
		pages.AddPage("http_error_modal", errorModal, true, true)
		return
	}

	var updateResp UpdateResponse
	if err := json.NewDecoder(resp.Body).Decode(&updateResp); err != nil {
		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Error parsing update info: %v", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("parse_error_modal")
			})
		pages.AddPage("parse_error_modal", errorModal, true, true)
		return
	}

	// Find stable version
	var stableVersion string
	for _, versions := range updateResp {
		if stable, ok := versions["Stable"]; ok {
			stableVersion = stable.Version
			break
		}
	}
	if stableVersion == "" {
		errorModal := tview.NewModal().
			SetText("No stable version found in update info.").
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("no_stable_modal")
			})
		pages.AddPage("no_stable_modal", errorModal, true, true)
		return
	}

	// Compare versions
	if compareVersions(config.Version, stableVersion) >= 0 {
		infoModal := tview.NewModal().
			SetText(fmt.Sprintf("You are up to date!\n\nCurrent version: %s\nLatest stable: %s", config.Version, stableVersion)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("up_to_date_modal")
			})
		pages.AddPage("up_to_date_modal", infoModal, true, true)
		return
	}

	// Show update modal
	updateModal := tview.NewModal().
		SetText(fmt.Sprintf("Update available!\n\nCurrent version: %s\nLatest stable: %s\n\nDo you want to upgrade?", config.Version, stableVersion)).
		AddButtons([]string{"Yes", "No"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Yes" {
				// Run upgrade
				cmd := exec.Command("./unrealircd", "upgrade")
				cmd.Dir = sourceDir
				if err := cmd.Run(); err != nil {
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Upgrade failed: %v", err)).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("upgrade_error_modal")
						})
					pages.AddPage("upgrade_error_modal", errorModal, true, true)
					return
				}
				// Success, update config
				usr, _ := user.Current()
				newBuildDir := filepath.Join(usr.HomeDir, "unrealircd-"+stableVersion)
				config.BuildDir = newBuildDir
				config.Version = stableVersion
				if err := saveConfig(config); err != nil {
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Failed to save config: %v", err)).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("save_error_modal")
						})
					pages.AddPage("save_error_modal", errorModal, true, true)
					return
				}
				successModal := tview.NewModal().
					SetText(fmt.Sprintf("Upgrade successful!\n\nNew build directory: %s\nNew version: %s", newBuildDir, stableVersion)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("upgrade_success_modal")
						pages.SwitchToPage("main_menu")
					})
				pages.AddPage("upgrade_success_modal", successModal, true, true)
			}
			pages.RemovePage("update_modal")
		})
	pages.AddPage("update_modal", updateModal, true, true)
}

func compareVersions(v1, v2 string) int {
	// Simple version comparison: split by "." and compare as ints
	p1 := strings.Split(v1, ".")
	p2 := strings.Split(v2, ".")
	for i := 0; i < len(p1) && i < len(p2); i++ {
		n1, _ := strconv.Atoi(p1[i])
		n2, _ := strconv.Atoi(p2[i])
		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}
	if len(p1) < len(p2) {
		return -1
	}
	if len(p1) > len(p2) {
		return 1
	}
	return 0
}

func setupNewInstallPage(app *tview.Application, pages *tview.Pages) {
	// First, fetch the latest version to suggest a default directory name
	resp, err := http.Get("https://www.unrealircd.org/downloads/list.json")
	if err != nil {
		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Error fetching updates: %v", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("fetch_error_modal")
			})
		pages.AddPage("fetch_error_modal", errorModal, true, true)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Failed to fetch updates: HTTP %d", resp.StatusCode)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("http_error_modal")
			})
		pages.AddPage("http_error_modal", errorModal, true, true)
		return
	}

	var updateResp UpdateResponse
	if err := json.NewDecoder(resp.Body).Decode(&updateResp); err != nil {
		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Error parsing update info: %v", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("parse_error_modal")
			})
		pages.AddPage("parse_error_modal", errorModal, true, true)
		return
	}

	// Find stable version
	var stableVersion string
	var downloadURL string
	for _, versions := range updateResp {
		if stable, ok := versions["Stable"]; ok {
			stableVersion = stable.Version
			downloadURL = stable.Downloads.Src
			break
		}
	}
	if stableVersion == "" || downloadURL == "" {
		errorModal := tview.NewModal().
			SetText("No stable version found in update info.").
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("no_stable_modal")
			})
		pages.AddPage("no_stable_modal", errorModal, true, true)
		return
	}

	// Create form for directory selection
	form := tview.NewForm()
	form.SetBorder(true).SetTitle(fmt.Sprintf("Set up UnrealIRCd %s", stableVersion))
	form.SetBorderColor(tcell.ColorBlue)

	usr, _ := user.Current()
	defaultDir := fmt.Sprintf("unrealircd-%s", stableVersion)

	form.AddInputField("Source Directory:", filepath.Join(usr.HomeDir, defaultDir), 50, nil, nil).
		AddButton("Next", func() {
			sourceDir := form.GetFormItem(0).(*tview.InputField).GetText()
			if sourceDir == "" {
				sourceDir = filepath.Join(usr.HomeDir, defaultDir)
			}
			// Start download and extraction
			downloadAndExtract(app, pages, stableVersion, downloadURL, sourceDir)
		}).
		AddButton("Cancel", func() {
			pages.SwitchToPage("main_menu")
		})

	form.SetFocus(0)
	pages.AddPage("setup_form", form, true, true)
}

func downloadAndExtract(app *tview.Application, pages *tview.Pages, version, downloadURL, sourceDir string) {
	// Show progress modal
	progressModal := tview.NewModal().
		SetText(fmt.Sprintf("Setting up UnrealIRCd %s...\n\nDownloading source...", version)).
		AddButtons([]string{}).
		SetDoneFunc(func(int, string) {})
	pages.AddPage("download_progress_modal", progressModal, true, true)

	go func() {
		// Download the tar.gz
		resp, err := http.Get(downloadURL)
		if err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("download_progress_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error downloading source: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("download_error_modal")
					})
				pages.AddPage("download_error_modal", errorModal, true, true)
			})
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("download_progress_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to download source: HTTP %d", resp.StatusCode)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("http_download_error_modal")
					})
				pages.AddPage("http_download_error_modal", errorModal, true, true)
			})
			return
		}

		// Create temp file
		tempFile, err := os.CreateTemp("", "unrealircd-*.tar.gz")
		if err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("download_progress_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error creating temp file: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("temp_error_modal")
					})
				pages.AddPage("temp_error_modal", errorModal, true, true)
			})
			return
		}
		defer os.Remove(tempFile.Name())
		defer tempFile.Close()

		_, err = io.Copy(tempFile, resp.Body)
		if err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("download_progress_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error saving download: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("save_error_modal")
					})
				pages.AddPage("save_error_modal", errorModal, true, true)
			})
			return
		}
		tempFile.Close()

		// Update progress
		app.QueueUpdateDraw(func() {
			progressModal.SetText(fmt.Sprintf("Setting up UnrealIRCd %s...\n\nExtracting source...", version))
		})

		// Extract
		err = extractTarGz(tempFile.Name(), sourceDir)
		if err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("download_progress_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error extracting source: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("extract_error_modal")
					})
				pages.AddPage("extract_error_modal", errorModal, true, true)
			})
			return
		}

		// Success - show config questions
		app.QueueUpdateDraw(func() {
			pages.RemovePage("download_progress_modal")
			configQuestionsPage(app, pages, sourceDir, version)
		})
	}()
}

func configQuestionsPage(app *tview.Application, pages *tview.Pages, sourceDir, version string) {
	usr, _ := user.Current()
	defaultInstallDir := filepath.Join(usr.HomeDir, "unrealircd")

	form := tview.NewForm()
	form.SetBorder(true).SetTitle(fmt.Sprintf("UnrealIRCd %s Configuration", version))
	form.SetBorderColor(tcell.ColorBlue)

	// Add all the configuration questions
	form.AddInputField("Installation directory:", defaultInstallDir, 50, nil, nil).
		AddInputField("Default permissions for config files (0600 recommended):", "0600", 10, nil, nil).
		AddInputField("Path to OpenSSL/LibreSSL (leave empty for auto-detect):", "", 50, nil, nil).
		AddDropDown("Support for non-HTTPS protocols (ftp, tftp, smb, http)?", []string{"No", "Yes"}, 0, nil).
		AddInputField("Nickname history length:", "2000", 10, nil, nil).
		AddDropDown("GeoIP engine:", []string{"classic", "libmaxminddb", "none"}, 0, nil).
		AddInputField("Maximum sockets/file descriptors (auto recommended):", "auto", 20, nil, nil).
		AddDropDown("Enable AddressSanitizer & UndefinedBehaviorSanitizer?", []string{"No", "Yes"}, 0, nil).
		AddInputField("Custom parameters for configure (optional):", "", 50, nil, nil)

	form.AddButton("Configure & Install", func() {
		// Collect all the answers
		installDir := form.GetFormItem(0).(*tview.InputField).GetText()
		defPerm := form.GetFormItem(1).(*tview.InputField).GetText()
		sslDir := form.GetFormItem(2).(*tview.InputField).GetText()
		_, remoteIncIdx := form.GetFormItem(3).(*tview.DropDown).GetCurrentOption()
		nickHist := form.GetFormItem(4).(*tview.InputField).GetText()
		_, geoipIdx := form.GetFormItem(5).(*tview.DropDown).GetCurrentOption()
		maxConn := form.GetFormItem(6).(*tview.InputField).GetText()
		_, sanitizerIdx := form.GetFormItem(7).(*tview.DropDown).GetCurrentOption()
		extraPara := form.GetFormItem(8).(*tview.InputField).GetText()

		// Convert dropdown values (GetCurrentOption returns string, int - but seems to be string, string in practice)
		remoteIncStr := "1" // Default to 1 (only HTTPS)
		if remoteIncIdx == "1" || remoteIncIdx == "Yes" {
			remoteIncStr = "2" // With cURL
		}
		geoipStr := "classic" // Default
		if geoipIdx == "1" {
			geoipStr = "libmaxminddb"
		} else if geoipIdx == "2" {
			geoipStr = "none"
		}
		sanitizerStr := "" // Default empty
		if sanitizerIdx == "1" || sanitizerIdx == "Yes" {
			sanitizerStr = "1"
		}

		// Save config.settings
		err := saveConfigSettings(sourceDir, installDir, defPerm, sslDir, remoteIncStr, nickHist, geoipStr, maxConn, sanitizerStr, extraPara)
		if err != nil {
			errorModal := tview.NewModal().
				SetText(fmt.Sprintf("Error saving config.settings: %v", err)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("config_save_error_modal")
				})
			pages.AddPage("config_save_error_modal", errorModal, true, true)
			return
		}

		// Continue with configuration and compilation
		continueInstallation(app, pages, sourceDir, version, installDir)
	})

	form.AddButton("Cancel", func() {
		pages.SwitchToPage("main_menu")
	})

	// Make the form scrollable if needed
	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab {
			// Handle tab navigation
			return event
		}
		return event
	})

	pages.AddPage("config_questions", form, true, true)
}

func saveConfigSettings(sourceDir, basePath, defPerm, sslDir, remoteInc, nickHist, geoip, maxConn, sanitizer, extraPara string) error {
	configPath := filepath.Join(sourceDir, "config.settings")

	content := fmt.Sprintf(`#
# These are the settings saved from running './Config'.
# Note that it is not recommended to edit config.settings by hand!
# Chances are you misunderstand what a variable does or what the
# supported values are. You better just re-run the ./Config script
# and answer appropriately there, to get a correct config.settings
# file.
#
BASEPATH="%s"
BINDIR="%s/bin"
DATADIR="%s/data"
CONFDIR="%s/conf"
MODULESDIR="%s/modules"
LOGDIR="%s/logs"
CACHEDIR="%s/cache"
DOCDIR="%s/doc"
TMPDIR="%s/tmp"
PRIVATELIBDIR="%s/lib"
MAXCONNECTIONS_REQUEST="%s"
NICKNAMEHISTORYLENGTH="%s"
GEOIP="%s"
DEFPERM="%s"
SSLDIR="%s"
REMOTEINC="%s"
CURLDIR="/usr"
NOOPEROVERRIDE=""
OPEROVERRIDEVERIFY=""
GENCERTIFICATE=""
SANITIZER="%s"
EXTRAPARA="%s"
ADVANCED=""
`, basePath, basePath, basePath, basePath, basePath, basePath, basePath, basePath, basePath, basePath,
   maxConn, nickHist, geoip, defPerm, sslDir, remoteInc, sanitizer, extraPara)

	return os.WriteFile(configPath, []byte(content), 0644)
}

func continueInstallation(app *tview.Application, pages *tview.Pages, sourceDir, version, buildDir string) {
	// Show progress modal
	progressModal := tview.NewModal().
		SetText(fmt.Sprintf("Installing UnrealIRCd %s...\n\nConfiguring...", version)).
		AddButtons([]string{}).
		SetDoneFunc(func(int, string) {})
	pages.AddPage("install_progress_modal", progressModal, true, true)

	go func() {
		// Run ./Config -quick to apply the config.settings
		configCmd := exec.Command("./Config", "-quick")
		configCmd.Dir = sourceDir
		configOutput, err := configCmd.CombinedOutput()
		if err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("install_progress_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error running ./Config -quick: %v\nOutput: %s", err, string(configOutput))).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("config_quick_error_modal")
					})
				pages.AddPage("config_quick_error_modal", errorModal, true, true)
			})
			return
		}

		// Update progress
		app.QueueUpdateDraw(func() {
			progressModal.SetText(fmt.Sprintf("Installing UnrealIRCd %s...\n\nCompiling...", version))
		})

		// Run make
		makeCmd := exec.Command("make")
		makeCmd.Dir = sourceDir
		makeOutput, err := makeCmd.CombinedOutput()
		if err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("install_progress_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error running make: %v\nOutput: %s", err, string(makeOutput))).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("make_error_modal")
					})
				pages.AddPage("make_error_modal", errorModal, true, true)
			})
			return
		}

		// Update progress
		app.QueueUpdateDraw(func() {
			progressModal.SetText(fmt.Sprintf("Installing UnrealIRCd %s...\n\nInstalling...", version))
		})

		// Run make install
		installCmd := exec.Command("make", "install")
		installCmd.Dir = sourceDir
		installOutput, err := installCmd.CombinedOutput()
		if err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("install_progress_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error running make install: %v\nOutput: %s", err, string(installOutput))).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("install_error_modal")
					})
				pages.AddPage("install_error_modal", errorModal, true, true)
			})
			return
		}

		// Set up config
		err = setupConfigFile(buildDir)
		if err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("install_progress_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error setting up config file: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("config_setup_error_modal")
					})
				pages.AddPage("config_setup_error_modal", errorModal, true, true)
			})
			return
		}

		// Save config
		config := &Config{
			SourceDir: sourceDir,
			BuildDir:  buildDir,
			Version:   version,
		}
		err = saveConfig(config)
		if err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("install_progress_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error saving config: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("config_save_error_modal")
					})
				pages.AddPage("config_save_error_modal", errorModal, true, true)
			})
			return
		}

		// Success
		app.QueueUpdateDraw(func() {
			pages.RemovePage("install_progress_modal")
			successModal := tview.NewModal().
				SetText(fmt.Sprintf("UnrealIRCd %s installed successfully!\n\nSource: %s\nBuild: %s\n\nInstallation complete with configuration file ready.", version, sourceDir, buildDir)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("install_success_modal")
					pages.SwitchToPage("main_menu")
				})
			pages.AddPage("install_success_modal", successModal, true, true)
		})
	}()
}

func setupConfigFile(buildDir string) error {
	confDir := filepath.Join(buildDir, "conf")
	examplesDir := filepath.Join(confDir, "examples")
	exampleConf := filepath.Join(examplesDir, "example.conf")
	targetConf := filepath.Join(confDir, "unrealircd.conf")

	// Check if unrealircd.conf already exists
	if _, err := os.Stat(targetConf); err == nil {
		// Already exists, skip
		return nil
	}

	// Check if example.conf exists
	if _, err := os.Stat(exampleConf); os.IsNotExist(err) {
		// Try alternative path for older versions
		exampleConf = filepath.Join(confDir, "example.conf")
		if _, err := os.Stat(exampleConf); os.IsNotExist(err) {
			return fmt.Errorf("example config file not found")
		}
	}

	// Copy example.conf to unrealircd.conf
	sourceFile, err := os.Open(exampleConf)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(targetConf)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

func extractTarGz(tarGzPath, destDir string) error {
	file, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	// First pass: find the top-level directory
	var topLevelDir string
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if header.Typeflag == tar.TypeDir {
			parts := strings.Split(strings.Trim(header.Name, "/"), "/")
			if len(parts) > 0 && topLevelDir == "" {
				topLevelDir = parts[0]
			} else if len(parts) > 0 && parts[0] != topLevelDir {
				// Multiple top-level directories, don't strip
				topLevelDir = ""
				break
			}
		}
	}

	// Reset the reader for second pass
	file.Seek(0, 0)
	gzr, err = gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzr.Close()
	tr = tar.NewReader(gzr)

	// Second pass: extract files
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Strip the top-level directory if it exists
		targetName := header.Name
		if topLevelDir != "" && strings.HasPrefix(targetName, topLevelDir+"/") {
			targetName = strings.TrimPrefix(targetName, topLevelDir+"/")
		}
		if targetName == "" {
			continue // Skip the top-level directory itself
		}

		target := filepath.Join(destDir, targetName)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			dir := filepath.Dir(target)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	return nil
}

func switchInstallPage(app *tview.Application, pages *tview.Pages) {
	// Scan for source dirs
	sourceDirs, err := scanSourceDirs()
	if err != nil {
		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Error scanning source dirs: %v", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("scan_error_modal")
			})
		pages.AddPage("scan_error_modal", errorModal, true, true)
		return
	}

	if len(sourceDirs) == 0 {
		errorModal := tview.NewModal().
			SetText("No UnrealIRCd source directories found.").
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("no_dirs_modal")
			})
		pages.AddPage("no_dirs_modal", errorModal, true, true)
		return
	}

	// Create list with versions
	list := tview.NewList()
	list.SetBorder(true).SetTitle("Select UnrealIRCd Install")
	list.SetBorderColor(tcell.ColorBlue)

	for _, dir := range sourceDirs {
		version, err := getUnrealIRCdVersion(dir)
		if err != nil {
			version = "Unknown"
		}
		displayName := fmt.Sprintf("UnrealIRCd %s (%s)", version, filepath.Base(dir))
		list.AddItem(displayName, fmt.Sprintf("Source: %s", dir), 0, nil)
	}

	list.SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		selectedDir := sourceDirs[index]
		version, err := getUnrealIRCdVersion(selectedDir)
		if err != nil {
			version = ""
		}

		// Read the build directory from the config.settings BASEPATH
		buildDir, err := getBasePathFromConfig(selectedDir)
		if err != nil {
			// Fallback to version-based path if BASEPATH can't be read
			usr, _ := user.Current()
			buildDir = filepath.Join(usr.HomeDir, "unrealircd")
			if version != "" {
				buildDir = filepath.Join(usr.HomeDir, "unrealircd-"+version)
			}
		}

		config := &Config{
			SourceDir: selectedDir,
			BuildDir:  buildDir,
			Version:   version,
		}
		err = saveConfig(config)
		if err != nil {
			errorModal := tview.NewModal().
				SetText(fmt.Sprintf("Error saving config: %v", err)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("save_config_error_modal")
				})
			pages.AddPage("save_config_error_modal", errorModal, true, true)
			return
		}

		successModal := tview.NewModal().
			SetText(fmt.Sprintf("Switched to UnrealIRCd %s\n\nSource: %s\nBuild: %s", version, selectedDir, buildDir)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("switch_success_modal")
				// Recreate main menu with new directories
				pages.RemovePage("main_menu")
				mainMenuPage(app, pages, selectedDir, buildDir)
				pages.SwitchToPage("main_menu")
			})
		pages.AddPage("switch_success_modal", successModal, true, true)
	})

	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.SwitchToPage("main_menu")
	})

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	flex.AddItem(list, 0, 1, true)
	flex.AddItem(backBtn, 3, 0, false)
	pages.AddPage("switch_install", flex, true, true)
}

func obbyScriptSubmenuPage(app *tview.Application, pages *tview.Pages, sourceDir, buildDir string) {
	// Text view on right for descriptions
	textView := &FocusableTextView{tview.NewTextView()}
	textView.SetBorder(true).SetTitle("Description")
	textView.SetDynamicColors(true)
	textView.SetWordWrap(true)
	textView.SetScrollable(true)

	// Descriptions for ObbyScript submenu
	descriptions := map[string]string{
		"• Browse GitHub Scripts": `Browse and install ObbyScript (.us/.usl) files from Valware's GitHub repository.

Features:
• View script contents with syntax highlighting
• One-click installation to your scripts directory
• Automatic configuration updates to scripts.conf
• Scripts are loaded automatically on server rehash

ObbyScript allows you to extend IRC server functionality with custom event handlers, commands, and automation.`,
		"• View Installed Scripts": `Manage your currently installed scripts and modules.

Features:
• View and edit script contents with built-in editor
• Syntax highlighting for .us/.usl files
• Uninstall scripts you no longer need
• Preview highlighted code before editing
• Automatic cleanup of configuration files

Keep your IRC server organized and up-to-date.`,
		"• Uninstall ObbyScript": `Completely uninstall ObbyScript from your server.

This will remove all script files, unload configurations, and clean up the installation.

Use this when you want to stop using scripts entirely.`}

	list := tview.NewList()
	list.SetBorder(true).SetBorderColor(tcell.ColorGreen)
	list.SetTitle("ObbyScript Menu")
	list.AddItem("• Browse GitHub Scripts", "  Browse and install ObbyScript (.us/.usl) files", 0, nil)
	list.AddItem("• View Installed Scripts", "  Manage your currently installed scripts and modules", 0, nil)
	list.AddItem("• Uninstall ObbyScript", "  Completely uninstall ObbyScript from your server", 0, nil)

	currentList = list

	header := createHeader()

	var lastClickTime time.Time
	var lastClickIndex = -1

	list.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		if desc, ok := descriptions[mainText]; ok {
			textView.SetText(desc)
		}
		now := time.Now()
		if index == lastClickIndex && now.Sub(lastClickTime) < 300*time.Millisecond {
			// Double-click detected
			switch mainText {
			case "• Browse GitHub Scripts":
				githubBrowserPage(app, pages, buildDir)
			case "• View Installed Scripts":
				installedScriptsPage(app, pages, buildDir)
			case "• Uninstall ObbyScript":
				uninstallObbyScript(app, pages, buildDir, sourceDir)
			}
		}
		lastClickIndex = index
		lastClickTime = now
	})

	list.SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		// For Enter key
		switch mainText {
		case "• Browse GitHub Scripts":
			githubBrowserPage(app, pages, buildDir)
		case "• View Installed Scripts":
			installedScriptsPage(app, pages, buildDir)
		case "• Uninstall ObbyScript":
			uninstallObbyScript(app, pages, buildDir, sourceDir)
		}
	})

	list.SetInputCapture(nil) // Remove custom input capture

	// Set initial description
	if len(descriptions) > 0 {
		textView.SetText(descriptions["• Browse GitHub Scripts"])
	}

	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.RemovePage("obby_script_submenu")
		pages.SwitchToPage("main_menu")
	})

	buttonBar := createButtonBar(backBtn)

	// Layout
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	browserFlex := tview.NewFlex().
		AddItem(list, 40, 0, true).
		AddItem(textView, 0, 1, false)
	flex.AddItem(header, 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(createFooter("ESC: Back | Enter: Select | q: Quit"), 3, 0, false)
	pages.AddPage("obby_script_submenu", flex, true, true)
	obbyScriptSubmenuFocusables = []tview.Primitive{list, textView, backBtn}
}

func moduleManagerSubmenuPage(app *tview.Application, pages *tview.Pages, sourceDir, buildDir string) {
	// Text view on right for descriptions
	textView := &FocusableTextView{tview.NewTextView()}
	textView.SetBorder(true).SetTitle("Description")
	textView.SetDynamicColors(true)
	textView.SetWordWrap(true)
	textView.SetScrollable(true)

	// Descriptions for Module Manager submenu
	descriptions := map[string]string{
		"• Browse UnrealIRCd Third-Party Modules (C)": `Download and install third-party C modules from multiple sources.

Features:
• Browse modules from official UnrealIRCd repository
• Support for custom module sources via modules.sources.list
• Automatic compilation and installation
• Post-install instructions and rehash prompts
• Module details including version, author, and documentation

Extend your IRC server with powerful compiled modules for enhanced functionality.`,
		"• Check Installed Modules": `Check the status of installed and loaded modules.

Features:
• Scan all configuration files for loaded modules
• Check modules directory for installed .so files
• Display comprehensive status: installed vs loaded
• Exclude default modules for clarity
• Helps manage and troubleshoot module configurations

Get a clear overview of your server's module setup.`,
		"• Upload Custom Module": `Upload and install a custom C module.

Features:
• Paste your module source code
• Automatic filename detection from module header
• Save to src/modules/third/ directory
• Ready for compilation and installation

Install your own custom modules directly into the source tree.`}

	list := tview.NewList()
	list.SetBorder(true).SetBorderColor(tcell.ColorGreen)
	list.SetTitle("Module Manager")
	list.AddItem("• Browse UnrealIRCd Third-Party Modules (C)", "  Download and install third-party C modules", 0, nil)
	list.AddItem("• Check Installed Modules", "  Check the status of installed and loaded modules", 0, nil)
	list.AddItem("• Upload Custom Module", "  Upload and install a custom C module", 0, nil)

	currentList = list

	header := createHeader()

	var lastClickTime time.Time
	var lastClickIndex = -1

	list.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		if desc, ok := descriptions[mainText]; ok {
			textView.SetText(desc)
		}
		now := time.Now()
		if index == lastClickIndex && now.Sub(lastClickTime) < 300*time.Millisecond {
			// Double-click detected
			switch mainText {
			case "• Browse UnrealIRCd Third-Party Modules (C)":
				thirdPartyBrowserPage(app, pages, sourceDir, buildDir)
			case "• Check Installed Modules":
				checkModulesPage(app, pages, buildDir, sourceDir)
			case "• Upload Custom Module":
				uploadCustomModulePage(app, pages, sourceDir)
			}
		}
		lastClickIndex = index
		lastClickTime = now
	})

	list.SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		// For Enter key
		switch mainText {
		case "• Browse UnrealIRCd Third-Party Modules (C)":
			thirdPartyBrowserPage(app, pages, sourceDir, buildDir)
		case "• Check Installed Modules":
			checkModulesPage(app, pages, buildDir, sourceDir)
		case "• Upload Custom Module":
			uploadCustomModulePage(app, pages, sourceDir)
		}
	})

	list.SetInputCapture(nil) // Remove custom input capture

	// Set initial description
	if len(descriptions) > 0 {
		textView.SetText(descriptions["• Browse UnrealIRCd Third-Party Modules (C)"])
	}

	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.RemovePage("module_manager_submenu")
		pages.SwitchToPage("main_menu")
	})

	buttonBar := createButtonBar(backBtn)

	// Layout
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	browserFlex := tview.NewFlex().
		AddItem(list, 40, 0, true).
		AddItem(textView, 0, 1, false)
	flex.AddItem(header, 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(createFooter("ESC: Back | Enter: Select | q: Quit"), 3, 0, false)
	pages.AddPage("module_manager_submenu", flex, true, true)
	moduleManagerSubmenuFocusables = []tview.Primitive{list, textView, backBtn}
}

func uploadCustomModulePage(app *tview.Application, pages *tview.Pages, sourceDir string) {
	textArea := tview.NewTextArea()
	textArea.SetBorder(true).SetTitle("Paste your module source code here")
	textArea.SetPlaceholder("Paste your C module code here...")

	header := createHeader()

	saveBtn := tview.NewButton("Save Module").SetSelectedFunc(func() {
		code := textArea.GetText()
		if code == "" {
			modal := tview.NewModal().
				SetText("Please paste some module code first.").
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("error_modal")
				})
			pages.AddPage("error_modal", modal, true, true)
			return
		}

		// Parse the module name from the header
		lines := strings.Split(code, "\n")
		var moduleName string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, `"third/`) && strings.Contains(line, `"`) {
				// Extract the name after "third/"
				start := strings.Index(line, `"third/`)
				if start != -1 {
					start += 7 // length of "third/"
					end := strings.Index(line[start:], `"`)
					if end != -1 {
						moduleName = line[start : start+end]
						break
					}
				}
			}
		}

		if moduleName == "" {
			modal := tview.NewModal().
				SetText("Could not find module name in header. Make sure it contains 'third/module_name' in the ModuleHeader.").
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("error_modal")
				})
			pages.AddPage("error_modal", modal, true, true)
			return
		}

		// Create the third directory if it doesn't exist
		thirdDir := filepath.Join(sourceDir, "src", "modules", "third")
		if err := os.MkdirAll(thirdDir, 0755); err != nil {
			modal := tview.NewModal().
				SetText(fmt.Sprintf("Failed to create directory: %v", err)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("error_modal")
				})
			pages.AddPage("error_modal", modal, true, true)
			return
		}

		// Save the file
		fileName := moduleName + ".c"
		filePath := filepath.Join(thirdDir, fileName)
		if err := os.WriteFile(filePath, []byte(code), 0644); err != nil {
			modal := tview.NewModal().
				SetText(fmt.Sprintf("Failed to save module: %v", err)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("error_modal")
				})
			pages.AddPage("error_modal", modal, true, true)
			return
		}

		modal := tview.NewModal().
			SetText(fmt.Sprintf("Module saved as %s\n\nYou can now compile it with:\nmake && make install", filePath)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("success_modal")
			})
		pages.AddPage("success_modal", modal, true, true)
	})

	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.RemovePage("upload_custom_module")
		pages.SwitchToPage("module_manager_submenu")
	})

	buttonBar := createButtonBar(backBtn, saveBtn)

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	flex.AddItem(header, 3, 0, false).
		AddItem(textArea, 0, 1, true).
		AddItem(buttonBar, 3, 0, false).
		AddItem(createFooter("ESC: Back | Ctrl+S: Save | q: Quit"), 3, 0, false)

	pages.AddPage("upload_custom_module", flex, true, true)
	app.SetFocus(textArea)
}

func githubBrowserPage(app *tview.Application, pages *tview.Pages, buildDir string) {
	confFile := filepath.Join(buildDir, "conf", "unrealircd.conf")
	confContent, err := os.ReadFile(confFile)
	if err == nil {
		confStr := string(confContent)
		if !strings.Contains(confStr, `include "scripts.conf"`) {
			confirmModal := tview.NewModal().
				SetText(`The include for scripts.conf is missing from unrealircd.conf. Add it?`).
				AddButtons([]string{"Yes", "No"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					if buttonLabel == "Yes" {
						newContent := `include "scripts.conf";` + "\n" + string(confContent)
						os.WriteFile(confFile, []byte(newContent), 0644)
						githubBrowserPage(app, pages, buildDir) // Retry
					}
					pages.RemovePage("scripts_include_check_modal")
				})
			pages.AddPage("scripts_include_check_modal", confirmModal, true, true)
			return
		}
	}

	// GitHub repo details
	owner := "unrealircd"
	repo := "unrealircd-contrib"
	path := "files"
	ref := "unreal6"

	// Fetch repo contents
	items, err := fetchRepoContents(owner, repo, path, ref)
	if err != nil {
		// Show error
		return
	}

	// Filter for files only
	var files []GitHubItem
	for _, item := range items {
		if item.Type == "file" {
			files = append(files, item)
		}
	}

	// Map for caching contents
	contentCache := make(map[string]string)

	// List on left
	list := tview.NewList()
	for _, file := range files {
		list.AddItem(file.Name, "", 0, nil)
	}
	list.SetBorder(true).SetTitle("GitHub Scripts")

	currentList = list

	// Text view on right
	textView := &FocusableTextView{tview.NewTextView()}
	textView.SetBorder(true).SetTitle("Content")
	textView.SetDynamicColors(true)
	textView.SetWordWrap(true)
	textView.SetScrollable(true)

	// Set initial content if any files
	if len(files) > 0 {
		go func() {
			content, err := fetchFileContent(files[0].DownloadURL)
			if err != nil {
				content = fmt.Sprintf("Error loading content: %v", err)
			} else {
				if strings.HasSuffix(files[0].Name, ".us") || strings.HasSuffix(files[0].Name, ".usl") {
					content = highlightUSL(content)
				}
			}
			contentCache[files[0].Name] = content
			app.QueueUpdateDraw(func() {
				textView.SetText(content)
			})
		}()
	}

	// Handle selection
	list.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		if content, cached := contentCache[mainText]; cached {
			textView.SetText(content)
		} else {
			textView.SetText("Loading...")
			go func() {
				for _, file := range files {
					if file.Name == mainText {
						content, err := fetchFileContent(file.DownloadURL)
						if err != nil {
							content = fmt.Sprintf("Error loading content: %v", err)
						} else {
							if strings.HasSuffix(file.Name, ".us") || strings.HasSuffix(file.Name, ".usl") {
								content = highlightUSL(content)
							}
						}
						contentCache[mainText] = content
						app.QueueUpdateDraw(func() {
							textView.SetText(content)
						})
						break
					}
				}
			}()
		}
	})

	// Layout
	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.SwitchToPage("main_menu")
	})
	installBtn := tview.NewButton("Install Selected").SetSelectedFunc(func() {
		index := list.GetCurrentItem()
		if index >= 0 && index < len(files) {
			file := files[index]
			go func() {
				app.QueueUpdateDraw(func() {
					textView.SetText("Installing...")
				})
				err := installScript(buildDir, file.DownloadURL, file.Name)
				if err != nil {
					app.QueueUpdateDraw(func() {
						textView.SetText(fmt.Sprintf("Installation failed: %v", err))
					})
				} else {
					app.QueueUpdateDraw(func() {
						textView.SetText("Script installed successfully!")
					})
				}
			}()
		}
	})
	buttonBar := createButtonBar(backBtn, installBtn)

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	browserFlex := tview.NewFlex().
		AddItem(list, 40, 0, true).
		AddItem(textView, 0, 1, false)
	flex.AddItem(createHeader(), 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(createFooter("ESC: Main Menu | Enter: Select | q: Quit"), 3, 0, false)
	pages.AddPage("github_browser", flex, true, true)
	githubBrowserFocusables = []tview.Primitive{list, textView, backBtn, installBtn}
}

func thirdPartyBrowserPage(app *tview.Application, pages *tview.Pages, sourceDir, buildDir string) {
	confFile := filepath.Join(buildDir, "conf", "unrealircd.conf")
	confContent, err := os.ReadFile(confFile)
	if err == nil {
		confStr := string(confContent)
		if !strings.Contains(confStr, `include "mods.conf"`) {
			confirmModal := tview.NewModal().
				SetText(`The include for mods.conf is missing from unrealircd.conf. Add it?`).
				AddButtons([]string{"Yes", "No"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					if buttonLabel == "Yes" {
						newContent := `include "mods.conf";` + "\n" + string(confContent)
						os.WriteFile(confFile, []byte(newContent), 0644)
						thirdPartyBrowserPage(app, pages, sourceDir, buildDir) // Retry
					}
					pages.RemovePage("mods_include_check_modal")
				})
			pages.AddPage("mods_include_check_modal", confirmModal, true, true)
			return
		}
	}

	// List on left
	list := tview.NewList()
	list.SetBorder(true).SetTitle("Third-Party Modules")

	currentList = list

	// Text view on right
	textView := &FocusableTextView{tview.NewTextView()}
	textView.SetBorder(true).SetTitle("Module Details")
	textView.SetDynamicColors(true)
	textView.SetWordWrap(true)
	textView.SetScrollable(true)
	textView.SetText("Loading modules...")

	// Get sources URLs
	var allModules []Module
	urls, err := parseModulesSources()
	if err != nil {
		textView.SetText(fmt.Sprintf("Error reading modules sources: %v", err))
	} else {
		for _, url := range urls {
			content, err := fetchFileContent(url)
			if err != nil {
				// Skip this source
				continue
			}
			modules, err := parseModulesList(content)
			if err != nil {
				// Skip
				continue
			}
			allModules = append(allModules, modules...)
		}
		if len(allModules) == 0 {
			textView.SetText("No modules found in any sources.")
		} else {
			for _, mod := range allModules {
				list.AddItem(mod.Name, mod.Description, 0, nil)
			}
			textView.SetText(formatModuleDetails(allModules[0]))
		}
	}

	// Handle selection
	list.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		if index >= 0 && index < len(allModules) {
			textView.SetText(formatModuleDetails(allModules[index]))
		}
	})

	// Layout
	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.SwitchToPage("main_menu")
	})
	installBtn := tview.NewButton("Install Selected").SetSelectedFunc(func() {
		index := list.GetCurrentItem()
		if index >= 0 && index < len(allModules) {
			mod := allModules[index]
			// Show loading modal
			installModal := tview.NewModal().
				SetText("Installing module... Please wait.").
				AddButtons([]string{}).
				SetDoneFunc(func(int, string) {})
			pages.AddPage("install_modal", installModal, true, true)
			go func() {
				moduleName := strings.TrimPrefix(mod.Name, "third/")
				filename := moduleName + ".c"
				err := installModule(sourceDir, buildDir, mod.Source, filename)
				app.QueueUpdateDraw(func() {
					pages.RemovePage("install_modal")
					if err != nil {
						textView.SetText(fmt.Sprintf("Installation failed: %v", err))
					} else {
						details := formatModuleDetails(mod)
						if len(mod.PostInstallText) > 0 {
							details += "\n\n[green]Post-Install Instructions:[white]\n" + strings.Join(mod.PostInstallText, "\n")
						}
						textView.SetText(details + "\n\n[green]Module installed successfully![white]")
						// Show rehash modal
						rehashModal := tview.NewModal().
							SetText("Module installed successfully! Rehash the server?").
							AddButtons([]string{"Yes", "No"}).
							SetDoneFunc(func(buttonIndex int, buttonLabel string) {
								if buttonLabel == "Yes" {
									go func() {
										app.QueueUpdateDraw(func() {
											textView.SetText("Rehashing...")
										})
										err := rehash(buildDir)
										app.QueueUpdateDraw(func() {
											if err != nil {
												textView.SetText(fmt.Sprintf("Rehash failed: %v", err))
											} else {
												textView.SetText("Server rehashed successfully!")
											}
										})
									}()
								}
								pages.RemovePage("rehash_modal")
							})
						pages.AddPage("rehash_modal", rehashModal, true, true)
					}
				})
			}()
		}
	})
	buttonBar := createButtonBar(backBtn, installBtn)

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	browserFlex := tview.NewFlex().
		AddItem(list, 80, 0, true).
		AddItem(textView, 0, 1, false)
	flex.AddItem(createHeader(), 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(createFooter("ESC: Main Menu | Enter: Select | q: Quit"), 3, 0, false)
	pages.AddPage("third_party_browser", flex, true, true)
	thirdPartyBrowserFocusables = []tview.Primitive{list, textView, backBtn, installBtn}
}

func installedScriptsPage(app *tview.Application, pages *tview.Pages, buildDir string) {
	scripts, err := getInstalledScripts(buildDir)
	if err != nil {
		// Show error
		return
	}

	list := tview.NewList()
	for _, script := range scripts {
		list.AddItem(script, "", 0, nil)
	}
	list.SetBorder(true).SetTitle("Installed Scripts")

	currentList = list

	textView := &FocusableTextView{tview.NewTextView()}
	textView.SetBorder(true).SetTitle("Content")
	textView.SetDynamicColors(true)
	textView.SetWordWrap(true)
	textView.SetScrollable(true)

	// Handle selection
	list.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		scriptPath := filepath.Join(buildDir, "scripts", mainText)
		content, err := os.ReadFile(scriptPath)
		if err != nil {
			textView.SetText(fmt.Sprintf("Error reading script: %v", err))
		} else {
			contentStr := string(content)
			if strings.HasSuffix(mainText, ".us") || strings.HasSuffix(mainText, ".usl") {
				contentStr = highlightUSL(contentStr)
			}
			textView.SetText(contentStr)
		}
	})

	// Display first script content if available
	if len(scripts) > 0 {
		scriptPath := filepath.Join(buildDir, "scripts", scripts[0])
		content, err := os.ReadFile(scriptPath)
		if err != nil {
			textView.SetText(fmt.Sprintf("Error reading script: %v", err))
		} else {
			contentStr := string(content)
			if strings.HasSuffix(scripts[0], ".us") || strings.HasSuffix(scripts[0], ".usl") {
				contentStr = highlightUSL(contentStr)
			}
			textView.SetText(contentStr)
		}
	}

	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.SwitchToPage("main_menu")
	})
	uninstallBtn := tview.NewButton("Uninstall Selected").SetSelectedFunc(func() {
		index := list.GetCurrentItem()
		if index >= 0 && index < len(scripts) {
			scriptName := scripts[index]
			confirmModal := tview.NewModal().
				SetText(fmt.Sprintf("Really uninstall '%s'?", scriptName)).
				AddButtons([]string{"Yes", "No"}).
				SetDoneFunc(func(confIndex int, confLabel string) {
					if confLabel == "Yes" {
						err := uninstallScript(buildDir, scriptName)
						if err != nil {
							errorModal := tview.NewModal().
								SetText(fmt.Sprintf("Error uninstalling: %v", err)).
								AddButtons([]string{"OK"}).
								SetDoneFunc(func(int, string) {
									pages.RemovePage("error_modal")
								})
							pages.AddPage("error_modal", errorModal, true, true)
						} else {
							pages.RemovePage("installed_scripts")
							installedScriptsPage(app, pages, buildDir)
						}
					}
					pages.RemovePage("confirm_modal")
				})
			pages.AddPage("confirm_modal", confirmModal, true, true)
		}
	})
	editBtn := tview.NewButton("Edit Selected").SetSelectedFunc(func() {
		index := list.GetCurrentItem()
		if index >= 0 && index < len(scripts) {
			scriptName := scripts[index]
			editScriptPage(app, pages, buildDir, scriptName)
		}
	})
	buttonBar := createButtonBar(backBtn, editBtn, uninstallBtn)

	// Layout
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	scriptsFlex := tview.NewFlex().
		AddItem(list, 40, 0, true).
		AddItem(textView, 0, 1, false)
	flex.AddItem(createHeader(), 3, 0, false).AddItem(scriptsFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(createFooter("ESC: Main Menu | Enter: Select | q: Quit"), 3, 0, false)
	pages.AddPage("installed_scripts", flex, true, true)
	installedScriptsFocusables = []tview.Primitive{list, textView, backBtn, editBtn, uninstallBtn}
}

func editScriptPage(app *tview.Application, pages *tview.Pages, buildDir, scriptName string) {
	currentList = nil
	scriptPath := filepath.Join(buildDir, "scripts", scriptName)
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Error reading script: %v", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("error_modal")
			})
		pages.AddPage("error_modal", errorModal, true, true)
		return
	}

	textArea := tview.NewTextArea()
	textArea.SetBorder(true).SetTitle(fmt.Sprintf("Editing %s", scriptName))
	textArea.SetText(string(content), false)

	saved := true
	textArea.SetChangedFunc(func() {
		if saved {
			textArea.SetTitle(fmt.Sprintf("Editing %s (Unsaved)", scriptName))
			saved = false
		}
	})

	textArea.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlS {
			newContent := textArea.GetText()
			err := os.WriteFile(scriptPath, []byte(newContent), 0644)
			if err != nil {
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error saving: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("error_modal")
					})
				pages.AddPage("error_modal", errorModal, true, true)
			} else {
				saved = true
				textArea.SetTitle(fmt.Sprintf("Editing %s (Saved)", scriptName))
			}
			return nil
		}
		if event.Key() == tcell.KeyCtrlX {
			if !saved {
				confirmModal := tview.NewModal().
					SetText("You have unsaved changes. Really exit without saving?").
					AddButtons([]string{"No", "Yes"}).
					SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						if buttonLabel == "Yes" {
							pages.RemovePage("edit_script")
						}
						pages.RemovePage("confirm_exit")
					})
				pages.AddPage("confirm_exit", confirmModal, true, true)
			} else {
				pages.RemovePage("edit_script")
			}
			return nil
		}
		return event
	})

	saveBtn := tview.NewButton("Save (Ctrl+S)").SetSelectedFunc(func() {
		newContent := textArea.GetText()
		err := os.WriteFile(scriptPath, []byte(newContent), 0644)
		if err != nil {
			errorModal := tview.NewModal().
				SetText(fmt.Sprintf("Error saving: %v", err)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("error_modal")
				})
			pages.AddPage("error_modal", errorModal, true, true)
		} else {
			saved = true
			textArea.SetTitle(fmt.Sprintf("Editing %s (Saved)", scriptName))
		}
	})
	cancelBtn := tview.NewButton("Cancel (Ctrl+X)").SetSelectedFunc(func() {
		if !saved {
			confirmModal := tview.NewModal().
				SetText("You have unsaved changes. Really exit without saving?").
				AddButtons([]string{"No", "Yes"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					if buttonLabel == "Yes" {
						pages.RemovePage("edit_script")
					}
					pages.RemovePage("confirm_exit")
				})
			pages.AddPage("confirm_exit", confirmModal, true, true)
		} else {
			pages.RemovePage("edit_script")
		}
	})

	previewBtn := tview.NewButton("Preview Highlighted").SetSelectedFunc(func() {
		content := textArea.GetText()
		if strings.HasSuffix(scriptName, ".us") || strings.HasSuffix(scriptName, ".usl") {
			content = highlightUSL(content)
		}
		previewView := tview.NewTextView()
		previewView.SetBorder(true).SetTitle(fmt.Sprintf("Preview: %s", scriptName))
		previewView.SetDynamicColors(true)
		previewView.SetWordWrap(true)
		previewView.SetScrollable(true)
		previewView.SetText(content)
		previewFlex := tview.NewFlex().SetDirection(tview.FlexRow)
		previewFlex.AddItem(previewView, 0, 1, true)
		closeBtn := tview.NewButton("Close").SetSelectedFunc(func() {
			pages.RemovePage("preview_modal")
		})
		previewFlex.AddItem(closeBtn, 3, 0, false)
		pages.AddPage("preview_modal", previewFlex, true, true)
	})

	buttonBar := createButtonBar(saveBtn, previewBtn, cancelBtn)

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	flex.AddItem(createHeader(), 3, 0, false).AddItem(textArea, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(createFooter("Ctrl+S: Save | Ctrl+X: Cancel | ESC: Cancel"), 3, 0, false)
	pages.AddPage("edit_script", flex, true, true)
	editScriptFocusables = []tview.Primitive{textArea, saveBtn, previewBtn, cancelBtn}
	app.SetFocus(textArea)
}