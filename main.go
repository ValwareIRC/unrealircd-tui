package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/net/html"
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

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var currentList *tview.List

var mainMenuFocusables []tview.Primitive
var githubBrowserFocusables []tview.Primitive
var installedScriptsFocusables []tview.Primitive
var thirdPartyBrowserFocusables []tview.Primitive
var editScriptFocusables []tview.Primitive
var checkModulesFocusables []tview.Primitive
var obbyScriptSubmenuFocusables []tview.Primitive
var moduleManagerSubmenuFocusables []tview.Primitive
var utilitiesFocusables []tview.Primitive

var installationTips = []string{
	"The IRCOp guide shows how to do everyday IRCOp tasks and contains tips on fighting spam and drones.\n\nhttps://www.unrealircd.org/docs/IRCOp_guide",
	"You can use a SSL/TLS certificate fingerprint instead of passwords in places like Oper Blocks and Link Blocks.",
	"You can use the /SAJOIN command to force a user to join a channel. Bans and other restrictions will be bypassed.",
	"Log files can use JSON logging. You can also send the JSON data to IRCOps on IRC.\n\nThe JSON is machine readable and contains lots of details about every log event.",
	"What is shown in WHOIS can be configured in detail via set::whois-details.",
	"UnrealIRCd 6 uses GeoIP by default. It is shown in WHOIS but also available as country in mask items,\n\nfor example it can be used in the TLD Block to serve a Spanish MOTD to people in Spanish speaking countries.",
	"Almost every channel mode can be disabled. Don't like halfops? Use blacklist-module chanmodes/halfop;",
	"If you run multiple servers then consider using Remote includes to share configuration settings.",
	"To upgrade UnrealIRCd on *NIX simply run: ./unrealircd upgrade",
	"You can use a SSL/TLS certificate fingerprints to exempt trusted users from server bans or allow them to send more commands per second.",
	"Use set::restrict-commands to prevent new users from executing certain commands like LIST. Useful against drones/spam.",
	"Channel mode +P makes a channel permanent. The topic and modes are preserved,\n\neven if all users leave the channel, and even if the server is restarted thanks to channeldb.",
	"If you don't want to receive private messages, set user mode +D. You can also force it on all users.\n\nOr, if you only want to allow private messages from people who are identified to Services then set +R.",
	"Don't like snomasks / server notices? Then configure logging to a channel.",
	"You can add a Webhook that is called on certain log events.\n\nThis can be used to automate things or to notify you in case of trouble.",
	"Consider contributing to make UnrealIRCd even better: reporting bugs, testing, helping out with support, ..",
	"On IRC you can use the HELPOP command to read about various IRC commands.",
	"Exempt your IP address from bans, just in case you or a fellow IRCOp accidentally GLINES you.",
	"If you still have users on plaintext port 6667, consider enabling Strict Transport Security to gently move users to SSL/TLS on port 6697.",
	"The Security article gives hands-on tips on how to deal with drone attacks, flooding, spammers, (D)DoS and more.",
	"Check out Special users on how to give trusted users/bots more rights without making them IRCOp.",
	"With the UnrealIRCd administration panel you can add and remove server bans and do other server management from your browser.",
	"If you want to bypass access checks for channels as an IRCOp, use SAMODE or SAJOIN. Or use OperOverride.",
	"You can exempt users dynamically from server bans, spamfilter, maxperip and other restrictions with the ELINE command on IRC.",
	"The blacklist { } block can be used to ban known troublemakers that are listed in blacklists like EfnetRBL and DroneBL.",
	"Channel mode +H provides Channel history to modern clients. Optionally, it can be stored on-disk to be preserved between server restarts.",
	"Channel anti-flood protection is on by default (since UnrealIRCd 6.2.0). You can override the default profile via +F.",
	"Connthrottle will limit the damage from big drone attacks. Check if the flood thresholds and exceptions are OK for your network.",
	"Did you know that users are put in the security-group known-users based on their reputation score or if they are identified to Services?\n\nUsers in this group receive a number of benefits, such as being able to send more messages per minute.",
	"The antirandom module can be a useful tool to block clients with random looking nicks.",
	"You can allow trusted users to send more messages per second without having to make them IRCOp. Especially useful for bots.",
}

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
	flex.AddItem(createHeader(), 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(ui.CreateFooter("ESC: Main Menu | q: Quit"), 3, 0, false)
	pages.AddPage("check_modules", flex, true, true)
	checkModulesFocusables = []tview.Primitive{list, textView, loadUnloadBtn, uninstallBtn, deleteBtn, backBtn}
}

const (
	configFile = ".unrealircd_tui_config"

	// Menu item constants
	MenuBrowseScripts       = "Browse GitHub Scripts (ObbyScript)"
	MenuViewInstalled       = "View Installed Scripts (ObbyScript)"
	MenuBrowseModules       = "Browse UnrealIRCd Third-Party Modules (C)"
	MenuCheckModules        = "Check Installed Modules"
	MenuUninstallObbyScript = "Uninstall ObbyScript"
	MenuRemoteControl       = "Remote Control (RPC)"
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

func generateRandomHexString(length int) string {
	bytes := make([]byte, length/2+1) // +1 to ensure we have enough bytes
	if _, err := rand.Read(bytes); err != nil {
		// Fallback if crypto/rand fails
		return strings.Repeat("a", length)
	}
	return hex.EncodeToString(bytes)[:length]
}

func buildAndInstall(sourceDir string, updateFunc func(string)) error {
	// Build
	updateFunc("Running 'make'...")
	cmd := exec.Command("make")
	cmd.Dir = sourceDir

	// Capture stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// Read output in real-time
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			updateFunc("[make] " + line)
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			updateFunc("[make] " + line)
		}
	}()

	if err := cmd.Wait(); err != nil {
		return err
	}

	// Install
	updateFunc("Running 'make install'...")
	cmd = exec.Command("make", "install")
	cmd.Dir = sourceDir

	stdout, err = cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err = cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// Read output in real-time
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			updateFunc("[make install] " + line)
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			updateFunc("[make install] " + line)
		}
	}()

	return cmd.Wait()
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
	text       []rune
	cursor     int
	title      string
	changed    func()
	saveFunc   func()
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
	Name                 string
	Description          string
	Version              string
	Author               string
	Troubleshooting      string
	Source               string
	Sha256sum            string
	LastUpdated          string
	MinUnrealircdVersion string
	MaxUnrealircdVersion string
	PostInstallText      []string
}

func fetchRepoContents(owner, repo, path, ref string) ([]GitHubItem, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", owner, repo, path, ref)
	resp, err := makeHTTPRequest(url)
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
	resp, err := makeHTTPRequest(downloadURL)
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

// InstallOptions holds the configuration for automated installation
type InstallOptions struct {
	NicknameHistory string
	GeoIP           string
	BasePath        string
	DefPerm         string
	SSLDir          string
	RemoteInc       string
	MaxConnections  string
	Sanitizer       string
	ExtraPara       string
}

// parseInstallOptions parses command line arguments for installation options
func parseInstallOptions(args []string) *InstallOptions {
	opts := &InstallOptions{
		NicknameHistory: "2000",
		GeoIP:           "classic",
		DefPerm:         "0600",
		SSLDir:          "",
		RemoteInc:       "1",
		MaxConnections:  "auto",
		Sanitizer:       "",
		ExtraPara:       "",
	}

	// Set default base path
	usr, _ := user.Current()
	opts.BasePath = filepath.Join(usr.HomeDir, "unrealircd")

	for _, arg := range args {
		if strings.HasPrefix(arg, "--nickname-history=") {
			opts.NicknameHistory = strings.TrimPrefix(arg, "--nickname-history=")
		} else if strings.HasPrefix(arg, "--geoip=") {
			geoip := strings.TrimPrefix(arg, "--geoip=")
			if geoip == "classic" || geoip == "libmaxminddb" || geoip == "none" {
				opts.GeoIP = geoip
			}
		} else if strings.HasPrefix(arg, "--basepath=") {
			opts.BasePath = strings.TrimPrefix(arg, "--basepath=")
		} else if strings.HasPrefix(arg, "--defperm=") {
			opts.DefPerm = strings.TrimPrefix(arg, "--defperm=")
		} else if strings.HasPrefix(arg, "--ssldir=") {
			opts.SSLDir = strings.TrimPrefix(arg, "--ssldir=")
		} else if strings.HasPrefix(arg, "--remoteinc=") {
			remoteinc := strings.TrimPrefix(arg, "--remoteinc=")
			if remoteinc == "0" || remoteinc == "1" || remoteinc == "2" {
				opts.RemoteInc = remoteinc
			}
		} else if strings.HasPrefix(arg, "--maxconnections=") {
			opts.MaxConnections = strings.TrimPrefix(arg, "--maxconnections=")
		} else if strings.HasPrefix(arg, "--sanitizer=") {
			sanitizer := strings.TrimPrefix(arg, "--sanitizer=")
			if sanitizer == "0" || sanitizer == "1" {
				opts.Sanitizer = sanitizer
			}
		} else if strings.HasPrefix(arg, "--extrapara=") {
			opts.ExtraPara = strings.TrimPrefix(arg, "--extrapara=")
		}
	}

	return opts
}

// runAutomatedInstall performs the complete installation process from command line
func runAutomatedInstall(opts *InstallOptions) error {
	fmt.Println("Starting automated UnrealIRCd installation...")
	fmt.Printf("Installation directory: %s\n", opts.BasePath)
	fmt.Printf("Nickname history: %s\n", opts.NicknameHistory)
	fmt.Printf("GeoIP engine: %s\n", opts.GeoIP)
	fmt.Printf("Permissions: %s\n", opts.DefPerm)

	// Step 1: Download latest version
	fmt.Println("\nStep 1: Checking for latest UnrealIRCd version...")
	version, downloadURL, err := getLatestVersion()
	if err != nil {
		return fmt.Errorf("failed to get latest version: %w", err)
	}
	fmt.Printf("Latest version: %s\n", version)
	fmt.Printf("Download URL: %s\n", downloadURL)

	// Step 2: Create source directory
	sourceDir := filepath.Join(os.TempDir(), "unrealircd-source-"+version)
	fmt.Printf("\nStep 2: Creating source directory: %s\n", sourceDir)
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		return fmt.Errorf("failed to create source directory: %w", err)
	}
	defer os.RemoveAll(sourceDir) // Clean up on error

	// Step 3: Download and extract
	fmt.Println("\nStep 3: Downloading and extracting...")
	err = downloadAndExtractFileCLI(downloadURL, sourceDir)
	if err != nil {
		return fmt.Errorf("failed to download and extract: %w", err)
	}

	// Find the actual source directory (tar.gz usually has a top-level directory)
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return fmt.Errorf("failed to read extracted directory: %w", err)
	}
	var actualSourceDir string
	for _, entry := range entries {
		if entry.IsDir() && strings.Contains(entry.Name(), "unrealircd") {
			actualSourceDir = filepath.Join(sourceDir, entry.Name())
			break
		}
	}
	if actualSourceDir == "" {
		return fmt.Errorf("could not find UnrealIRCd source directory in extracted files")
	}
	fmt.Printf("Found source directory: %s\n", actualSourceDir)

	// Step 4: Save config.settings
	fmt.Println("\nStep 4: Creating configuration...")
	err = saveConfigSettings(actualSourceDir, opts.BasePath, opts.DefPerm, opts.SSLDir, opts.RemoteInc, opts.NicknameHistory, opts.GeoIP, opts.MaxConnections, opts.Sanitizer, opts.ExtraPara)
	if err != nil {
		return fmt.Errorf("failed to save config.settings: %w", err)
	}

	// Step 5: Run ./Config -quick
	fmt.Println("\nStep 5: Running configuration...")
	err = runConfigQuick(actualSourceDir)
	if err != nil {
		return fmt.Errorf("failed to run ./Config -quick: %w", err)
	}

	// Step 6: Build
	fmt.Println("\nStep 6: Building UnrealIRCd...")
	err = runMake(actualSourceDir)
	if err != nil {
		return fmt.Errorf("failed to build: %w", err)
	}

	// Step 7: Install
	fmt.Println("\nStep 7: Installing...")
	err = runMakeInstall(actualSourceDir, opts.BasePath)
	if err != nil {
		return fmt.Errorf("failed to install: %w", err)
	}

	// Step 8: Setup config
	fmt.Println("\nStep 8: Setting up configuration...")
	err = setupConfigFile(opts.BasePath)
	if err != nil {
		return fmt.Errorf("failed to setup config: %w", err)
	}

	// Step 9: Save our config
	fmt.Println("\nStep 9: Saving installation configuration...")
	config := &Config{
		SourceDir: actualSourceDir,
		BuildDir:  opts.BasePath,
		Version:   version,
	}
	err = saveConfig(config)
	if err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("\nInstallation completed successfully!\n")
	fmt.Printf("UnrealIRCd %s installed to: %s\n", version, opts.BasePath)
	fmt.Printf("Configuration files are in: %s/conf/\n", opts.BasePath)
	fmt.Printf("You can start the server with: %s/bin/unrealircd\n", opts.BasePath)

	return nil
}

// getLatestVersion fetches the latest UnrealIRCd version
func getLatestVersion() (string, string, error) {
	resp, err := http.Get("https://www.unrealircd.org/downloads/list.json")
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	var updateResp UpdateResponse
	if err := json.NewDecoder(resp.Body).Decode(&updateResp); err != nil {
		return "", "", err
	}

	// Find the latest stable version across all branches
	var latestVersion string
	var latestURL string
	for _, versions := range updateResp {
		if stable, ok := versions["Stable"]; ok && stable.Downloads.Src != "" {
			if latestVersion == "" || compareVersions(stable.Version, latestVersion) > 0 {
				latestVersion = stable.Version
				latestURL = stable.Downloads.Src
			}
		}
	}

	if latestVersion == "" {
		return "", "", fmt.Errorf("no stable version found")
	}

	return latestVersion, latestURL, nil
}

