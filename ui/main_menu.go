package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// MainMenuPage creates the main menu page
func MainMenuPage(app *tview.Application, pages *tview.Pages, sourceDir, buildDir string) {
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
	// list.AddItem("• ObbyScript", "  Manage ObbyScript installation and scripts", 0, nil)
	list.AddItem("• Dev Tools", "  Developer tools and utilities", 0, nil)

	// Note: currentList is a global variable that needs to be accessible
	// This will be handled when we move globals

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
				ConfigurationMenuPage(app, pages, buildDir)
			case "• Utilities":
				UtilitiesPage(app, pages, buildDir)
			case "• Module Manager":
				moduleManagerSubmenuPage(app, pages, sourceDir, buildDir)
			case "• Check for Updates":
				checkForUpdatesPage(app, pages, sourceDir, buildDir)
			case "• Installation Options":
				installationOptionsPage(app, pages, sourceDir, buildDir)
			case "• Remote Control (RPC)":
				RemoteControlMenuPage(app, pages, buildDir)
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
			ConfigurationMenuPage(app, pages, buildDir)
		case "• Utilities":
			UtilitiesPage(app, pages, buildDir)
		case "• Module Manager":
			moduleManagerSubmenuPage(app, pages, sourceDir, buildDir)
		case "• Check for Updates":
			checkForUpdatesPage(app, pages, sourceDir, buildDir)
		case "• Installation Options":
			installationOptionsPage(app, pages, sourceDir, buildDir)
		case "• Remote Control (RPC)":
			RemoteControlMenuPage(app, pages, buildDir)
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
	flex.AddItem(header, 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(CreateFooter("ESC: Back | Enter: Select | q: Quit"), 3, 0, false)
	pages.AddPage("main_menu", flex, true, true)
	// mainMenuFocusables = []tview.Primitive{list, textView, quitBtn} // TODO: handle globals
}

// UtilitiesPage creates the utilities page
func UtilitiesPage(app *tview.Application, pages *tview.Pages, buildDir string) {
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
		"configtest":    "Test the configuration file for syntax errors and validity.\n\nThis command checks if your unrealircd.conf and other configuration files are properly formatted and contain no errors before starting the server.",
		"start":         "Start the IRC Server.\n\nLaunches the UnrealIRCd daemon. Make sure the configuration is tested first with configtest.",
		"stop":          "Stop (kill) the IRC Server.\n\nGracefully shuts down the running UnrealIRCd process. All users will be disconnected.",
		"rehash":        "Reload the configuration file.\n\nReloads the configuration without restarting the server. Useful for applying configuration changes while the server is running.",
		"reloadtls":     "Reload the SSL/TLS certificates.\n\nReloads SSL/TLS certificates and keys without restarting the server. Useful when certificates have been renewed.",
		"restart":       "Restart the IRC Server (stop+start).\n\nStops the server and starts it again. All users will be disconnected during the restart.",
		"status":        "Show current status of the IRC Server.\n\nDisplays information about whether the server is running, PID, uptime, and basic statistics.",
		"module-status": "Show all currently loaded modules.\n\nLists all modules that are currently loaded in the running server, including core and third-party modules.",
		"version":       "Display the UnrealIRCd version.\n\nShows the version number, build date, and other version information of the installed UnrealIRCd.",
		"genlinkblock":  "Generate link { } block for the other side.\n\nCreates a sample link block configuration that can be used to connect to another IRC server.",
		"gencloak":      "Display 3 random cloak keys.\n\nGenerates random cloak keys that can be used in the configuration for hostname cloaking.",
		"spkifp":        "Display SPKI Fingerprint.\n\nShows the SPKI (Subject Public Key Info) fingerprint of the server's SSL/TLS certificate.",
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
	flex.AddItem(createHeader(), 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(CreateFooter("ESC: Main Menu | Enter: Execute Command | q: Quit"), 3, 0, false)
	pages.AddPage("utilities", flex, true, true)
	// utilitiesFocusables = []tview.Primitive{list, outputView, backBtn, runBtn} // TODO: handle globals
	app.SetFocus(list)

	// Set initial description
	if len(utilities) > 0 {
		if desc, ok := descriptions[utilities[0]]; ok {
			outputView.SetText(desc)
		}
	}
}

// moduleManagerSubmenuPage creates the module manager submenu page
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

	// currentList = list // TODO: handle global

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
				CheckModulesPage(app, pages, buildDir, sourceDir)
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
			CheckModulesPage(app, pages, buildDir, sourceDir)
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
	flex.AddItem(header, 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(CreateFooter("ESC: Back | Enter: Select | q: Quit"), 3, 0, false)
	pages.AddPage("module_manager_submenu", flex, true, true)
	// moduleManagerSubmenuFocusables = []tview.Primitive{list, textView, backBtn} // TODO: handle globals
}

// uploadCustomModulePage creates the upload custom module page
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
		AddItem(CreateFooter("ESC: Back | Ctrl+S: Save | q: Quit"), 3, 0, false)

	pages.AddPage("upload_custom_module", flex, true, true)
	app.SetFocus(textArea)
}

// thirdPartyBrowserPage creates the third-party module browser page
func thirdPartyBrowserPage(app *tview.Application, pages *tview.Pages, sourceDir, buildDir string) {
	// Placeholder implementation - needs module fetching logic
	textView := &FocusableTextView{tview.NewTextView()}
	textView.SetBorder(true).SetTitle("Module Details")
	textView.SetDynamicColors(true)
	textView.SetWordWrap(true)
	textView.SetScrollable(true)
	textView.SetText("Third-party module browser - implementation pending module utilities migration")

	list := tview.NewList()
	list.SetBorder(true).SetTitle("Third-Party Modules")
	list.AddItem("Feature not yet implemented", "Module utilities need to be migrated", 0, nil)

	header := createHeader()
	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.SwitchToPage("main_menu")
	})
	buttonBar := createButtonBar(backBtn)

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	browserFlex := tview.NewFlex().
		AddItem(list, 80, 0, true).
		AddItem(textView, 0, 1, false)
	flex.AddItem(header, 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(CreateFooter("ESC: Main Menu | Enter: Select | q: Quit"), 3, 0, false)
	pages.AddPage("third_party_browser", flex, true, true)
}