// downloadAndExtractFileCLI downloads and extracts a file (CLI version)
func downloadAndExtractFileCLI(url, destDir string) error {
	// Create temp file
	tempFile, err := os.CreateTemp("", "unrealircd-*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Download
	fmt.Printf("Downloading from %s...\n", url)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	_, err = io.Copy(tempFile, resp.Body)
	if err != nil {
		return err
	}

	// Extract
	fmt.Printf("Extracting to %s...\n", destDir)
	return extractTarGz(tempFile.Name(), destDir)
}

// runConfigQuick runs ./Config -quick
func runConfigQuick(sourceDir string) error {
	cmd := exec.Command("./Config", "-quick")
	cmd.Dir = sourceDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runMake runs make
func runMake(sourceDir string) error {
	cmd := exec.Command("make")
	cmd.Dir = sourceDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runMakeInstall runs make install
func runMakeInstall(sourceDir, installDir string) error {
	cmd := exec.Command("make", "install")
	cmd.Dir = sourceDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	// Check for command-line arguments
	if len(os.Args) > 1 {
		if len(os.Args) == 3 && os.Args[1] == "--dev-test-fleet" {
			numStr := os.Args[2]
			numServers, err := strconv.Atoi(numStr)
			if err != nil || numServers < 2 || numServers > 1000 {
				fmt.Fprintf(os.Stderr, "Error: Invalid number of servers. Must be between 2 and 1000.\n")
				fmt.Fprintf(os.Stderr, "Usage: %s --dev-test-fleet <number>\n", os.Args[0])
				os.Exit(1)
			}
			// Run test fleet creation in CLI mode
			runTestFleetCLI(numServers)
			return
		} else if os.Args[1] == "--install-latest-unrealircd" {
			// Parse installation options
			installOpts := parseInstallOptions(os.Args[2:])
			err := runAutomatedInstall(installOpts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Installation failed: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Installation completed successfully!")
			return
		} else {
			fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
			fmt.Fprintf(os.Stderr, "\nGUI Mode:\n")
			fmt.Fprintf(os.Stderr, "  (no arguments)  Start the interactive GUI\n")
			fmt.Fprintf(os.Stderr, "\nCLI Mode:\n")
			fmt.Fprintf(os.Stderr, "  --dev-test-fleet <number>  Create a test fleet with N servers (2-1000)\n")
			fmt.Fprintf(os.Stderr, "  --install-latest-unrealircd [options]  Install latest UnrealIRCd automatically\n")
			fmt.Fprintf(os.Stderr, "\nInstallation Options:\n")
			fmt.Fprintf(os.Stderr, "  --nickname-history=<num>     Nickname history length (default: 2000)\n")
			fmt.Fprintf(os.Stderr, "  --geoip=<classic|libmaxminddb|none>  GeoIP engine (default: classic)\n")
			fmt.Fprintf(os.Stderr, "  --basepath=<path>            Installation directory (default: ~/unrealircd)\n")
			fmt.Fprintf(os.Stderr, "  --defperm=<perms>            Default permissions (default: 0600)\n")
			fmt.Fprintf(os.Stderr, "  --ssldir=<path>              OpenSSL/LibreSSL directory (default: auto-detect)\n")
			fmt.Fprintf(os.Stderr, "  --remoteinc=<0|1|2>          Remote includes support (default: 1)\n")
			fmt.Fprintf(os.Stderr, "  --maxconnections=<num|auto>  Maximum connections (default: auto)\n")
			fmt.Fprintf(os.Stderr, "  --sanitizer=<0|1>            Enable sanitizers (default: 0)\n")
			fmt.Fprintf(os.Stderr, "  --extrapara=<params>         Extra configure parameters\n")
			os.Exit(1)
		}
	}

	// Continue with normal GUI mode
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
			sourceDirs = []string{} // Continue with empty list so user can install
		}
		if len(sourceDirs) == 0 {
			// No source dirs found — show main menu with prompt to install
			usr, _ := user.Current()
			sourceDir = ""
			buildDir = filepath.Join(usr.HomeDir, "unrealircd")
			mainMenuPage(app, pages, sourceDir, buildDir)
			// Show a welcome modal prompting installation
			welcomeModal := tview.NewModal().
				SetText("No UnrealIRCd installation detected.\n\nWould you like to set one up now?").
				AddButtons([]string{"Yes", "No"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					pages.RemovePage("welcome_modal")
					if buttonLabel == "Yes" {
						setupNewInstallPage(app, pages)
					}
				})
			pages.AddPage("welcome_modal", welcomeModal, true, true)
		} else if len(sourceDirs) == 1 {
			sourceDir = sourceDirs[0]
			version, err := getUnrealIRCdVersion(sourceDir)
			if err != nil {
				// Handle error, perhaps show message or set empty
				version = ""
			}
			usr, _ := user.Current()
			buildDir = filepath.Join(usr.HomeDir, "unrealircd")
			config = &Config{SourceDir: sourceDir, BuildDir: buildDir, Version: version}
			saveConfig(config)
			mainMenuPage(app, pages, sourceDir, buildDir)
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
				config = &Config{SourceDir: sourceDir, BuildDir: buildDir, Version: version}
				saveConfig(config)
				mainMenuPage(app, pages, sourceDir, buildDir)
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
			case "utilities":
				focusables = utilitiesFocusables
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
	contentFlex.AddItem(createHeader(), 3, 0, false).AddItem(list, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(ui.CreateFooter("Enter: Select | q: Quit"), 3, 0, false)

	// Auto-size height based on content
	contentHeight := len(sourceDirs) + 8 // items + title + buttons + footer + padding
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
	// Check if build directory exists and prompt for deletion
	if _, err := os.Stat(buildDir); err == nil {
		confirmModal := tview.NewModal().
			SetText(fmt.Sprintf("Build directory '%s' already exists. Delete it?", buildDir)).
			AddButtons([]string{"No", "Yes"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				if buttonLabel == "Yes" {
					err := os.RemoveAll(buildDir)
					if err != nil {
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Error deleting build directory: %v", err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("error_modal")
							})
						pages.AddPage("error_modal", errorModal, true, true)
						return
					}
				}
				// Start installation
				startInstallation(app, pages, sourceDir, buildDir)
			})
		pages.AddPage("build_confirm", confirmModal, true, true)
		return
	}

	// Start installation
	startInstallation(app, pages, sourceDir, buildDir)
}

func startInstallation(app *tview.Application, pages *tview.Pages, sourceDir, buildDir string) {
	currentList = nil
	textView := tview.NewTextView()
	textView.SetBorder(true).SetTitle("Installation Progress")
	textView.SetDynamicColors(true)
	textView.SetWordWrap(true)
	textView.SetScrollable(true)

	// Create tip display
	tipView := tview.NewTextView()
	tipView.SetBorder(true).SetTitle("Tips and Tricks")
	tipView.SetDynamicColors(true)
	tipView.SetWordWrap(true)
	tipView.SetScrollable(true)

	// Start tip rotation
	go func() {
		tipIndex := 0
		for {
			app.QueueUpdateDraw(func() {
				tipView.SetText(installationTips[tipIndex])
			})
			tipIndex = (tipIndex + 1) % len(installationTips)
			time.Sleep(20 * time.Second)
		}
	}()

	// Create cancel button
	cancelBtn := tview.NewButton("Cancel").SetSelectedFunc(func() {
		confirmModal := tview.NewModal().
			SetText("Cancel installation? This will stop the current process.").
			AddButtons([]string{"No", "Yes"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				if buttonLabel == "Yes" {
					pages.RemovePage("install")
					mainMenuPage(app, pages, "", "")
				}
				pages.RemovePage("cancel_confirm")
			})
		pages.AddPage("cancel_confirm", confirmModal, true, true)
	})

	go func() {
		update := func(msg string) {
			app.QueueUpdateDraw(func() {
				fmt.Fprintf(textView, "%s\n", msg)
				textView.ScrollToEnd()
			})
		}

		update("Starting installation...")
		if err := buildAndInstall(sourceDir, update); err != nil {
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

	// Create a centered modal-like layout for the build output
	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView(), 0, 1, false). // Top spacer
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(tview.NewTextView(), 0, 1, false). // Left spacer
			AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(createHeader(), 3, 0, false).
				AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
					AddItem(textView, 0, 2, true). // Installation progress takes 2/3
					AddItem(tipView, 0, 1, false), // Tip takes 1/3
					0, 1, false). // Equal height for progress and tip
				AddItem(createButtonBar(cancelBtn), 3, 0, false),
									120, 0, false). // Even wider for both views
			AddItem(tview.NewTextView(), 0, 1, false), // Right spacer
								0, 1, false).
		AddItem(tview.NewTextView(), 0, 1, false) // Bottom spacer

	pages.AddPage("install", flex, true, true)
	app.SetFocus(textView) // Ensure textView gets focus immediately
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
		"• Services Package": `Install and configure IRC services packages.

Features:
• Install Anope IRC Services (NickServ, ChanServ, etc.)
• Install Atheme IRC Services (alternative services package)
• Automatic download, compilation, and configuration
• Link services to UnrealIRCd automatically
• Configure UnrealIRCd for services integration

Add professional services to your IRC network with one-click installation.`,
		"• Configuration": `Browse and preview configuration files.

Features:
• View all configuration files and folders
• Preview file contents directly in the interface
• Navigate through configuration directory structure
• Quick access to UnrealIRCd configuration files

Easily manage and review your server configuration.`,
		"• Installation Options": `Manage UnrealIRCd installations.

Features:
• Set up new UnrealIRCd installations
• Switch between existing installations
• Uninstall and remove installations
• Manage multiple UnrealIRCd versions

Complete installation management for your IRC server.`,
		// "• ObbyScript": `Manage ObbyScript installation and scripts.

		// Features:
		// • Browse and install scripts from GitHub
		// • View and edit installed scripts
		// • Uninstall ObbyScript completely
		// • Automatic configuration management
		// • Syntax highlighting and code preview

		// Extend your IRC server functionality with custom scripts and automation.`,
		"• Dev Tools": `Developer tools and utilities.

Features:
• Run tests and diagnostics
• Access development resources
• Debug and troubleshooting tools
• Development utilities and helpers

Tools for developers working with UnrealIRCd.`,
		"• Utilities": `Execute UnrealIRCd command-line utilities.

Features:
• Run ./unrealircd commands like rehash, mkpasswd, upgrade
• View command output in real-time
• Execute commands with Enter key (not mouse click)
• Access server management utilities

Direct access to UnrealIRCd's command-line interface.`}

	list := tview.NewList()
	list.SetBorder(true).SetBorderColor(tcell.ColorGreen)
	list.AddItem("• Configuration", "  Browse and preview configuration files", 0, nil)
	list.AddItem("• Utilities", "  Execute UnrealIRCd command-line utilities", 0, nil)
	list.AddItem("• Module Manager", "  Manage UnrealIRCd C modules", 0, nil)
	list.AddItem("• Check for Updates", "  Check for available UnrealIRCd updates", 0, nil)
	list.AddItem("• Installation Options", "  Manage UnrealIRCd installations", 0, nil)
	list.AddItem("• Remote Control (RPC)", "  Control UnrealIRCd server via JSON-RPC API", 0, nil)
	list.AddItem("• Services Package", "  Install and configure IRC services packages", 0, nil)
	// list.AddItem("• ObbyScript", "  Manage ObbyScript installation and scripts", 0, nil)
	list.AddItem("• Dev Tools", "  Developer tools and utilities", 0, nil)

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
			case "• Configuration":
				ui.ConfigurationMenuPage(app, pages, buildDir)
			case "• Utilities":
				utilitiesPage(app, pages, buildDir)
			case "• Module Manager":
				moduleManagerSubmenuPage(app, pages, sourceDir, buildDir)
			case "• Check for Updates":
				checkForUpdatesPage(app, pages, sourceDir, buildDir)
			case "• Installation Options":
				installationOptionsPage(app, pages, sourceDir, buildDir)
			case "• Remote Control (RPC)":
				ui.RemoteControlMenuPage(app, pages, buildDir)
			case "• Services Package":
				servicesPackageSubmenuPage(app, pages, sourceDir, buildDir)
			// case "• ObbyScript":
			// 	obbyScriptSubmenuPage(app, pages, sourceDir, buildDir)
			case "• Dev Tools":
				devToolsSubmenuPage(app, pages, sourceDir, buildDir)
			}
		}
		lastClickIndex = index
		lastClickTime = now
	})

	list.SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		// For Enter key
		switch mainText {
		case "• Configuration":
			ui.ConfigurationMenuPage(app, pages, buildDir)
		case "• Utilities":
			utilitiesPage(app, pages, buildDir)
		case "• Module Manager":
			moduleManagerSubmenuPage(app, pages, sourceDir, buildDir)
		case "• Check for Updates":
			checkForUpdatesPage(app, pages, sourceDir, buildDir)
		case "• Installation Options":
			installationOptionsPage(app, pages, sourceDir, buildDir)
		case "• Remote Control (RPC)":
			ui.RemoteControlMenuPage(app, pages, buildDir)
		case "• Services Package":
			servicesPackageSubmenuPage(app, pages, sourceDir, buildDir)
		// case "• ObbyScript":
		// 	obbyScriptSubmenuPage(app, pages, sourceDir, buildDir)
		case "• Dev Tools":
			devToolsSubmenuPage(app, pages, sourceDir, buildDir)
		}
	})

	list.SetInputCapture(nil) // Remove custom input capture

	// Set initial description
	if len(descriptions) > 0 {
		textView.SetText(descriptions["• Configuration"])
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
	flex.AddItem(header, 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(ui.CreateFooter("ESC: Back | Enter: Select | q: Quit"), 3, 0, false)
	pages.AddPage("main_menu", flex, true, true)
	mainMenuFocusables = []tview.Primitive{list, textView, quitBtn}
}

func utilitiesPage(app *tview.Application, pages *tview.Pages, buildDir string) {
	// List of utilities on the left
	list := tview.NewList()
	list.SetBorder(true)
	list.SetTitle("UnrealIRCd Utilities")
	list.SetBorderColor(tcell.ColorGreen)

	// Output text view on the right
	outputView := tview.NewTextView()
	outputView.SetBorder(true)
	outputView.SetTitle("Command Output")
	outputView.SetDynamicColors(true)
	outputView.SetWordWrap(true)
	outputView.SetScrollable(true)

	// Descriptions for utilities
	descriptions := map[string]string{
		"configtest":   "Test the configuration file for syntax errors and validity.\n\nThis command checks if your unrealircd.conf and other configuration files are properly formatted and contain no errors before starting the server.",
		"start":        "Start the IRC Server.\n\nLaunches the UnrealIRCd daemon. Make sure the configuration is tested first with configtest.",
		"stop":         "Stop (kill) the IRC Server.\n\nGracefully shuts down the running UnrealIRCd process. All users will be disconnected.",
		"rehash":       "Reload the configuration file.\n\nReloads the configuration without restarting the server. Useful for applying configuration changes while the server is running.",
		"reloadtls":    "Reload the SSL/TLS certificates.\n\nReloads SSL/TLS certificates and keys without restarting the server. Useful when certificates have been renewed.",
		"restart":      "Restart the IRC Server (stop+start).\n\nStops the server and starts it again. All users will be disconnected during the restart.",
		"status":       "Show current status of the IRC Server.\n\nDisplays information about whether the server is running, PID, uptime, and basic statistics.",
		"module-status": "Show all currently loaded modules.\n\nLists all modules that are currently loaded in the running server, including core and third-party modules.",
		"version":      "Display the UnrealIRCd version.\n\nShows the version number, build date, and other version information of the installed UnrealIRCd.",
		"genlinkblock": "Generate link { } block for the other side.\n\nCreates a sample link block configuration that can be used to connect to another IRC server.",
		"gencloak":     "Display 3 random cloak keys.\n\nGenerates random cloak keys that can be used in the configuration for hostname cloaking.",
		"spkifp":       "Display SPKI Fingerprint.\n\nShows the SPKI (Subject Public Key Info) fingerprint of the server's SSL/TLS certificate.",
	}

	// Add utility commands
	utilities := []string{
		"configtest",
		"start",
		"stop",
		"rehash",
		"reloadtls",
		"restart",
		"status",
		"module-status",
		"version",
		"genlinkblock",
		"gencloak",
		"spkifp",
	}
	for _, util := range utilities {
		// Get short description from the map or default
		shortDesc := "Execute " + util
		if desc, ok := descriptions[util]; ok {
			// Take first line or shorten
			lines := strings.Split(desc, "\n")
			if len(lines) > 0 {
				shortDesc = lines[0]
			}
		}
		list.AddItem(util, shortDesc, 0, nil)
	}

	currentList = list

	// Update description when selection changes
	list.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		if desc, ok := descriptions[mainText]; ok {
			outputView.SetText(desc)
		} else {
			outputView.SetText("No description available.")
		}
	})

	// Handle Enter key press to execute command (not on click)
	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter {
			index := list.GetCurrentItem()
			if index >= 0 && index < len(utilities) {
				command := utilities[index]
				// Clear previous output
				outputView.Clear()
				// Execute the command
				go func() {
					cmd := exec.Command("./unrealircd", command)
					cmd.Dir = buildDir
					output, err := cmd.CombinedOutput()
					app.QueueUpdateDraw(func() {
						if err != nil {
							fmt.Fprintf(outputView, "Error executing %s:\n%s\n\n%s", command, err.Error(), string(output))
						} else {
							fmt.Fprintf(outputView, "Output of ./unrealircd %s:\n\n%s", command, string(output))
						}
					})
				}()
			}
			return nil // Consume the event
		}
		return event
	})

	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.SwitchToPage("main_menu")
	})

	runBtn := tview.NewButton("Run").SetSelectedFunc(func() {
		index := list.GetCurrentItem()
		if index >= 0 && index < len(utilities) {
			command := utilities[index]
			// Clear previous output
			outputView.Clear()
			// Execute the command
			go func() {
				cmd := exec.Command("./unrealircd", command)
				cmd.Dir = buildDir
				output, err := cmd.CombinedOutput()
				app.QueueUpdateDraw(func() {
					if err != nil {
						fmt.Fprintf(outputView, "Error executing %s:\n%s\n\n%s", command, err.Error(), string(output))
					} else {
						fmt.Fprintf(outputView, "Output of ./unrealircd %s:\n\n%s", command, string(output))
					}
				})
			}()
		}
	})

	buttonBar := createButtonBar(backBtn, runBtn)

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	browserFlex := tview.NewFlex().
		AddItem(list, 40, 0, true).
		AddItem(outputView, 0, 1, false)
	flex.AddItem(createHeader(), 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(ui.CreateFooter("ESC: Main Menu | Enter: Execute Command | q: Quit"), 3, 0, false)
	pages.AddPage("utilities", flex, true, true)
	utilitiesFocusables = []tview.Primitive{list, outputView, backBtn, runBtn}
	app.SetFocus(list)

	// Set initial description
	if len(utilities) > 0 {
		if desc, ok := descriptions[utilities[0]]; ok {
			outputView.SetText(desc)
		}
	}
}

func htmlToText(htmlContent string) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return htmlContent // fallback to original if parsing fails
	}

	var text strings.Builder
	var extractText func(*html.Node, bool)
	extractText = func(n *html.Node, inPre bool) {
		if n.Type == html.TextNode {
			if inPre {
				text.WriteString(n.Data)
			} else {
				// Normalize whitespace
				data := strings.ReplaceAll(n.Data, "\t", " ")
				data = regexp.MustCompile(`\s+`).ReplaceAllString(data, " ")
				text.WriteString(data)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extractText(c, inPre || (n.Type == html.ElementNode && n.Data == "pre"))
		}
		// Add formatting for block elements
		if n.Type == html.ElementNode {
			switch n.Data {
			case "p":
				text.WriteString("\n\n")
			case "div":
				text.WriteString("\n")
			case "br":
				text.WriteString("\n")
			case "h1", "h2":
				text.WriteString("\n\n")
			case "h3", "h4", "h5", "h6":
				text.WriteString("\n")
			case "li":
				text.WriteString("\n• ")
			case "tr":
				text.WriteString("\n")
			case "td", "th":
				text.WriteString(" | ")
			}
		}
	}

	extractText(doc, false)

	// Clean up the text
	result := text.String()
	// Remove excessive whitespace and empty lines
	result = regexp.MustCompile(`\n{3,}`).ReplaceAllString(result, "\n\n")
	result = regexp.MustCompile(`^\n+`).ReplaceAllString(result, "")
	result = regexp.MustCompile(`\n+$`).ReplaceAllString(result, "")
	result = strings.TrimSpace(result)
	
	// Remove excessive spaces
	result = regexp.MustCompile(`  +`).ReplaceAllString(result, " ")
	
	return result
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
	// Check if source directory exists and prompt for deletion
	if _, err := os.Stat(sourceDir); err == nil {
		confirmModal := tview.NewModal().
			SetText(fmt.Sprintf("Source directory '%s' already exists. Delete it?", sourceDir)).
			AddButtons([]string{"No", "Yes"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				if buttonLabel == "Yes" {
					err := os.RemoveAll(sourceDir)
					if err != nil {
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Error deleting source directory: %v", err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("error_modal")
							})
						pages.AddPage("error_modal", errorModal, true, true)
						return
					}
				}
				// Start the actual download
				startDownloadAndExtract(app, pages, version, downloadURL, sourceDir)
			})
		pages.AddPage("source_confirm", confirmModal, true, true)
		return
	}

	// Start the actual download
	startDownloadAndExtract(app, pages, version, downloadURL, sourceDir)
}

func startDownloadAndExtract(app *tview.Application, pages *tview.Pages, version, downloadURL, sourceDir string) {
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
CURLDIR=""
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
	// Start installation
	continueInstallationAfterChecks(app, pages, sourceDir, version, buildDir)
}

func continueInstallationAfterChecks(app *tview.Application, pages *tview.Pages, sourceDir, version, buildDir string) {
	textView := tview.NewTextView()
	textView.SetBorder(true).SetTitle(fmt.Sprintf("Installing UnrealIRCd %s", version))
	textView.SetDynamicColors(true)
	textView.SetWordWrap(true)
	textView.SetScrollable(true)

	// Create tip display
	tipView := tview.NewTextView()
	tipView.SetBorder(true).SetTitle("Tip of the Day")
	tipView.SetDynamicColors(true)
	tipView.SetWordWrap(true)
	tipView.SetScrollable(true)

	// Start tip rotation
	go func() {
		tipIndex := 0
		for {
			app.QueueUpdateDraw(func() {
				tipView.SetText(installationTips[tipIndex])
			})
			tipIndex = (tipIndex + 1) % len(installationTips)
			time.Sleep(20 * time.Second)
		}
	}()

	// Create cancel button
	cancelBtn := tview.NewButton("Cancel").SetSelectedFunc(func() {
		confirmModal := tview.NewModal().
			SetText("Cancel installation? This will stop the current process.").
			AddButtons([]string{"No", "Yes"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				if buttonLabel == "Yes" {
					pages.RemovePage("install")
					mainMenuPage(app, pages, "", "")
				}
				pages.RemovePage("cancel_confirm")
			})
		pages.AddPage("cancel_confirm", confirmModal, true, true)
	})

	go func() {
		update := func(msg string) {
			app.QueueUpdateDraw(func() {
				fmt.Fprintf(textView, "%s\n", msg)
				textView.ScrollToEnd()
			})
		}

		update("Running ./Config -quick...")
		// Run ./Config -quick to apply the config.settings
		configCmd := exec.Command("./Config", "-quick")
		configCmd.Dir = sourceDir

		// Capture output
		configStdout, err := configCmd.StdoutPipe()
		if err != nil {
			update(fmt.Sprintf("Error setting up config pipe: %v", err))
			return
		}
		configStderr, err := configCmd.StderrPipe()
		if err != nil {
			update(fmt.Sprintf("Error setting up config pipe: %v", err))
			return
		}

		if err := configCmd.Start(); err != nil {
			update(fmt.Sprintf("Error starting ./Config: %v", err))
			return
		}

		// Read config output
		go func() {
			scanner := bufio.NewScanner(configStdout)
			for scanner.Scan() {
				line := scanner.Text()
				update(fmt.Sprintf("[Config] %s", line))
			}
		}()
		go func() {
			scanner := bufio.NewScanner(configStderr)
			for scanner.Scan() {
				line := scanner.Text()
				update(fmt.Sprintf("[Config] %s", line))
			}
		}()

		if err := configCmd.Wait(); err != nil {
			update(fmt.Sprintf("Error running ./Config -quick: %v", err))
			return
		}

		update("Running make...")
		// Run make with real-time output
		makeCmd := exec.Command("make")
		makeCmd.Dir = sourceDir

		makeStdout, err := makeCmd.StdoutPipe()
		if err != nil {
			update(fmt.Sprintf("Error setting up make pipe: %v", err))
			return
		}
		makeStderr, err := makeCmd.StderrPipe()
		if err != nil {
			update(fmt.Sprintf("Error setting up make pipe: %v", err))
			return
		}

		if err := makeCmd.Start(); err != nil {
			update(fmt.Sprintf("Error starting make: %v", err))
			return
		}

		// Read make output
		go func() {
			scanner := bufio.NewScanner(makeStdout)
			for scanner.Scan() {
				line := scanner.Text()
				update(fmt.Sprintf("[make] %s", line))
			}
		}()
		go func() {
			scanner := bufio.NewScanner(makeStderr)
			for scanner.Scan() {
				line := scanner.Text()
				update(fmt.Sprintf("[make] %s", line))
			}
		}()

		if err := makeCmd.Wait(); err != nil {
			update(fmt.Sprintf("Error running make: %v", err))
			return
		}

		update("Running make install...")
		// Run make install with real-time output
		installCmd := exec.Command("make", "install")
		installCmd.Dir = sourceDir

		installStdout, err := installCmd.StdoutPipe()
		if err != nil {
			update(fmt.Sprintf("Error setting up install pipe: %v", err))
			return
		}
		installStderr, err := installCmd.StderrPipe()
		if err != nil {
			update(fmt.Sprintf("Error setting up install pipe: %v", err))
			return
		}

		if err := installCmd.Start(); err != nil {
			update(fmt.Sprintf("Error starting make install: %v", err))
			return
		}

		// Read install output
		go func() {
			scanner := bufio.NewScanner(installStdout)
			for scanner.Scan() {
				line := scanner.Text()
				update(fmt.Sprintf("[make install] %s", line))
			}
		}()
		go func() {
			scanner := bufio.NewScanner(installStderr)
			for scanner.Scan() {
				line := scanner.Text()
				update(fmt.Sprintf("[make install] %s", line))
			}
		}()

		if err := installCmd.Wait(); err != nil {
			update(fmt.Sprintf("Error running make install: %v", err))
			return
		}

		update("Setting up configuration...")
		// Set up config
		err = setupConfigFile(buildDir)
		if err != nil {
			update(fmt.Sprintf("Error setting up config file: %v", err))
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
			update(fmt.Sprintf("Error saving config: %v", err))
			return
		}

		update("Installation complete!")
		app.QueueUpdateDraw(func() {
			pages.RemovePage("install")
			mainMenuPage(app, pages, sourceDir, buildDir)
		})
	}()

	// Create a centered modal-like layout for the build output
	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView(), 0, 1, false). // Top spacer
		AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(tview.NewTextView(), 0, 1, false). // Left spacer
			AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
				AddItem(createHeader(), 3, 0, false).
				AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
					AddItem(textView, 0, 2, true). // Installation progress takes 2/3
					AddItem(tipView, 0, 1, false), // Tip takes 1/3
					0, 1, false). // Equal height for progress and tip
				AddItem(createButtonBar(cancelBtn), 3, 0, false),
									120, 0, false). // Even wider for both views
			AddItem(tview.NewTextView(), 0, 1, false), // Right spacer
								0, 1, false).
		AddItem(tview.NewTextView(), 0, 1, false) // Bottom spacer

	pages.AddPage("install", flex, true, true)
	app.SetFocus(textView) // Ensure textView gets focus immediately
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
	// Ensure destination directory exists
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}

	// Use system tar command for reliable extraction
	cmd := exec.Command("tar", "-zxvf", tarGzPath, "-C", destDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar extraction failed: %w\nOutput: %s", err, string(output))
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
	flex.AddItem(header, 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(ui.CreateFooter("ESC: Back | Enter: Select | q: Quit"), 3, 0, false)
	pages.AddPage("obby_script_submenu", flex, true, true)
	obbyScriptSubmenuFocusables = []tview.Primitive{list, textView, backBtn}
}

func devToolsSubmenuPage(app *tview.Application, pages *tview.Pages, sourceDir, buildDir string) {
	// Text view on right for descriptions
	textView := &FocusableTextView{tview.NewTextView()}
	textView.SetBorder(true).SetTitle("Description")
	textView.SetDynamicColors(true)
	textView.SetWordWrap(true)
	textView.SetScrollable(true)

	// Descriptions for Dev Tools submenu
	descriptions := map[string]string{
		"• Tests": `Run tests and diagnostics.

Features:
• Execute unit tests and integration tests
• Run diagnostic checks on installations
• Validate configuration files
• Test module loading and functionality
• Performance and health checks
• Test Fleet - Create multiple linked UnrealIRCd instances

Comprehensive testing suite for UnrealIRCd installations.`,
		"• Resources": `Access development resources and documentation.

Features:
• View UnrealIRCd documentation and guides
• Access API references and specifications
• Browse development tools and utilities
• View configuration examples and templates
• Access community resources and support

Development resources and documentation for UnrealIRCd.`}

	list := tview.NewList()
	list.SetBorder(true).SetBorderColor(tcell.ColorGreen)
	list.SetTitle("Dev Tools")
	list.AddItem("• Tests", "  Run tests and diagnostics", 0, nil)
	list.AddItem("• Resources", "  Access development resources and documentation", 0, nil)

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
			case "• Tests":
				testsSubmenuPage(app, pages, sourceDir, buildDir)
			case "• Resources":
				// TODO: Implement resources page
			}
		}
		lastClickIndex = index
		lastClickTime = now
	})

	list.SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		// For Enter key
		switch mainText {
		case "• Tests":
			testsSubmenuPage(app, pages, sourceDir, buildDir)
		case "• Resources":
			// TODO: Implement resources page
		}
	})

	list.SetInputCapture(nil) // Remove custom input capture

	// Set initial description
	if len(descriptions) > 0 {
		textView.SetText(descriptions["• Tests"])
	}

	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.RemovePage("dev_tools_submenu")
		pages.SwitchToPage("main_menu")
	})

	buttonBar := createButtonBar(backBtn)

	// Layout
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	browserFlex := tview.NewFlex().
		AddItem(list, 40, 0, true).
		AddItem(textView, 0, 1, false)
	flex.AddItem(header, 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(ui.CreateFooter("ESC: Back | Enter: Select | q: Quit"), 3, 0, false)
	pages.AddPage("dev_tools_submenu", flex, true, true)
	app.SetFocus(list)
}

func testsSubmenuPage(app *tview.Application, pages *tview.Pages, sourceDir, buildDir string) {
	// Text view on right for descriptions
	textView := &FocusableTextView{tview.NewTextView()}
	textView.SetBorder(true).SetTitle("Description")
	textView.SetDynamicColors(true)
	textView.SetWordWrap(true)
	textView.SetScrollable(true)

	// Descriptions for Tests submenu
	descriptions := map[string]string{
		"• Test Fleet": `Create a test fleet of linked UnrealIRCd servers.

Features:
• Choose number of servers (2-50)
• Automatic download of latest UnrealIRCd
• Dynamic installation to separate directories
• Automatic configuration with unique server names
• Link block generation and application
• Spanning tree topology (not mesh)

Create a test network of interconnected UnrealIRCd servers for testing purposes.`,
		"• Manage Fleet": `Start and stop test fleet servers.

Features:
• Scan for existing test fleets
• Start/stop individual servers
• Start/stop entire fleets
• View server status
• Monitor running processes

Control your test fleet servers with ease.`}

	list := tview.NewList()
	list.SetBorder(true).SetBorderColor(tcell.ColorYellow)
	list.SetTitle("Tests")
	list.AddItem("• Test Fleet", "  Create a test fleet of linked UnrealIRCd servers", 0, nil)
	list.AddItem("• Manage Fleet", "  Start and stop test fleet servers", 0, nil)

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
			case "• Test Fleet":
				testFleetPage(app, pages)
			case "• Manage Fleet":
				manageFleetPage(app, pages)
			}
		}
		lastClickIndex = index
		lastClickTime = now
	})

	list.SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		// For Enter key
		switch mainText {
		case "• Test Fleet":
			testFleetPage(app, pages)
		case "• Manage Fleet":
			manageFleetPage(app, pages)
		}
	})

	list.SetInputCapture(nil) // Remove custom input capture

	// Set initial description
	if len(descriptions) > 0 {
		textView.SetText(descriptions["• Test Fleet"])
	}

	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.RemovePage("tests_submenu")
		pages.SwitchToPage("dev_tools_submenu")
	})

	buttonBar := createButtonBar(backBtn)

	// Layout
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	// Top section: main menu and description
	topFlex := tview.NewFlex().
		AddItem(list, 40, 0, true).
		AddItem(textView, 0, 1, false)

	flex.AddItem(header, 3, 0, false).
		AddItem(topFlex, 0, 1, true).
		AddItem(buttonBar, 3, 0, false).
		AddItem(ui.CreateFooter("ESC: Back | Enter: Select | q: Quit"), 3, 0, false)
	pages.AddPage("tests_submenu", flex, true, true)
	app.SetFocus(list)
}