// checkForUpdatesPage creates the check for updates page
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

// installationOptionsPage creates the installation options page
func installationOptionsPage(app *tview.Application, pages *tview.Pages, sourceDir, buildDir string) {
	// Placeholder implementation - needs source directory scanning logic
	textView := &FocusableTextView{tview.NewTextView()}
	textView.SetBorder(true).SetTitle("Installation Options")
	textView.SetDynamicColors(true)
	textView.SetWordWrap(true)
	textView.SetScrollable(true)
	textView.SetText("Installation options - implementation pending source directory scanning utilities migration")

	header := createHeader()
	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.SwitchToPage("main_menu")
	})
	buttonBar := createButtonBar(backBtn)

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	flex.AddItem(header, 3, 0, false).AddItem(textView, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(CreateFooter("ESC: Main Menu | q: Quit"), 3, 0, false)
	pages.AddPage("installation_options", flex, true, true)
}

// devToolsSubmenuPage creates the dev tools submenu page
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

	// currentList = list // TODO: handle global

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
	flex.AddItem(header, 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(CreateFooter("ESC: Back | Enter: Select | q: Quit"), 3, 0, false)
	pages.AddPage("dev_tools_submenu", flex, true, true)
	app.SetFocus(list)
}

// testsSubmenuPage creates the tests submenu page
func testsSubmenuPage(app *tview.Application, pages *tview.Pages, sourceDir, buildDir string) {
	// Placeholder implementation - needs fleet management logic
	textView := &FocusableTextView{tview.NewTextView()}
	textView.SetBorder(true).SetTitle("Tests")
	textView.SetDynamicColors(true)
	textView.SetWordWrap(true)
	textView.SetScrollable(true)
	textView.SetText("Tests submenu - implementation pending fleet management utilities migration")

	list := tview.NewList()
	list.SetBorder(true).SetBorderColor(tcell.ColorYellow)
	list.SetTitle("Tests")
	list.AddItem("• Test Fleet", "  Create a test fleet of linked UnrealIRCd servers", 0, nil)
	list.AddItem("• Manage Fleet", "  Start and stop test fleet servers", 0, nil)

	header := createHeader()
	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		pages.RemovePage("tests_submenu")
		pages.SwitchToPage("dev_tools_submenu")
	})
	buttonBar := createButtonBar(backBtn)

	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	browserFlex := tview.NewFlex().
		AddItem(list, 40, 0, true).
		AddItem(textView, 0, 1, false)
	flex.AddItem(header, 3, 0, false).AddItem(browserFlex, 0, 1, true).AddItem(buttonBar, 3, 0, false).AddItem(CreateFooter("ESC: Back | Enter: Select | q: Quit"), 3, 0, false)
	pages.AddPage("tests_submenu", flex, true, true)
}