func testFleetPage(app *tview.Application, pages *tview.Pages) {
	// Create a form for number input
	form := tview.NewForm()
	form.SetBorder(true).SetTitle("Test Fleet Setup")
	form.SetBorderColor(tcell.ColorYellow)

	// Add number input field and buttons
	form.AddInputField("Number of servers (2-1000)", "2", 10, func(text string, ch rune) bool {
		// Only allow digits
		return (ch >= '0' && ch <= '9') || ch == 0
	}, nil).
		AddButton("Create Fleet", func() {
			numStr := form.GetFormItem(0).(*tview.InputField).GetText()
			numServers, err := strconv.Atoi(numStr)
			if err != nil || numServers < 2 || numServers > 1000 {
				errorModal := tview.NewModal().
					SetText("Please enter a valid number between 2 and 1000.").
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("fleet_error_modal")
					})
				pages.AddPage("fleet_error_modal", errorModal, true, true)
				return
			}

			// Start fleet creation process
			createTestFleet(app, pages, numServers)
		}).
		AddButton("Cancel", func() {
			pages.RemovePage("test_fleet")
			pages.SwitchToPage("tests_submenu")
		})

	// Layout
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	flex.AddItem(createHeader(), 3, 0, false)
	flex.AddItem(tview.NewTextView(), 1, 0, false) // Spacer
	flex.AddItem(form, 10, 0, true)
	flex.AddItem(tview.NewTextView(), 1, 0, false) // Spacer
	flex.AddItem(ui.CreateFooter("Enter: Select | Tab: Next Field | Esc: Cancel"), 3, 0, false)

	centeredFlex := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(tview.NewTextView(), 0, 1, false).
		AddItem(flex, 100, 0, true).
		AddItem(tview.NewTextView(), 0, 1, false)

	pages.AddPage("test_fleet", centeredFlex, true, true)
	app.SetFocus(form)
}

func manageFleetPage(app *tview.Application, pages *tview.Pages) {
	// Remove any existing manage_fleet page to avoid conflicts
	pages.RemovePage("manage_fleet")

	// Scan for existing fleet directories
	fleets, err := scanForFleets()
	if err != nil {
		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Error scanning for fleets: %v", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("scan_fleets_error_modal")
			})
		pages.AddPage("scan_fleets_error_modal", errorModal, true, true)
		return
	}

	if len(fleets) == 0 {
		errorModal := tview.NewModal().
			SetText("No test fleets found. Create a test fleet first.").
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("no_fleets_modal")
				pages.RemovePage("manage_fleet")
				pages.SwitchToPage("tests_submenu")
			})
		pages.AddPage("no_fleets_modal", errorModal, true, true)
		return
	}

	// Create list of fleets
	list := tview.NewList()
	list.SetBorder(true).SetTitle("Select Fleet to Manage")
	list.SetBorderColor(tcell.ColorGreen)

	for _, fleet := range fleets {
		displayName := fmt.Sprintf("Fleet %s (%d servers)", fleet.Suffix, fleet.ServerCount)
		secondaryText := fmt.Sprintf("Source: %s | Servers: %s-1 to %s-%d", fleet.SourceDir, fleet.Suffix, fleet.Suffix, fleet.ServerCount)
		list.AddItem(displayName, secondaryText, 0, nil)
	}

	list.SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		selectedFleet := fleets[index]
		showFleetControlPage(app, pages, selectedFleet)
	})

	deleteBtn := tview.NewButton("Delete Fleet").SetSelectedFunc(func() {
		index := list.GetCurrentItem()
		if index < 0 || index >= len(fleets) {
			return
		}
		selectedFleet := fleets[index]
		confirmModal := tview.NewModal().
			SetText(fmt.Sprintf("Delete fleet '%s' (%d servers)?\n\nThis will permanently remove all fleet directories and cannot be undone!", selectedFleet.Suffix, selectedFleet.ServerCount)).
			AddButtons([]string{"No", "Yes"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				if buttonLabel == "Yes" {
					// Delete the fleet
					err := deleteFleet(selectedFleet)
					if err != nil {
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Error deleting fleet: %v", err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("error_modal")
							})
						pages.AddPage("error_modal", errorModal, true, true)
					} else {
						// Success - refresh the manage fleet page
						manageFleetPage(app, pages)
					}
				}
				pages.RemovePage("confirm_modal")
			})
		pages.AddPage("confirm_modal", confirmModal, true, true)
	})

	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.SwitchToPage("tests_submenu")
	})

	buttonBar := createButtonBar(deleteBtn, backBtn)

	// Layout
	contentFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	contentFlex.AddItem(createHeader(), 3, 0, false).AddItem(list, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(ui.CreateFooter("Enter: Select fleet | d: Delete Fleet | b: Back"), 3, 0, false)

	centeredFlex := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(tview.NewTextView(), 0, 1, false).
		AddItem(contentFlex, 100, 0, true).
		AddItem(tview.NewTextView(), 0, 1, false)

	pages.AddPage("manage_fleet", centeredFlex, true, true)
	app.SetFocus(list)
}

type FleetInfo struct {
	Suffix      string
	SourceDir   string
	BuildDirs   []string
	ServerCount int
}

func scanForFleets() ([]FleetInfo, error) {
	usr, _ := user.Current()
	baseDir := usr.HomeDir

	// Find all directories matching the fleet pattern
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}

	fleetMap := make(map[string]*FleetInfo)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()

		// Check if it's a fleet source directory (unrealircd-fleet-{suffix})
		if strings.HasPrefix(dirName, "unrealircd-fleet-") && strings.Count(dirName, "-") == 2 {
			// This is "unrealircd-fleet-{suffix}", extract suffix
			parts := strings.Split(dirName, "-")
			if len(parts) >= 3 {
				suffix := parts[2]
				if fleetMap[suffix] == nil {
					fleetMap[suffix] = &FleetInfo{
						Suffix:    suffix,
						SourceDir: filepath.Join(baseDir, dirName),
						BuildDirs: []string{},
					}
				}
			}
		}

		// Check if it's a fleet build directory (unrealircd-fleet-{suffix}-{number})
		if strings.HasPrefix(dirName, "unrealircd-fleet-") && strings.Count(dirName, "-") == 3 {
			parts := strings.Split(dirName, "-")
			if len(parts) >= 4 {
				suffix := parts[2]
				serverNumStr := parts[3]
				serverNum, err := strconv.Atoi(serverNumStr)
				if err != nil {
					continue
				}

				if fleetMap[suffix] == nil {
					fleetMap[suffix] = &FleetInfo{
						Suffix:    suffix,
						SourceDir: filepath.Join(baseDir, "unrealircd-fleet-"+suffix),
						BuildDirs: []string{},
					}
				}

				fleetMap[suffix].BuildDirs = append(fleetMap[suffix].BuildDirs, filepath.Join(baseDir, dirName))
				if serverNum > fleetMap[suffix].ServerCount {
					fleetMap[suffix].ServerCount = serverNum
				}
			}
		}
	}

	// Convert map to slice and validate fleets
	var fleets []FleetInfo
	for _, fleet := range fleetMap {
		// Only include fleets that have both source and at least one build directory
		if _, err := os.Stat(fleet.SourceDir); err == nil && len(fleet.BuildDirs) > 0 {
			// Sort build directories by server number
			sort.Slice(fleet.BuildDirs, func(i, j int) bool {
				iNum := extractServerNumber(fleet.BuildDirs[i])
				jNum := extractServerNumber(fleet.BuildDirs[j])
				return iNum < jNum
			})
			fleets = append(fleets, *fleet)
		}
	}

	return fleets, nil
}

func extractServerNumber(dirPath string) int {
	parts := strings.Split(filepath.Base(dirPath), "-")
	if len(parts) >= 4 {
		if num, err := strconv.Atoi(parts[3]); err == nil {
			return num
		}
	}
	return 0
}

func deleteFleet(fleet FleetInfo) error {
	// Delete source directory
	if err := os.RemoveAll(fleet.SourceDir); err != nil {
		return fmt.Errorf("failed to delete source directory %s: %v", fleet.SourceDir, err)
	}

	// Delete all build directories
	for _, buildDir := range fleet.BuildDirs {
		if err := os.RemoveAll(buildDir); err != nil {
			return fmt.Errorf("failed to delete build directory %s: %v", buildDir, err)
		}
	}

	return nil
}

func showFleetControlPage(app *tview.Application, pages *tview.Pages, fleet FleetInfo) {
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	// Title
	title := tview.NewTextView().
		SetText(fmt.Sprintf("Fleet Control - %s (%d servers)", fleet.Suffix, fleet.ServerCount)).
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)
	title.SetBorder(true).SetTitle("Fleet Management")

	// Server list
	list := tview.NewList().
		SetWrapAround(false).
		SetHighlightFullLine(true)

	// Add individual server controls
	for i, buildDir := range fleet.BuildDirs {
		serverNum := i + 1
		serverName := fmt.Sprintf("Server %d", serverNum)
		list.AddItem(serverName, fmt.Sprintf("Directory: %s", filepath.Base(buildDir)), rune('1'+i), func() {
			showServerControlPage(app, pages, fleet, serverNum-1)
		})
	}

	// Add bulk controls
	list.AddItem("Start All Servers", "Start all servers in the fleet", 'a', func() {
		startAllServers(app, pages, fleet)
	})
	list.AddItem("Stop All Servers", "Stop all servers in the fleet", 's', func() {
		stopAllServers(app, pages, fleet)
	})
	list.AddItem("Check Status", "Check status of all servers", 'c', func() {
		checkFleetStatus(app, pages, fleet)
	})

	// Delete fleet button
	deleteBtn := tview.NewButton("Delete Fleet").SetSelectedFunc(func() {
		confirmModal := tview.NewModal().
			SetText(fmt.Sprintf("Delete fleet '%s' (%d servers)?\n\nThis will permanently remove all fleet directories and cannot be undone!", fleet.Suffix, fleet.ServerCount)).
			AddButtons([]string{"No", "Yes"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				if buttonLabel == "Yes" {
					// Delete the fleet
					err := deleteFleet(fleet)
					if err != nil {
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Error deleting fleet: %v", err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("error_modal")
							})
						pages.AddPage("error_modal", errorModal, true, true)
					} else {
						// Success - go back to tests submenu
						pages.SwitchToPage("tests_submenu")
					}
				}
				pages.RemovePage("confirm_modal")
			})
		pages.AddPage("confirm_modal", confirmModal, true, true)
	})

	// Back button
	backBtn := tview.NewButton("Back to Fleet List").SetSelectedFunc(func() {
		pages.SwitchToPage("tests_submenu")
	})

	buttonBar := createButtonBar(deleteBtn, backBtn)

	flex.AddItem(title, 3, 1, false)
	flex.AddItem(list, 0, 1, true)
	flex.AddItem(buttonBar, 3, 1, false)

	pages.AddPage(fmt.Sprintf("fleetControl-%s", fleet.Suffix), flex, true, true)
	pages.SwitchToPage(fmt.Sprintf("fleetControl-%s", fleet.Suffix))
}

func showServerControlPage(app *tview.Application, pages *tview.Pages, fleet FleetInfo, serverIndex int) {
	if serverIndex < 0 || serverIndex >= len(fleet.BuildDirs) {
		return
	}

	buildDir := fleet.BuildDirs[serverIndex]
	serverNum := serverIndex + 1

	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	// Title
	title := tview.NewTextView().
		SetText(fmt.Sprintf("Server %d Control - Fleet %s", serverNum, fleet.Suffix)).
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true)
	title.SetBorder(true).SetTitle("Server Management")

	// Server info
	info := tview.NewTextView().
		SetText(fmt.Sprintf("Directory: %s\nFleet: %s\nServer Number: %d", filepath.Base(buildDir), fleet.Suffix, serverNum)).
		SetDynamicColors(true)
	info.SetBorder(true).SetTitle("Server Info")

	// Control buttons
	btnFlex := tview.NewFlex().SetDirection(tview.FlexColumn)

	startBtn := tview.NewButton("Start Server").SetSelectedFunc(func() {
		startServer(app, pages, buildDir, fmt.Sprintf("fleet-%s-%d", fleet.Suffix, serverNum))
	})

	stopBtn := tview.NewButton("Stop Server").SetSelectedFunc(func() {
		stopServer(app, pages, fmt.Sprintf("fleet-%s-%d", fleet.Suffix, serverNum))
	})

	statusBtn := tview.NewButton("Check Status").SetSelectedFunc(func() {
		checkServerStatus(app, pages, buildDir, fmt.Sprintf("fleet-%s-%d", fleet.Suffix, serverNum))
	})

	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.SwitchToPage(fmt.Sprintf("fleetControl-%s", fleet.Suffix))
	})

	btnFlex.AddItem(startBtn, 0, 1, true)
	btnFlex.AddItem(stopBtn, 0, 1, false)
	btnFlex.AddItem(statusBtn, 0, 1, false)
	btnFlex.AddItem(backBtn, 0, 1, false)

	flex.AddItem(title, 3, 1, false)
	flex.AddItem(info, 5, 1, false)
	flex.AddItem(btnFlex, 3, 1, false)

	pages.AddPage(fmt.Sprintf("serverControl-%s-%d", fleet.Suffix, serverNum), flex, true, true)
	pages.SwitchToPage(fmt.Sprintf("serverControl-%s-%d", fleet.Suffix, serverNum))
}

func startAllServers(app *tview.Application, pages *tview.Pages, fleet FleetInfo) {
	modal := tview.NewModal().
		SetText(fmt.Sprintf("Starting all %d servers in fleet %s...\nBuildDirs: %v", fleet.ServerCount, fleet.Suffix, fleet.BuildDirs)).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			pages.RemovePage("startingAll")
		})

	pages.AddPage("startingAll", modal, true, true)

	go func() {
		for i, buildDir := range fleet.BuildDirs {
			serverName := fmt.Sprintf("fleet-%s-%d", fleet.Suffix, i+1)
			app.QueueUpdateDraw(func() {
				modal.SetText(fmt.Sprintf("Starting server %d/%d: %s\nDir: %s", i+1, fleet.ServerCount, serverName, buildDir))
			})
			if err := startServerProcess(buildDir, serverName); err != nil {
				app.QueueUpdateDraw(func() {
					modal.SetText(fmt.Sprintf("Error starting server %d (%s): %v", i+1, serverName, err))
				})
				return
			}
		}
		app.QueueUpdateDraw(func() {
			modal.SetText(fmt.Sprintf("All %d servers in fleet %s started successfully!", fleet.ServerCount, fleet.Suffix))
		})
	}()
}

func stopAllServers(app *tview.Application, pages *tview.Pages, fleet FleetInfo) {
	modal := tview.NewModal().
		SetText(fmt.Sprintf("Stopping all %d servers in fleet %s...", fleet.ServerCount, fleet.Suffix)).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			pages.RemovePage("stoppingAll")
		})

	pages.AddPage("stoppingAll", modal, true, true)

	go func() {
		for i := range fleet.BuildDirs {
			serverName := fmt.Sprintf("fleet-%s-%d", fleet.Suffix, i+1)
			stopServerProcess(serverName)
		}
		app.QueueUpdateDraw(func() {
			modal.SetText(fmt.Sprintf("All %d servers in fleet %s stopped!", fleet.ServerCount, fleet.Suffix))
		})
	}()
}

func checkFleetStatus(app *tview.Application, pages *tview.Pages, fleet FleetInfo) {
	modal := tview.NewModal().
		SetText("Checking fleet status...").
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			pages.RemovePage("checkingStatus")
		})

	pages.AddPage("checkingStatus", modal, true, true)

	go func() {
		running := 0
		total := len(fleet.BuildDirs)

		for i, _ := range fleet.BuildDirs {
			serverName := fmt.Sprintf("fleet-%s-%d", fleet.Suffix, i+1)
			if isServerRunning(serverName) {
				running++
			}
		}

		app.QueueUpdateDraw(func() {
			modal.SetText(fmt.Sprintf("Fleet %s: %d/%d servers running", fleet.Suffix, running, total))
		})
	}()
}

func startServer(app *tview.Application, pages *tview.Pages, buildDir, serverName string) {
	modal := tview.NewModal().
		SetText(fmt.Sprintf("Starting server %s...", serverName)).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			pages.RemovePage("startingServer")
		})

	pages.AddPage("startingServer", modal, true, true)

	go func() {
		if err := startServerProcess(buildDir, serverName); err != nil {
			app.QueueUpdateDraw(func() {
				modal.SetText(fmt.Sprintf("Error starting server: %v", err))
			})
		} else {
			app.QueueUpdateDraw(func() {
				modal.SetText(fmt.Sprintf("Server %s started successfully!", serverName))
			})
		}
	}()
}

func stopServer(app *tview.Application, pages *tview.Pages, serverName string) {
	modal := tview.NewModal().
		SetText(fmt.Sprintf("Stopping server %s...", serverName)).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			pages.RemovePage("stoppingServer")
		})

	pages.AddPage("stoppingServer", modal, true, true)

	go func() {
		stopServerProcess(serverName)
		app.QueueUpdateDraw(func() {
			modal.SetText(fmt.Sprintf("Server %s stopped!", serverName))
		})
	}()
}

func checkServerStatus(app *tview.Application, pages *tview.Pages, buildDir, serverName string) {
	modal := tview.NewModal().
		SetText(fmt.Sprintf("Checking status of %s...", serverName)).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			pages.RemovePage("checkingServer")
		})

	pages.AddPage("checkingServer", modal, true, true)

	go func() {
		running := isServerRunning(serverName)
		status := "stopped"
		if running {
			status = "running"
		}
		app.QueueUpdateDraw(func() {
			modal.SetText(fmt.Sprintf("Server %s is %s", serverName, status))
		})
	}()
}

func startServerProcess(buildDir, serverName string) error {
	// Check if already running
	if isServerRunning(serverName) {
		return fmt.Errorf("server %s is already running", serverName)
	}

	// Check if binary exists
	binaryPath := filepath.Join(buildDir, "unrealircd")
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return fmt.Errorf("unrealircd binary not found at %s", binaryPath)
	}

	// Check if config exists
	configPath := filepath.Join(buildDir, "conf", "unrealircd.conf")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file not found at %s", configPath)
	}

	// Start the server process
	cmd := exec.Command("./unrealircd", "start")
	cmd.Dir = buildDir

	// Capture output for debugging
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %v", err)
	}

	// Wait a moment to see if it stays running
	time.Sleep(2 * time.Second)

	if cmd.Process == nil || cmd.ProcessState != nil {
		return fmt.Errorf("process exited immediately. stdout: %s, stderr: %s", stdout.String(), stderr.String())
	}

	// Store the process info (simplified - in real implementation you'd want to track PIDs)
	// For now, we'll just let it run in background
	go func() {
		cmd.Wait()
	}()

	return nil
}

func stopServerProcess(serverName string) {
	// Find and kill the process
	cmd := exec.Command("pkill", "-f", serverName)
	cmd.Run() // Ignore errors - process might not exist
}

func isServerRunning(serverName string) bool {
	cmd := exec.Command("pgrep", "-f", serverName)
	return cmd.Run() == nil
}

func randomString(n int) string {
	letters := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return ""
	}

	for i := 0; i < n; i++ {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b)
}

func generateSequentialSID(index int) string {
	// Start from 001 and go sequentially
	// 001-999, then 0AA-0ZZ, then 100-999, 1AA-1ZZ, etc.

	if index <= 999 {
		return fmt.Sprintf("%03d", index)
	}

	// For indices > 999, use base-36 encoding with a prefix
	// 1000 = 0AA, 1001 = 0AB, etc.
	adjustedIndex := index - 1000

	// Calculate the prefix (0-9) and suffix (AA-ZZ)
	prefix := adjustedIndex / 676 // 26*26 = 676
	suffixIndex := adjustedIndex % 676

	suffix1 := 'A' + rune(suffixIndex/26)
	suffix2 := 'A' + rune(suffixIndex%26)

	return fmt.Sprintf("%d%c%c", prefix, suffix1, suffix2)
}

func modifyFleetServerConfig(exampleContent, suffix string, serverIndex, totalServers int) string {
	serverName := fmt.Sprintf("fleet-%s-%d", suffix, serverIndex)
	// just so it doesn't get in the way
	port := 16667 + serverIndex - 1
	sslPort := 26697 + serverIndex - 1
	serverPort := 36900 + serverIndex - 1

	// Replace key configuration values
	content := exampleContent

	// Replace server name (look for CHANGE THIS or default values)
	content = strings.ReplaceAll(content, "irc.example.org", fmt.Sprintf("%s.test", serverName))
	content = strings.ReplaceAll(content, "ExampleNet", "TestFleet")
	content = strings.ReplaceAll(content, "Example IRC Server", fmt.Sprintf("Test IRC Server %d", serverIndex))

	// Replace port (look for default port 6667)
	content = strings.ReplaceAll(content, "port 6667;", fmt.Sprintf("port %d;", port))
	content = strings.ReplaceAll(content, "port 6697;", fmt.Sprintf("port %d;", sslPort))
	content = strings.ReplaceAll(content, "port 6900;", fmt.Sprintf("port %d;", serverPort))

	// Replace SID (look for default 001)
	content = strings.ReplaceAll(content, "sid \"001\";", fmt.Sprintf("sid \"%s\";", generateSequentialSID(serverIndex)))

	// Replace email
	content = strings.ReplaceAll(content, "set.this.to.email.address", "fake@email.com")

	// Oper block shit
	content = strings.ReplaceAll(content, "bobsmith", "testoper")
	content = strings.ReplaceAll(content, "$argon2id..etc..", "passwordlol")

	content = strings.Replace(content, "and another one", randomString(80), 1)
	content = strings.Replace(content, "and another one", randomString(80), 1)

	// Add a comment at the top indicating this is a test fleet config
	headerComment := fmt.Sprintf("// Test fleet server configuration - %s\n// Auto-generated for server %d of %d\n\n", serverName, serverIndex, totalServers)
	content = headerComment + content

	return content
}

func generateFleetLinkBlocks(suffix string, numServers int, homeDir string, updateOutput func(string)) error {
	for i := 1; i <= numServers; i++ {
		buildDir := filepath.Join(homeDir, fmt.Sprintf("unrealircd-fleet-%s-%d", suffix, i))

		updateOutput(fmt.Sprintf("Generating link block for server %d...", i))

		// Run genlinkblock command
		cmd := exec.Command("./unrealircd", "genlinkblock")
		cmd.Dir = buildDir

		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to generate link block for server %d: %v\nOutput: %s", i, err, string(output))
		}

		// Extract link block from output (between the hash lines)
		outputStr := string(output)
		startMarker := "################################################################################"
		startIdx := strings.Index(outputStr, startMarker)
		if startIdx == -1 {
			return fmt.Errorf("could not find start marker in genlinkblock output for server %d", i)
		}

		// Find the second occurrence of the marker (end of link block)
		endIdx := strings.Index(outputStr[startIdx+len(startMarker):], startMarker)
		if endIdx == -1 {
			return fmt.Errorf("could not find end marker in genlinkblock output for server %d", i)
		}
		endIdx += startIdx + len(startMarker) + len(startMarker)

		linkBlock := outputStr[startIdx:endIdx]
		outgoing := fmt.Sprintf("fleet-%s-%d.test;", suffix, i)
		linkBlock = strings.ReplaceAll(linkBlock, outgoing, "127.0.0.1;")
		linkBlock = strings.ReplaceAll(linkBlock, "#", "")

		// Add link block to neighboring servers
		// For server i, add to server i-1 and i+1 if they exist
		neighbors := []int{}
		if i > 1 {
			neighbors = append(neighbors, i-1)
		}
		if i < numServers {
			neighbors = append(neighbors, i+1)
		}

		for _, neighbor := range neighbors {
			neighborBuildDir := filepath.Join(homeDir, fmt.Sprintf("unrealircd-fleet-%s-%d", suffix, neighbor))
			configPath := filepath.Join(neighborBuildDir, "conf", "unrealircd.conf")

			// Read existing config
			configContent, err := os.ReadFile(configPath)
			if err != nil {
				return fmt.Errorf("failed to read config for server %d: %v", neighbor, err)
			}

			// Append link block
			newContent := string(configContent) + "\n" + linkBlock + "\n"

			// Write back
			if err := os.WriteFile(configPath, []byte(newContent), 0644); err != nil {
				return fmt.Errorf("failed to write config for server %d: %v", neighbor, err)
			}

			updateOutput(fmt.Sprintf("Added link block from server %d to server %d", i, neighbor))
		}
	}

	return nil
}

func createTestFleet(app *tview.Application, pages *tview.Pages, numServers int) {
	// Create a text view to show output
	outputView := tview.NewTextView()
	outputView.SetBorder(true).SetTitle("Test Fleet Creation Output")
	outputView.SetDynamicColors(true)
	outputView.SetWordWrap(true)
	outputView.SetScrollable(true)

	// Create progress modal with output view
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	flex.AddItem(tview.NewTextView().SetText("Creating test fleet..."), 1, 0, false)
	flex.AddItem(outputView, 0, 1, false)
	flex.AddItem(tview.NewTextView().SetText("Press ESC to cancel"), 1, 0, false)

	pages.AddPage("fleet_progress_modal", flex, true, true)

	go func() {
		updateOutput := func(text string) {
			app.QueueUpdateDraw(func() {
				fmt.Fprintf(outputView, "%s\n", text)
				outputView.ScrollToEnd()
			})
		}

		// First, fetch the latest version info
		updateOutput("Fetching latest UnrealIRCd version...")
		resp, err := http.Get("https://www.unrealircd.org/downloads/list.json")
		if err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("fleet_progress_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error fetching updates: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("fleet_fetch_error_modal")
					})
				pages.AddPage("fleet_fetch_error_modal", errorModal, true, true)
			})
			return
		}
		defer resp.Body.Close()

		var updateResp UpdateResponse
		if err := json.NewDecoder(resp.Body).Decode(&updateResp); err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("fleet_progress_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error parsing update info: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("fleet_parse_error_modal")
					})
				pages.AddPage("fleet_parse_error_modal", errorModal, true, true)
			})
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

		if stableVersion == "" {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("fleet_progress_modal")
				errorModal := tview.NewModal().
					SetText("Could not find stable version").
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("fleet_stable_error_modal")
					})
				pages.AddPage("fleet_stable_error_modal", errorModal, true, true)
			})
			return
		}

		updateOutput(fmt.Sprintf("Found stable version: %s", stableVersion))

		// Generate random suffix for this fleet
		suffixBytes := make([]byte, 4)
		if _, err := rand.Read(suffixBytes); err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("fleet_progress_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error generating random suffix: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("fleet_suffix_error_modal")
					})
				pages.AddPage("fleet_suffix_error_modal", errorModal, true, true)
			})
			return
		}
		suffix := hex.EncodeToString(suffixBytes)[:8]

		updateOutput(fmt.Sprintf("Using fleet suffix: %s", suffix))

		// Create source directory
		usr, _ := user.Current()
		sourceDir := filepath.Join(usr.HomeDir, fmt.Sprintf("unrealircd-fleet-%s", suffix))
		updateOutput(fmt.Sprintf("Creating source directory: %s", sourceDir))

		if err := os.MkdirAll(sourceDir, 0755); err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("fleet_progress_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error creating source directory: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("fleet_source_error_modal")
					})
				pages.AddPage("fleet_source_error_modal", errorModal, true, true)
			})
			return
		}

		// Download and extract source
		downloadURL = fmt.Sprintf("https://www.unrealircd.org/downloads/unrealircd-%s.tar.gz", stableVersion)
		updateOutput(fmt.Sprintf("Downloading from: %s", downloadURL))

		resp, err = http.Get(downloadURL)
		if err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("fleet_progress_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error downloading source: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("fleet_download_error_modal")
					})
				pages.AddPage("fleet_download_error_modal", errorModal, true, true)
			})
			return
		}
		defer resp.Body.Close()

		// Extract tar.gz
		gzr, err := gzip.NewReader(resp.Body)
		if err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("fleet_progress_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error creating gzip reader: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("fleet_gzip_error_modal")
					})
				pages.AddPage("fleet_gzip_error_modal", errorModal, true, true)
			})
			return
		}
		defer gzr.Close()

		tr := tar.NewReader(gzr)
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("fleet_progress_modal")
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Error reading tar: %v", err)).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("fleet_tar_error_modal")
						})
					pages.AddPage("fleet_tar_error_modal", errorModal, true, true)
				})
				return
			}

			// Skip the top-level directory
			if header.Typeflag == tar.TypeDir && strings.Contains(header.Name, "unrealircd-") {
				continue
			}

			target := filepath.Join(sourceDir, strings.TrimPrefix(header.Name, fmt.Sprintf("unrealircd-%s/", stableVersion)))
			if header.Typeflag == tar.TypeDir {
				if err := os.MkdirAll(target, 0755); err != nil {
					app.QueueUpdateDraw(func() {
						pages.RemovePage("fleet_progress_modal")
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Error creating directory: %v", err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("fleet_mkdir_error_modal")
							})
						pages.AddPage("fleet_mkdir_error_modal", errorModal, true, true)
					})
					return
				}
			} else {
				// Ensure parent directory exists
				parentDir := filepath.Dir(target)
				if err := os.MkdirAll(parentDir, 0755); err != nil {
					app.QueueUpdateDraw(func() {
						pages.RemovePage("fleet_progress_modal")
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Error creating parent directory: %v", err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("fleet_mkdir_error_modal")
							})
						pages.AddPage("fleet_mkdir_error_modal", errorModal, true, true)
					})
					return
				}
				f, err := os.Create(target)
				if err != nil {
					app.QueueUpdateDraw(func() {
						pages.RemovePage("fleet_progress_modal")
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Error creating file: %v", err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("fleet_create_error_modal")
							})
						pages.AddPage("fleet_create_error_modal", errorModal, true, true)
					})
					return
				}
				if _, err := io.Copy(f, tr); err != nil {
					f.Close()
					app.QueueUpdateDraw(func() {
						pages.RemovePage("fleet_progress_modal")
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Error writing file: %v", err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("fleet_write_error_modal")
							})
						pages.AddPage("fleet_write_error_modal", errorModal, true, true)
					})
					return
				}
				f.Close()
			}
		}

		updateOutput("Source extracted successfully")

		// Build each server individually with correct paths
		for i := 1; i <= numServers; i++ {
			// Create build directory for this server
			buildDir := filepath.Join(usr.HomeDir, fmt.Sprintf("unrealircd-fleet-%s-%d", suffix, i))
			if err := os.MkdirAll(buildDir, 0755); err != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("fleet_progress_modal")
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Error creating build directory for server %d: %v", i, err)).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("fleet_build_dir_error_modal")
						})
					pages.AddPage("fleet_build_dir_error_modal", errorModal, true, true)
				})
				return
			}

			// Create config.settings for this server
			err := saveConfigSettings(sourceDir, buildDir, "0600", "", "0", "2000", "classic", "auto", "", "")
			if err != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("fleet_progress_modal")
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Error saving config.settings for server %d: %v", i, err)).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("fleet_config_save_error_modal")
						})
					pages.AddPage("fleet_config_save_error_modal", errorModal, true, true)
				})
				return
			}

			// Make Config script executable
			chmodCmd := exec.Command("chmod", "+x", "Config")
			chmodCmd.Dir = sourceDir
			if err := chmodCmd.Run(); err != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("fleet_progress_modal")
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Failed to make Config executable for server %d: %v", i, err)).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("fleet_chmod_error_modal")
						})
					pages.AddPage("fleet_chmod_error_modal", errorModal, true, true)
				})
				return
			}

			// Make configure script executable
			chmodConfigureCmd := exec.Command("chmod", "+x", "configure")
			chmodConfigureCmd.Dir = sourceDir
			if err := chmodConfigureCmd.Run(); err != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("fleet_progress_modal")
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Failed to make configure executable for server %d: %v", i, err)).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("fleet_chmod_configure_error_modal")
						})
					pages.AddPage("fleet_chmod_configure_error_modal", errorModal, true, true)
				})
				return
			}

			// Run ./Config -quick
			updateOutput(fmt.Sprintf("Configuring server %d of %d...", i, numServers))
			configCmd := exec.Command("./Config", "-quick")
			configCmd.Dir = sourceDir
			if output, err := configCmd.CombinedOutput(); err != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("fleet_progress_modal")

					// Create a scrollable text view for the error output
					errorTextView := tview.NewTextView().
						SetText(fmt.Sprintf("Config failed for server %d: %v\n\nOutput:\n%s", i, err, string(output))).
						SetDynamicColors(true).
						SetWordWrap(true).
						SetScrollable(true).
						SetBorder(true).
						SetTitle("Config Error Details")

					// Create a flex layout with the text view and OK button
					errorFlex := tview.NewFlex().SetDirection(tview.FlexRow).
						AddItem(errorTextView, 0, 1, true).
						AddItem(tview.NewButton("OK").SetSelectedFunc(func() {
							pages.RemovePage("fleet_config_error_modal")
						}), 1, 0, false)

					pages.AddPage("fleet_config_error_modal", errorFlex, true, true)
					app.SetFocus(errorTextView)
				})
				return
			} else {
				// Show config output on success
				updateOutput(fmt.Sprintf("Config output for server %d:\n%s", i, string(output)))
			}

			// Make buildmod script executable
			buildmodChmodCmd := exec.Command("chmod", "+x", "src/buildmod")
			buildmodChmodCmd.Dir = sourceDir
			if err := buildmodChmodCmd.Run(); err != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("fleet_progress_modal")
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Failed to make buildmod executable for server %d: %v", i, err)).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("fleet_buildmod_chmod_error_modal")
						})
					pages.AddPage("fleet_buildmod_chmod_error_modal", errorModal, true, true)
				})
				return
			}

			updateOutput(fmt.Sprintf("Building server %d of %d...", i, numServers))

			// Run make
			makeCmd := exec.Command("make")
			makeCmd.Dir = sourceDir
			if output, err := makeCmd.CombinedOutput(); err != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("fleet_progress_modal")

					// Create a scrollable text view for the error output
					errorTextView := tview.NewTextView().
						SetText(fmt.Sprintf("Make failed for server %d: %v\n\nOutput:\n%s", i, err, string(output))).
						SetDynamicColors(true).
						SetWordWrap(true).
						SetScrollable(true).
						SetBorder(true).
						SetTitle("Make Error Details")

					// Create a flex layout with the text view and OK button
					errorFlex := tview.NewFlex().SetDirection(tview.FlexRow).
						AddItem(errorTextView, 0, 1, true).
						AddItem(tview.NewButton("OK").SetSelectedFunc(func() {
							pages.RemovePage("fleet_make_error_modal")
						}), 1, 0, false)

					pages.AddPage("fleet_make_error_modal", errorFlex, true, true)
					app.SetFocus(errorTextView)
				})
				return
			} else {
				// Show make output on success
				updateOutput(fmt.Sprintf("Make output for server %d:\n%s", i, string(output)))
			}

			// Generate TLS certificates for this server
			updateOutput(fmt.Sprintf("Generating TLS certificates for server %d...", i))
			pemCmd := exec.Command("make", "pem")
			pemCmd.Dir = sourceDir
			// Provide non-interactive answers for certificate generation
			pemCmd.Stdin = strings.NewReader("\n\nTestFleet\nIRCd\nfleet-" + suffix + "-" + fmt.Sprintf("%d", i) + ".test\n\n\n\n")
			if output, err := pemCmd.CombinedOutput(); err != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("fleet_progress_modal")

					// Create a scrollable text view for the error output
					errorTextView := tview.NewTextView().
						SetText(fmt.Sprintf("Certificate generation failed for server %d: %v\n\nOutput:\n%s", i, err, string(output))).
						SetDynamicColors(true).
						SetWordWrap(true).
						SetScrollable(true).
						SetBorder(true).
						SetTitle("Certificate Generation Error Details")

					// Create a flex layout with the text view and OK button
					errorFlex := tview.NewFlex().SetDirection(tview.FlexRow).
						AddItem(errorTextView, 0, 1, true).
						AddItem(tview.NewButton("OK").SetSelectedFunc(func() {
							pages.RemovePage("fleet_pem_error_modal")
						}), 1, 0, false)

					pages.AddPage("fleet_pem_error_modal", errorFlex, true, true)
					app.SetFocus(errorTextView)
				})
				return
			} else {
				// Show certificate generation output on success
				updateOutput(fmt.Sprintf("Certificate generation output for server %d:\n%s", i, string(output)))
			}

			updateOutput(fmt.Sprintf("Installing server %d of %d...", i, numServers))

			// Run make install
			installCmd := exec.Command("make", "install")
			installCmd.Dir = sourceDir
			if output, err := installCmd.CombinedOutput(); err != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("fleet_progress_modal")

					// Create a scrollable text view for the error output
					errorTextView := tview.NewTextView().
						SetText(fmt.Sprintf("Install failed for server %d: %v\n\nOutput:\n%s", i, err, string(output))).
						SetDynamicColors(true).
						SetWordWrap(true).
						SetScrollable(true).
						SetBorder(true).
						SetTitle("Install Error Details")

					// Create a flex layout with the text view and OK button
					errorFlex := tview.NewFlex().SetDirection(tview.FlexRow).
						AddItem(errorTextView, 0, 1, true).
						AddItem(tview.NewButton("OK").SetSelectedFunc(func() {
							pages.RemovePage("fleet_install_error_modal")
						}), 1, 0, false)

					pages.AddPage("fleet_install_error_modal", errorFlex, true, true)
					app.SetFocus(errorTextView)
				})
				return
			} else {
				// Show install output on success
				updateOutput(fmt.Sprintf("Install output for server %d:\n%s", i, string(output)))
			}

			// Clean build artifacts for next server
			cleanCmd := exec.Command("make", "clean")
			cleanCmd.Dir = sourceDir
			cleanCmd.Run() // Ignore errors for clean
		}

		for i := 1; i <= numServers; i++ {
			updateOutput(fmt.Sprintf("Setting up server %d of %d...", i, numServers))

			// Create build directory
			buildDir := filepath.Join(usr.HomeDir, fmt.Sprintf("unrealircd-fleet-%s-%d", suffix, i))
			if err := os.MkdirAll(buildDir, 0755); err != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("fleet_progress_modal")
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Error creating build directory for server %d: %v", i, err)).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("fleet_build_dir_error_modal")
						})
					pages.AddPage("fleet_build_dir_error_modal", errorModal, true, true)
				})
				return
			}

			// Configure server-specific settings
			confDir := filepath.Join(buildDir, "conf")
			if err := os.MkdirAll(confDir, 0755); err != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("fleet_progress_modal")
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Error creating conf directory for server %d: %v", i, err)).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("fleet_conf_dir_error_modal")
						})
					pages.AddPage("fleet_conf_dir_error_modal", errorModal, true, true)
				})
				return
			}

			// Copy and modify server-specific config from example.conf
			exampleConfPath := filepath.Join(sourceDir, "doc", "conf", "examples", "example.conf")
			exampleContent, err := os.ReadFile(exampleConfPath)
			if err != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("fleet_progress_modal")
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Error reading example.conf for server %d: %v", i, err)).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("fleet_example_read_error_modal")
						})
					pages.AddPage("fleet_example_read_error_modal", errorModal, true, true)
				})
				return
			}

			// Modify the config for this server
			configContent := modifyFleetServerConfig(string(exampleContent), suffix, i, numServers)
			configPath := filepath.Join(confDir, "unrealircd.conf")
			if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("fleet_progress_modal")
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Error writing config for server %d: %v", i, err)).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("fleet_config_write_error_modal")
						})
					pages.AddPage("fleet_config_write_error_modal", errorModal, true, true)
				})
				return
			}

			// Create symbolic links for required directories
			links := map[string]string{
				"conf": "etc",
				"logs": "logs",
				"data": "data",
				"tmp":  "tmp",
			}

			for linkName, targetName := range links {
				linkPath := filepath.Join(buildDir, linkName)
				targetPath := filepath.Join(buildDir, targetName)
				if err := os.Symlink(targetPath, linkPath); err != nil && !os.IsExist(err) {
					app.QueueUpdateDraw(func() {
						pages.RemovePage("fleet_progress_modal")
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Error creating symlink for server %d: %v", i, err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("fleet_symlink_error_modal")
							})
						pages.AddPage("fleet_symlink_error_modal", errorModal, true, true)
					})
					return
				}
			}
		}

		// Generate and add link blocks between servers
		updateOutput("Generating link blocks between servers...")
		if err := generateFleetLinkBlocks(suffix, numServers, usr.HomeDir, updateOutput); err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("fleet_progress_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Error generating link blocks: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("fleet_link_error_modal")
					})
				pages.AddPage("fleet_link_error_modal", errorModal, true, true)
			})
			return
		}

		// Success!
		app.QueueUpdateDraw(func() {
			pages.RemovePage("fleet_progress_modal")
			successModal := tview.NewModal().
				SetText(fmt.Sprintf("Test fleet created successfully!\n\nFleet suffix: %s\nServers: %d\n\nSource directory: %s\nBuild directories: %s-1 through %s-%d", suffix, numServers, sourceDir, fmt.Sprintf("unrealircd-fleet-%s", suffix), fmt.Sprintf("unrealircd-fleet-%s", suffix), numServers)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("fleet_success_modal")
					pages.SwitchToPage("tests_submenu")
				})
			pages.AddPage("fleet_success_modal", successModal, true, true)
		})
	}()
}

func runTestFleetCLI(numServers int) {
	fmt.Printf("Creating test fleet with %d servers...\n", numServers)

	// Fetch the latest version info
	fmt.Print("Fetching latest UnrealIRCd version... ")
	resp, err := http.Get("https://www.unrealircd.org/downloads/list.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var updateResp UpdateResponse
	if err := json.NewDecoder(resp.Body).Decode(&updateResp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing update info: %v\n", err)
		os.Exit(1)
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
		fmt.Fprintf(os.Stderr, "No stable version found in update info.\n")
		os.Exit(1)
	}
	fmt.Printf("Found %s\n", stableVersion)

	// Generate random suffix
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	randomSuffix := hex.EncodeToString(randomBytes)[:8]

	// Download the source
	fmt.Print("Downloading source... ")
	resp, err = http.Get(downloadURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	// Create temp file for the downloaded archive
	tempFile, err := os.CreateTemp("", "unrealircd-fleet-*.tar.gz")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating temp file: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error saving download: %v\n", err)
		os.Exit(1)
	}
	tempFile.Close()
	fmt.Println("Done")

	// Now create each server in the fleet
	usr, _ := user.Current()
	baseDir := usr.HomeDir

	// Create the shared source directory with random suffix
	sourceDir := filepath.Join(baseDir, fmt.Sprintf("unrealircd-fleet-%s", randomSuffix))

	// Remove existing source directory if it exists
	os.RemoveAll(sourceDir)

	// Extract source once
	fmt.Printf("Extracting source to %s... ", sourceDir)
	err = extractTarGz(tempFile.Name(), sourceDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting source: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Done")

	for i := 1; i <= numServers; i++ {
		fmt.Printf("Setting up server %d of %d... ", i, numServers)

		// Create build directory for this server with suffix + number
		buildDir := filepath.Join(baseDir, fmt.Sprintf("unrealircd-fleet-%s-%d", randomSuffix, i))

		// Remove existing build directory if it exists
		os.RemoveAll(buildDir)

		// Configure this server
		err = configureFleetServer(sourceDir, buildDir, i, numServers, stableVersion)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error configuring server %d: %v\n", i, err)
			os.Exit(1)
		}
		fmt.Println("Done")
	}

	// Success!
	fmt.Printf("\nTest fleet created successfully!\n\n")
	fmt.Printf("Created %d UnrealIRCd servers in spanning tree topology.\n\n", numServers)
	fmt.Printf("Source directory: ~/unrealircd-fleet-%s\n", randomSuffix)
	fmt.Printf("Build directories: ~/unrealircd-fleet-%s-1 through ~/unrealircd-fleet-%s-%d\n", randomSuffix, randomSuffix, numServers)
}

func configureFleetServer(sourceDir, buildDir string, serverIndex, totalServers int, version string) error {
	return configureFleetServerWithOutput(sourceDir, buildDir, serverIndex, totalServers, version, func(string) {})
}

func configureFleetServerWithOutput(sourceDir, buildDir string, serverIndex, totalServers int, version string, updateOutput func(string)) error {
	// Save config.settings
	updateOutput("  Saving configuration...")
	err := saveConfigSettings(sourceDir, buildDir, "0600", "", "0", "2000", "classic", "auto", "", "")
	if err != nil {
		return fmt.Errorf("saving config.settings: %w", err)
	}

	// Run ./configure
	updateOutput("  Running configure...")
	cmd := exec.Command("./Config", "-quick")
	cmd.Dir = sourceDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("running configure: %w\nOutput: %s", err, string(output))
	}

	// Run make
	updateOutput("  Running make...")
	cmd = exec.Command("make")
	cmd.Dir = sourceDir
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("running make: %w\nOutput: %s", err, string(output))
	}

	// Run make install
	updateOutput("  Running make install...")
	cmd = exec.Command("make", "install")
	cmd.Dir = sourceDir
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("running make install: %w\nOutput: %s", err, string(output))
	}

	// Configure unrealircd.conf
	updateOutput("  Configuring unrealircd.conf...")
	err = configureFleetUnrealIRCdConf(buildDir, serverIndex, totalServers)
	if err != nil {
		return fmt.Errorf("configuring unrealircd.conf: %w", err)
	}

	return nil
}

func configureFleetUnrealIRCdConf(buildDir string, serverIndex, totalServers int) error {
	// First, ensure unrealircd.conf exists by copying from example.conf
	err := setupConfigFile(buildDir)
	if err != nil {
		return fmt.Errorf("setting up config file: %w", err)
	}

	confFile := filepath.Join(buildDir, "conf", "unrealircd.conf")

	// Read the config file
	content, err := os.ReadFile(confFile)
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}

	contentStr := string(content)

	// Replace 'irc.example.org' with the current server name
	serverName := fmt.Sprintf("irc%d.testfleet.local", serverIndex)
	contentStr = strings.ReplaceAll(contentStr, "irc.example.org", serverName)

	// Replace the 'me' block with unique server name and three-digit SID
	sid := fmt.Sprintf("%03d", serverIndex)
	meBlock := fmt.Sprintf(`me {
	name "%s";
	info "Test Fleet Server %d";
	sid "%s";
};`, serverName, serverIndex, sid)

	// Use regex to replace the me block
	meRegex := regexp.MustCompile(`(?s)me\s*\{[^}]*\};`)
	contentStr = string(meRegex.ReplaceAllLiteral([]byte(contentStr), []byte(meBlock)))

	// Replace cloak keys - replace each "and another one" with a unique random hex string
	cloakKeyRegex := regexp.MustCompile(`"and another one";`)
	contentStr = cloakKeyRegex.ReplaceAllStringFunc(contentStr, func(match string) string {
		return fmt.Sprintf(`"%s";`, generateRandomHexString(80))
	})

	// Replace email address
	contentStr = strings.ReplaceAll(contentStr, "set.this.to.email.address", "random@email.com")

	// Replace oper name and password
	contentStr = strings.ReplaceAll(contentStr, "bobsmith", "testoper")
	contentStr = strings.ReplaceAll(contentStr, `password "$argon2id..etc..";`, `password "testpasslol";`)

	// Add link blocks for spanning tree topology
	var linkBlocks strings.Builder

	// If this is not the first server, connect to the previous server
	if serverIndex > 1 {
		prevServerName := fmt.Sprintf("irc%d.testfleet.local", serverIndex-1)
		linkBlocks.WriteString(fmt.Sprintf(`

link %s {
	incoming {
		mask *;
	}
	outgoing {
		hostname "127.0.0.1";
		port %d;
		options { tls; autoconnect; }
	}
	password "testfleetpassword%d" { spkifp; }
	class servers;
};`, prevServerName, 6660+serverIndex-1, serverIndex-1))
	}

	// If this is not the last server, prepare for the next server to connect
	if serverIndex < totalServers {
		nextServerName := fmt.Sprintf("irc%d.testfleet.local", serverIndex+1)
		linkBlocks.WriteString(fmt.Sprintf(`

/* Link block for %s - to be added to server %d's config:
link %s {
	incoming {
		mask *;
	}
	outgoing {
		hostname "127.0.0.1";
		port %d;
		options { tls; autoconnect; }
	}
	password "testfleetpassword%d" { spkifp; }
	class servers;
};
*/`, nextServerName, serverIndex+1, serverName, 6660+serverIndex, serverIndex))
	}

	// Add link blocks before the closing bracket of the file
	if linkBlocks.Len() > 0 {
		// Find the last closing brace and add link blocks before it
		lastBraceIndex := strings.LastIndex(contentStr, "}")
		if lastBraceIndex != -1 {
			contentStr = contentStr[:lastBraceIndex] + linkBlocks.String() + "\n}" + contentStr[lastBraceIndex+1:]
		}
	}

	// Write back the modified config
	err = os.WriteFile(confFile, []byte(contentStr), 0644)
	if err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

func installationOptionsPage(app *tview.Application, pages *tview.Pages, sourceDir, buildDir string) {
	// Text view on right for descriptions
	textView := &FocusableTextView{tview.NewTextView()}
	textView.SetBorder(true).SetTitle("Description")
	textView.SetDynamicColors(true)
	textView.SetWordWrap(true)
	textView.SetScrollable(true)

	// Descriptions for Installation Options submenu
	descriptions := map[string]string{
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
		"• Uninstall UnrealIRCd": `Remove an existing UnrealIRCd installation.

Features:
• Select from detected installations
• Delete both source and build directories
• Clean removal of all installation files
• Automatic configuration cleanup

Completely remove unwanted UnrealIRCd installations.`}

	list := tview.NewList()
	list.SetBorder(true).SetBorderColor(tcell.ColorGreen)
	list.SetTitle("Installation Options")
	list.AddItem("• Set up new UnrealIRCd install", "  Set up a new UnrealIRCd installation", 0, nil)
	list.AddItem("• Switch UnrealIRCd install", "  Switch between installed UnrealIRCd versions", 0, nil)
	list.AddItem("• Uninstall UnrealIRCd", "  Remove an existing UnrealIRCd installation", 0, nil)

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
			case "• Set up new UnrealIRCd install":
				setupNewInstallPage(app, pages)
			case "• Switch UnrealIRCd install":
				switchInstallPage(app, pages)
			case "• Uninstall UnrealIRCd":
				uninstallUnrealIRCdPage(app, pages)
			}
		}
		lastClickIndex = index
		lastClickTime = now
	})

	list.SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		// For Enter key
		switch mainText {
		case "• Set up new UnrealIRCd install":
			setupNewInstallPage(app, pages)
		case "• Switch UnrealIRCd install":
			switchInstallPage(app, pages)
		case "• Uninstall UnrealIRCd":
			uninstallUnrealIRCdPage(app, pages)
		}
	})

	list.SetInputCapture(nil) // Remove custom input capture

	// Set initial description
	if len(descriptions) > 0 {
		textView.SetText(descriptions["• Set up new UnrealIRCd install"])
	}

	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.RemovePage("installation_options")
		pages.SwitchToPage("main_menu")
	})

	buttonBar := createButtonBar(backBtn)

	// Layout
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	browserFlex := tview.NewFlex().
		AddItem(list, 40, 0, true).
		AddItem(textView, 0, 1, false)
	flex.AddItem(header, 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(ui.CreateFooter("Double-click or Enter: Select | b: Back"), 3, 0, false)
	pages.AddPage("installation_options", flex, true, true)
	app.SetFocus(list)
}

func uninstallUnrealIRCdPage(app *tview.Application, pages *tview.Pages) {
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
			SetText("No UnrealIRCd installations found to uninstall.").
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("no_installs_modal")
			})
		pages.AddPage("no_installs_modal", errorModal, true, true)
		return
	}

	// Create list with installations
	list := tview.NewList()
	list.SetBorder(true).SetTitle("Select Installation to Uninstall")
	list.SetBorderColor(tcell.ColorRed)

	for _, dir := range sourceDirs {
		version, err := getUnrealIRCdVersion(dir)
		if err != nil {
			version = "Unknown"
		}

		// Get build directory
		buildDir, err := getBasePathFromConfig(dir)
		if err != nil {
			// Fallback to version-based path
			usr, _ := user.Current()
			buildDir = filepath.Join(usr.HomeDir, "unrealircd")
			if version != "" {
				buildDir = filepath.Join(usr.HomeDir, "unrealircd-"+version)
			}
		}

		displayName := fmt.Sprintf("UnrealIRCd %s", version)
		secondaryText := fmt.Sprintf("Source: %s | Build: %s", dir, buildDir)
		list.AddItem(displayName, secondaryText, 0, nil)
	}

	list.SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		selectedSourceDir := sourceDirs[index]

		// Get version and build dir
		version, err := getUnrealIRCdVersion(selectedSourceDir)
		if err != nil {
			version = "Unknown"
		}

		buildDir, err := getBasePathFromConfig(selectedSourceDir)
		if err != nil {
			usr, _ := user.Current()
			buildDir = filepath.Join(usr.HomeDir, "unrealircd")
			if version != "" {
				buildDir = filepath.Join(usr.HomeDir, "unrealircd-"+version)
			}
		}

		// Confirm uninstallation
		confirmModal := tview.NewModal().
			SetText(fmt.Sprintf("Uninstall UnrealIRCd %s?\n\nThis will permanently delete:\n• Source directory: %s\n• Build directory: %s\n\nThis action cannot be undone!", version, selectedSourceDir, buildDir)).
			AddButtons([]string{"Cancel", "Uninstall"}).
			SetDoneFunc(func(buttonIndex int, buttonLabel string) {
				if buttonLabel == "Uninstall" {
					// Delete source directory
					err := os.RemoveAll(selectedSourceDir)
					sourceDeleted := err == nil

					// Delete build directory
					err = os.RemoveAll(buildDir)
					buildDeleted := err == nil

					// Show result
					var message string
					if sourceDeleted && buildDeleted {
						message = fmt.Sprintf("Successfully uninstalled UnrealIRCd %s.\n\nDeleted source and build directories.", version)
					} else if sourceDeleted {
						message = fmt.Sprintf("Partially uninstalled UnrealIRCd %s.\n\nDeleted source directory, but failed to delete build directory: %v", version, err)
					} else if buildDeleted {
						message = fmt.Sprintf("Partially uninstalled UnrealIRCd %s.\n\nDeleted build directory, but failed to delete source directory.", version)
					} else {
						message = fmt.Sprintf("Failed to uninstall UnrealIRCd %s.\n\nCould not delete directories.", version)
					}

					resultModal := tview.NewModal().
						SetText(message).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("uninstall_result_modal")
							pages.RemovePage("uninstall_confirm")
							// Refresh the list by going back
							uninstallUnrealIRCdPage(app, pages)
						})
					pages.AddPage("uninstall_result_modal", resultModal, true, true)
				} else {
					pages.RemovePage("uninstall_confirm")
				}
			})
		pages.AddPage("uninstall_confirm", confirmModal, true, true)
	})

	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.RemovePage("uninstall_unrealircd")
		pages.SwitchToPage("installation_options")
	})

	buttonBar := createButtonBar(backBtn)

	// Layout
	contentFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	contentFlex.AddItem(createHeader(), 3, 0, false).AddItem(list, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(ui.CreateFooter("Enter: Select installation to uninstall | b: Back"), 3, 0, false)

	centeredFlex := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(tview.NewTextView(), 0, 1, false).
		AddItem(contentFlex, 100, 0, true).
		AddItem(tview.NewTextView(), 0, 1, false)

	pages.AddPage("uninstall_unrealircd", centeredFlex, true, true)
	app.SetFocus(list)
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
	flex.AddItem(header, 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(ui.CreateFooter("ESC: Back | Enter: Select | q: Quit"), 3, 0, false)
	pages.AddPage("module_manager_submenu", flex, true, true)
	moduleManagerSubmenuFocusables = []tview.Primitive{list, textView, backBtn}
}

func servicesPackageSubmenuPage(app *tview.Application, pages *tview.Pages, sourceDir, buildDir string) {
	// Text view on right for descriptions
	textView := &FocusableTextView{tview.NewTextView()}
	textView.SetBorder(true).SetTitle("Description")
	textView.SetDynamicColors(true)
	textView.SetWordWrap(true)
	textView.SetScrollable(true)

	// Descriptions for Services Package submenu
	descriptions := map[string]string{
		"• Install Anope Services": `Install and configure Anope IRC Services.

Features:
• Download latest Anope Services source code
• Automatic compilation and installation
• Generate services configuration files
• Set up linking with UnrealIRCd
• Configure ulines and link blocks
• Post-install setup and testing

Anope provides NickServ, ChanServ, MemoServ, and other essential IRC services for user and channel management.`,
		"• Install Atheme Services": `Install and configure Atheme IRC Services.

Features:
• Download latest Atheme Services source code
• Automatic compilation and installation
• Generate services configuration files
• Set up linking with UnrealIRCd
• Configure ulines and link blocks
• Post-install setup and testing

Atheme provides NickServ, ChanServ, MemoServ, and other IRC services with a focus on security and flexibility.`}

	list := tview.NewList()
	list.SetBorder(true).SetBorderColor(tcell.ColorBlue)
	list.SetTitle("Services Package")
	list.AddItem("• Install Anope Services", "  Install and configure Anope IRC Services", 0, nil)
	list.AddItem("• Install Atheme Services", "  Install and configure Atheme IRC Services", 0, nil)

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
			case "• Install Anope Services":
				installAnopeServices(app, pages, buildDir)
			case "• Install Atheme Services":
				installAthemeServices(app, pages, buildDir)
			}
		}
		lastClickIndex = index
		lastClickTime = now
	})

	list.SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		// For Enter key
		switch mainText {
		case "• Install Anope Services":
			installAnopeServices(app, pages, buildDir)
		case "• Install Atheme Services":
			installAthemeServices(app, pages, buildDir)
		}
	})

	list.SetInputCapture(nil) // Remove custom input capture

	// Set initial description
	if len(descriptions) > 0 {
		textView.SetText(descriptions["• Install Anope Services"])
	}

	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.RemovePage("services_package_submenu")
		pages.SwitchToPage("main_menu")
	})

	buttonBar := createButtonBar(backBtn)

	// Layout
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	browserFlex := tview.NewFlex().
		AddItem(list, 40, 0, true).
		AddItem(textView, 0, 1, false)
	flex.AddItem(header, 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(ui.CreateFooter("Double-click or Enter: Select | b: Back"), 3, 0, false)
	pages.AddPage("services_package_submenu", flex, true, true)
	app.SetFocus(list)
}

// Helper function to make HTTP requests with User-Agent
func makeHTTPRequest(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "UnrealIRCd-TUI/1.0")
	
	client := &http.Client{}
	return client.Do(req)
}

func installAnopeServices(app *tview.Application, pages *tview.Pages, buildDir string) {
	// Show progress modal for Anope installation
	modal := tview.NewModal().
		SetText("Installing Anope Services...\n\nThis may take several minutes.").
		AddButtons([]string{}).
		SetDoneFunc(func(int, string) {})
	pages.AddPage("anope_install_modal", modal, true, true)

	go func() {
		updateProgress := func(text string) {
			app.QueueUpdateDraw(func() {
				modal.SetText(text)
			})
		}

		// Create services directory
		usr, _ := user.Current()
		servicesDir := filepath.Join(usr.HomeDir, "anope-services")
		updateProgress("Creating services directory...")

		if err := os.MkdirAll(servicesDir, 0755); err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("anope_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to create services directory: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("anope_error_modal")
					})
				pages.AddPage("anope_error_modal", errorModal, true, true)
			})
			return
		}

		// Download latest Anope version
		updateProgress("Fetching latest Anope version...")
		resp, err := makeHTTPRequest("https://api.github.com/repos/anope/anope/releases/latest")
		if err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("anope_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to fetch Anope releases: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("anope_error_modal")
					})
				pages.AddPage("anope_error_modal", errorModal, true, true)
			})
			return
		}
		defer resp.Body.Close()

		var release struct {
			TagName    string `json:"tag_name"`
			TarballURL string `json:"tarball_url"`
			Assets     []struct {
				Name        string `json:"name"`
				DownloadURL string `json:"browser_download_url"`
			} `json:"assets"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("anope_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to parse release info: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("anope_error_modal")
					})
				pages.AddPage("anope_error_modal", errorModal, true, true)
			})
			return
		}

		// Use the tarball URL from GitHub
		downloadURL := release.TarballURL
		if downloadURL == "" {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("anope_install_modal")
				errorModal := tview.NewModal().
					SetText("Could not find Anope source download").
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("anope_error_modal")
					})
				pages.AddPage("anope_error_modal", errorModal, true, true)
			})
			return
		}

		updateProgress(fmt.Sprintf("Downloading Anope %s...", release.TagName))

		// Download and extract
		resp, err = makeHTTPRequest(downloadURL)
		if err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("anope_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to download Anope: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("anope_error_modal")
					})
				pages.AddPage("anope_error_modal", errorModal, true, true)
			})
			return
		}
		defer resp.Body.Close()

		// Extract tar.gz
		gzr, err := gzip.NewReader(resp.Body)
		if err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("anope_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to create gzip reader: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("anope_error_modal")
					})
				pages.AddPage("anope_error_modal", errorModal, true, true)
			})
			return
		}
		defer gzr.Close()

		tr := tar.NewReader(gzr)
		var topLevelDir string
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("anope_install_modal")
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Failed to read tar: %v", err)).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("anope_error_modal")
						})
					pages.AddPage("anope_error_modal", errorModal, true, true)
				})
				return
			}

			// Find the top-level directory
			if header.Typeflag == tar.TypeDir && topLevelDir == "" {
				topLevelDir = header.Name
				continue
			}

			// Skip if we haven't found the top-level dir yet
			if topLevelDir == "" {
				continue
			}

			target := filepath.Join(servicesDir, strings.TrimPrefix(header.Name, topLevelDir))
			if header.Typeflag == tar.TypeDir {
				if err := os.MkdirAll(target, 0755); err != nil {
					app.QueueUpdateDraw(func() {
						pages.RemovePage("anope_install_modal")
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Failed to create directory: %v", err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("anope_error_modal")
							})
						pages.AddPage("anope_error_modal", errorModal, true, true)
					})
					return
				}
			} else {
				parentDir := filepath.Dir(target)
				if err := os.MkdirAll(parentDir, 0755); err != nil {
					app.QueueUpdateDraw(func() {
						pages.RemovePage("anope_install_modal")
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Failed to create parent directory: %v", err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("anope_error_modal")
							})
						pages.AddPage("anope_error_modal", errorModal, true, true)
					})
					return
				}
				f, err := os.Create(target)
				if err != nil {
					app.QueueUpdateDraw(func() {
						pages.RemovePage("anope_install_modal")
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Failed to create file: %v", err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("anope_error_modal")
							})
						pages.AddPage("anope_error_modal", errorModal, true, true)
					})
					return
				}
				if _, err := io.Copy(f, tr); err != nil {
					f.Close()
					app.QueueUpdateDraw(func() {
						pages.RemovePage("anope_install_modal")
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Failed to write file: %v", err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("anope_error_modal")
							})
						pages.AddPage("anope_error_modal", errorModal, true, true)
					})
					return
				}
				f.Close()
			}
		}

		// Debug: list extracted files
		var extractedFiles []string
		err = filepath.Walk(servicesDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			relPath, _ := filepath.Rel(servicesDir, path)
			extractedFiles = append(extractedFiles, relPath)
			return nil
		})
		if err != nil {
			extractedFiles = []string{"error listing files"}
		}

		updateProgress("Configuring Anope...")

		// Create config.cache for Config script
		configCacheContent := fmt.Sprintf(`INSTDIR="%s"
RUNGROUP=""
UMASK=077
DEBUG="no"
USE_PCH="no"
EXTRA_INCLUDE_DIRS=""
EXTRA_LIB_DIRS=""
EXTRA_CONFIG_ARGS=""
`, servicesDir)
		configCachePath := filepath.Join(servicesDir, "config.cache")
		if err := os.WriteFile(configCachePath, []byte(configCacheContent), 0644); err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("anope_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to create config.cache: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("anope_error_modal")
					})
				pages.AddPage("anope_error_modal", errorModal, true, true)
			})
			return
		}

		// Make Config script executable
		configPath := filepath.Join(servicesDir, "Config")
		if err := os.Chmod(configPath, 0755); err != nil {
			// Config script might not exist, try CMake directly
		}

		// Try Config script first, fall back to CMake
		cmd := exec.Command("./Config", "-quick")
		cmd.Dir = servicesDir
		if output, err := cmd.CombinedOutput(); err != nil {
			// Config failed, try CMake directly
			updateProgress("Config failed, trying CMake...")
			cmd = exec.Command("cmake", "-B", "build", "-S", ".")
			cmd.Dir = servicesDir
			if cmakeOutput, cmakeErr := cmd.CombinedOutput(); cmakeErr != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("anope_install_modal")
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Both Config and CMake failed.\nConfig: %v\nCMake: %v\nConfig output: %s\nCMake output: %s\nExtracted files: %v", err, cmakeErr, string(output), string(cmakeOutput), extractedFiles)).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("anope_error_modal")
						})
					pages.AddPage("anope_error_modal", errorModal, true, true)
				})
				return
			}
		} else {
			// Config succeeded, proceed to build
		}

		updateProgress("Building Anope...")

		// Check if build directory exists (created by CMake) or build in-place
		buildDir := filepath.Join(servicesDir, "build")
		if _, err := os.Stat(buildDir); os.IsNotExist(err) {
			// No build directory, build in-place
			cmd = exec.Command("make")
			cmd.Dir = servicesDir
		} else {
			// Build directory exists, use it
			cmd = exec.Command("make")
			cmd.Dir = buildDir
		}
		if output, err := cmd.CombinedOutput(); err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("anope_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Make failed: %v\nOutput: %s", err, string(output))).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("anope_error_modal")
					})
				pages.AddPage("anope_error_modal", errorModal, true, true)
			})
			return
		}

		updateProgress("Installing Anope...")

		// Run make install from appropriate directory
		if _, err := os.Stat(buildDir); os.IsNotExist(err) {
			cmd = exec.Command("make", "install")
			cmd.Dir = servicesDir
		} else {
			cmd = exec.Command("make", "install")
			cmd.Dir = buildDir
		}
		if output, err := cmd.CombinedOutput(); err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("anope_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Make install failed: %v\nOutput: %s", err, string(output))).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("anope_error_modal")
					})
				pages.AddPage("anope_error_modal", errorModal, true, true)
			})
			return
		}

		updateProgress("Configuring services...")

		// Configure anope.conf
		anopeConfPath := filepath.Join(servicesDir, "conf", "anope.conf")
		if err := configureAnopeServices(anopeConfPath, buildDir); err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("anope_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to configure services: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("anope_error_modal")
					})
				pages.AddPage("anope_error_modal", errorModal, true, true)
			})
			return
		}

		updateProgress("Configuring UnrealIRCd linking...")

		// Configure UnrealIRCd for services linking
		if err := configureUnrealForServices(buildDir, "services.localhost", "anope"); err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("anope_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to configure UnrealIRCd: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("anope_error_modal")
					})
				pages.AddPage("anope_error_modal", errorModal, true, true)
			})
			return
		}

		// Success
		app.QueueUpdateDraw(func() {
			pages.RemovePage("anope_install_modal")
			successModal := tview.NewModal().
				SetText(fmt.Sprintf("Anope Services %s installed successfully!\n\nServices directory: %s\n\nTo start services: cd %s && ./services\n\nDon't forget to rehash UnrealIRCd after starting services.", release.TagName, servicesDir, servicesDir)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("anope_success_modal")
				})
			pages.AddPage("anope_success_modal", successModal, true, true)
		})
	}()
}

func configureAnopeServices(configPath, unrealBuildDir string) error {
	// Read the default services.conf
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading services.conf: %w", err)
	}

	contentStr := string(content)

	// Basic configuration replacements
	contentStr = strings.ReplaceAll(contentStr, "irc.example.org", "services.localhost")
	contentStr = strings.ReplaceAll(contentStr, "example.org", "localhost")

	// Set up server info
	serverInfo := `serverinfo {
	name = "services.localhost"
	description = "Anope IRC Services"
	numeric = "00A"
}`

	// Replace the serverinfo block
	serverInfoRegex := regexp.MustCompile(`(?s)serverinfo\s*\{[^}]*\};`)
	contentStr = string(serverInfoRegex.ReplaceAllLiteral([]byte(contentStr), []byte(serverInfo)))

	// Configure uplink
	uplinkConfig := `uplink {
	host = "127.0.0.1"
	port = 6667
	password = "serviceslinkpass"
}`

	// Replace uplink block
	uplinkRegex := regexp.MustCompile(`(?s)uplink\s*\{[^}]*\};`)
	contentStr = string(uplinkRegex.ReplaceAllLiteral([]byte(contentStr), []byte(uplinkConfig)))

	// Write back
	return os.WriteFile(configPath, []byte(contentStr), 0644)
}

func configureAthemeServices(configPath, unrealBuildDir string) error {
	// Read the default atheme.conf
	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading atheme.conf: %w", err)
	}

	contentStr := string(content)

	// Basic configuration replacements
	contentStr = strings.ReplaceAll(contentStr, "irc.example.org", "services.localhost")
	contentStr = strings.ReplaceAll(contentStr, "example.org", "localhost")

	// Configure server info
	serverInfo := `serverinfo {
	name = "services.localhost";
	desc = "Atheme IRC Services";
	numeric = "00A";
};`

	// Replace the serverinfo block
	serverInfoRegex := regexp.MustCompile(`(?s)serverinfo\s*\{[^}]*\};`)
	contentStr = string(serverInfoRegex.ReplaceAllLiteral([]byte(contentStr), []byte(serverInfo)))

	// Configure uplink
	uplinkConfig := `uplink "irc.localhost" {
	host = "127.0.0.1";
	port = 6667;
	password = "serviceslinkpass";
};`

	// Replace uplink block
	uplinkRegex := regexp.MustCompile(`(?s)uplink\s+"[^"]*"\s*\{[^}]*\};`)
	contentStr = string(uplinkRegex.ReplaceAllLiteral([]byte(contentStr), []byte(uplinkConfig)))

	// Write back
	return os.WriteFile(configPath, []byte(contentStr), 0644)
}

func configureUnrealForServices(buildDir, servicesHost, servicesType string) error {
	confFile := filepath.Join(buildDir, "conf", "unrealircd.conf")

	// Read current config
	content, err := os.ReadFile(confFile)
	if err != nil {
		return fmt.Errorf("reading unrealircd.conf: %w", err)
	}

	contentStr := string(content)

	// Add ulines block if not present
	if !strings.Contains(contentStr, "ulines {") {
		contentStr += fmt.Sprintf(`

ulines {
	%s;
};`, servicesHost)
	}

	// Add link block for services
	linkBlock := fmt.Sprintf(`

link %s {
	incoming {
		mask *;
	}
	outgoing {
		hostname 127.0.0.1;
		port 6667;
		options { autoconnect; };
	}
	password "serviceslinkpass" { spkifp; }
	class servers;
};`, servicesHost)

	contentStr += linkBlock

	// Write back
	return os.WriteFile(confFile, []byte(contentStr), 0644)
}

func installAthemeServices(app *tview.Application, pages *tview.Pages, buildDir string) {
	// Show progress modal for Atheme installation
	modal := tview.NewModal().
		SetText("Installing Atheme Services...\n\nThis may take several minutes.").
		AddButtons([]string{}).
		SetDoneFunc(func(int, string) {})
	pages.AddPage("atheme_install_modal", modal, true, true)

	go func() {
		updateProgress := func(text string) {
			app.QueueUpdateDraw(func() {
				modal.SetText(text)
			})
		}

		// Create services directory
		usr, _ := user.Current()
		servicesDir := filepath.Join(usr.HomeDir, "atheme-services")
		updateProgress("Creating services directory...")

		if err := os.MkdirAll(servicesDir, 0755); err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("atheme_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to create services directory: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("atheme_error_modal")
					})
				pages.AddPage("atheme_error_modal", errorModal, true, true)
			})
			return
		}

		// Download latest Atheme version
		updateProgress("Fetching latest Atheme version...")
		resp, err := makeHTTPRequest("https://api.github.com/repos/atheme/atheme/releases/latest")
		if err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("atheme_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to fetch Atheme releases: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("atheme_error_modal")
					})
				pages.AddPage("atheme_error_modal", errorModal, true, true)
			})
			return
		}
		defer resp.Body.Close()

		var release struct {
			TagName    string `json:"tag_name"`
			TarballURL string `json:"tarball_url"`
			Assets     []struct {
				Name        string `json:"name"`
				DownloadURL string `json:"browser_download_url"`
			} `json:"assets"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("atheme_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to parse release info: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("atheme_error_modal")
					})
				pages.AddPage("atheme_error_modal", errorModal, true, true)
			})
			return
		}

		// Use the tarball URL from GitHub
		downloadURL := release.TarballURL
		if downloadURL == "" {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("atheme_install_modal")
				errorModal := tview.NewModal().
					SetText("Could not find Atheme source download").
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("atheme_error_modal")
					})
				pages.AddPage("atheme_error_modal", errorModal, true, true)
			})
			return
		}

		updateProgress(fmt.Sprintf("Downloading Atheme %s...", release.TagName))

		// Download and extract
		resp, err = makeHTTPRequest(downloadURL)
		if err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("atheme_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to download Atheme: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("atheme_error_modal")
					})
				pages.AddPage("atheme_error_modal", errorModal, true, true)
			})
			return
		}
		defer resp.Body.Close()

		// Extract tar.gz
		gzr, err := gzip.NewReader(resp.Body)
		if err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("atheme_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to create gzip reader: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("atheme_error_modal")
					})
				pages.AddPage("atheme_error_modal", errorModal, true, true)
			})
			return
		}
		defer gzr.Close()

		tr := tar.NewReader(gzr)
		var topLevelDir string
		for {
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("atheme_install_modal")
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Failed to read tar: %v", err)).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("atheme_error_modal")
						})
					pages.AddPage("atheme_error_modal", errorModal, true, true)
				})
				return
			}

			// Find the top-level directory
			if header.Typeflag == tar.TypeDir && topLevelDir == "" {
				topLevelDir = header.Name
				continue
			}

			// Skip if we haven't found the top-level dir yet
			if topLevelDir == "" {
				continue
			}

			target := filepath.Join(servicesDir, strings.TrimPrefix(header.Name, topLevelDir))
			if header.Typeflag == tar.TypeDir {
				if err := os.MkdirAll(target, 0755); err != nil {
					app.QueueUpdateDraw(func() {
						pages.RemovePage("atheme_install_modal")
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Failed to create directory: %v", err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("atheme_error_modal")
							})
						pages.AddPage("atheme_error_modal", errorModal, true, true)
					})
					return
				}
			} else {
				parentDir := filepath.Dir(target)
				if err := os.MkdirAll(parentDir, 0755); err != nil {
					app.QueueUpdateDraw(func() {
						pages.RemovePage("atheme_install_modal")
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Failed to create parent directory: %v", err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("atheme_error_modal")
							})
						pages.AddPage("atheme_error_modal", errorModal, true, true)
					})
					return
				}
				f, err := os.Create(target)
				if err != nil {
					app.QueueUpdateDraw(func() {
						pages.RemovePage("atheme_install_modal")
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Failed to create file: %v", err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("atheme_error_modal")
							})
						pages.AddPage("atheme_error_modal", errorModal, true, true)
					})
					return
				}
				if _, err := io.Copy(f, tr); err != nil {
					f.Close()
					app.QueueUpdateDraw(func() {
						pages.RemovePage("atheme_install_modal")
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Failed to write file: %v", err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("atheme_error_modal")
							})
						pages.AddPage("atheme_error_modal", errorModal, true, true)
					})
					return
				}
				f.Close()
			}
		}

		updateProgress("Configuring Atheme...")

		// Make scripts executable
		autogenPath := filepath.Join(servicesDir, "autogen.sh")
		configurePath := filepath.Join(servicesDir, "configure")
		os.Chmod(autogenPath, 0755) // Ignore errors if file doesn't exist
		os.Chmod(configurePath, 0755) // Ignore errors if file doesn't exist

		// Run autogen/configure
		cmd := exec.Command("./autogen.sh")
		cmd.Dir = servicesDir
		if _, err := cmd.CombinedOutput(); err != nil {
			// Try configure directly if autogen fails
			cmd = exec.Command("./configure", "--prefix="+servicesDir)
			cmd.Dir = servicesDir
			if output, err := cmd.CombinedOutput(); err != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("atheme_install_modal")
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Configure failed: %v\nOutput: %s", err, string(output))).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("atheme_error_modal")
						})
					pages.AddPage("atheme_error_modal", errorModal, true, true)
				})
				return
			}
		} else {
			// Run configure after autogen
			cmd = exec.Command("./configure", "--prefix="+servicesDir)
			cmd.Dir = servicesDir
			if output, err := cmd.CombinedOutput(); err != nil {
				app.QueueUpdateDraw(func() {
					pages.RemovePage("atheme_install_modal")
					errorModal := tview.NewModal().
						SetText(fmt.Sprintf("Configure failed: %v\nOutput: %s", err, string(output))).
						AddButtons([]string{"OK"}).
						SetDoneFunc(func(int, string) {
							pages.RemovePage("atheme_error_modal")
						})
					pages.AddPage("atheme_error_modal", errorModal, true, true)
				})
				return
			}
		}

		updateProgress("Building Atheme...")

		// Run make
		cmd = exec.Command("make")
		cmd.Dir = servicesDir
		if output, err := cmd.CombinedOutput(); err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("atheme_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Make failed: %v\nOutput: %s", err, string(output))).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("atheme_error_modal")
					})
				pages.AddPage("atheme_error_modal", errorModal, true, true)
			})
			return
		}

		updateProgress("Installing Atheme...")

		// Run make install
		cmd = exec.Command("make", "install")
		cmd.Dir = servicesDir
		if output, err := cmd.CombinedOutput(); err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("atheme_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Make install failed: %v\nOutput: %s", err, string(output))).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("atheme_error_modal")
					})
				pages.AddPage("atheme_error_modal", errorModal, true, true)
			})
			return
		}

		updateProgress("Configuring services...")

		// Configure atheme.conf
		athemeConfPath := filepath.Join(servicesDir, "etc", "atheme.conf")
		if err := configureAthemeServices(athemeConfPath, buildDir); err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("atheme_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to configure services: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("atheme_error_modal")
					})
				pages.AddPage("atheme_error_modal", errorModal, true, true)
			})
			return
		}

		updateProgress("Configuring UnrealIRCd linking...")

		// Configure UnrealIRCd for services linking
		if err := configureUnrealForServices(buildDir, "services.localhost", "atheme"); err != nil {
			app.QueueUpdateDraw(func() {
				pages.RemovePage("atheme_install_modal")
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to configure UnrealIRCd: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("atheme_error_modal")
					})
				pages.AddPage("atheme_error_modal", errorModal, true, true)
			})
			return
		}

		// Success
		app.QueueUpdateDraw(func() {
			pages.RemovePage("atheme_install_modal")
			successModal := tview.NewModal().
				SetText(fmt.Sprintf("Atheme Services %s installed successfully!\n\nServices directory: %s\n\nTo start services: cd %s && ./bin/atheme-services\n\nDon't forget to rehash UnrealIRCd after starting services.", release.TagName, servicesDir, servicesDir)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("atheme_success_modal")
				})
			pages.AddPage("atheme_success_modal", successModal, true, true)
		})
	}()
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
		AddItem(ui.CreateFooter("ESC: Back | Ctrl+S: Save | q: Quit"), 3, 0, false)

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
	flex.AddItem(createHeader(), 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(ui.CreateFooter("ESC: Main Menu | Enter: Select | q: Quit"), 3, 0, false)
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
	flex.AddItem(createHeader(), 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(ui.CreateFooter("ESC: Main Menu | Enter: Select | q: Quit"), 3, 0, false)
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
	flex.AddItem(createHeader(), 3, 0, false).AddItem(scriptsFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(ui.CreateFooter("ESC: Main Menu | Enter: Select | q: Quit"), 3, 0, false)
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
	flex.AddItem(createHeader(), 3, 0, false).AddItem(textArea, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(ui.CreateFooter("Ctrl+S: Save | Ctrl+X: Cancel | ESC: Cancel"), 3, 0, false)
	pages.AddPage("edit_script", flex, true, true)
	editScriptFocusables = []tview.Primitive{textArea, saveBtn, previewBtn, cancelBtn}
	app.SetFocus(textArea)
}
