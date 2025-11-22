package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"utui/rpc"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func CreateFooter(shortcuts string) *tview.TextView {
	footer := tview.NewTextView()
	footer.SetText(shortcuts)
	footer.SetTextAlign(tview.AlignCenter)
	footer.SetBorder(true)
	return footer
}

func parseLogTimestamp(timestampStr string) time.Time {
	// Debug: log the timestamp string we're trying to parse
	debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	fmt.Fprintf(debugFile, "[DEBUG] Parsing timestamp: '%s'\n", timestampStr)
	debugFile.Close()

	// Try RFC3339 with nanoseconds first (format like "2025-11-10T02:31:41.077Z")
	if t, err := time.Parse(time.RFC3339Nano, timestampStr); err == nil {
		debugFile, _ = os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		fmt.Fprintf(debugFile, "[DEBUG] Successfully parsed as RFC3339Nano: %v\n", t)
		debugFile.Close()
		return t
	}

	// Try RFC3339
	if t, err := time.Parse(time.RFC3339, timestampStr); err == nil {
		debugFile, _ = os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		fmt.Fprintf(debugFile, "[DEBUG] Successfully parsed as RFC3339: %v\n", t)
		debugFile.Close()
		return t
	}

	// Try other formats if needed
	if t, err := time.Parse("2006-01-02T15:04:05.000Z", timestampStr); err == nil {
		return t
	}

	// If all parsing fails, return current time
	debugFile, _ = os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	fmt.Fprintf(debugFile, "[DEBUG] Failed to parse timestamp '%s', using current time\n", timestampStr)
	debugFile.Close()
	return time.Now()
}

// sortLogEntries sorts log entries by timestamp (oldest first)
func sortLogEntries(entries []*rpc.FileLogEntry) {
	sort.Slice(entries, func(i, j int) bool {
		timeI := parseLogTimestamp(entries[i].Timestamp)
		timeJ := parseLogTimestamp(entries[j].Timestamp)
		return timeI.Before(timeJ)
	})
}

// formatJSONTree formats JSON data in a tree-like structure with indentation and colors
func formatJSONTree(data interface{}, indent string) string {
	var result strings.Builder

	switch v := data.(type) {
	case map[string]interface{}:
		// Sort keys for consistent output
		var keys []string
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, key := range keys {
			result.WriteString(indent)

			// Check if value is a primitive (string, number, bool)
			if isPrimitive(v[key]) {
				// Format as key-value pair with colors
				result.WriteString(fmt.Sprintf("[yellow]%s[-]\t", key))
				switch val := v[key].(type) {
				case string:
					result.WriteString(fmt.Sprintf("[green]\"%s\"[-]", val))
				case bool:
					if val {
						result.WriteString("[cyan]true[-]")
					} else {
						result.WriteString("[cyan]false[-]")
					}
				default:
					result.WriteString(fmt.Sprintf("[cyan]%v[-]", val))
				}
				result.WriteString("\n")
			} else {
				// Value is an object or array, put key on its own line
				result.WriteString(fmt.Sprintf("[yellow]%s[-]\n", key))

				nextIndent := indent + "  "
				result.WriteString(formatJSONTree(v[key], nextIndent))
			}
		}
	case []interface{}:
		for _, item := range v {
			result.WriteString(indent)
			if isPrimitive(item) {
				// Format primitive array items with colors
				switch val := item.(type) {
				case string:
					result.WriteString(fmt.Sprintf("[red]\"%s\"[-]", val))
				case bool:
					if val {
						result.WriteString("[blue]true[-]")
					} else {
						result.WriteString("[blue]false[-]")
					}
				default:
					result.WriteString(fmt.Sprintf("[blue]%v[-]", val))
				}
			} else {
				// Nested object/array in array
				result.WriteString(formatJSONTree(item, indent+"  "))
			}
			result.WriteString("\n")
		}
	default:
		// This shouldn't happen in normal JSON, but handle primitives just in case
		switch val := v.(type) {
		case string:
			result.WriteString(fmt.Sprintf("%s[green]\"%s\"[-]\n", indent, val))
		default:
			result.WriteString(fmt.Sprintf("%s[cyan]%v[-]\n", indent, val))
		}
	}

	return result.String()
}

// isPrimitive checks if a value is a JSON primitive (string, number, bool, null)
func isPrimitive(value interface{}) bool {
	switch value.(type) {
	case string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
		return true
	case nil:
		return true
	default:
		return false
	}
}

func RemoteControlMenuPage(app *tview.Application, pages *tview.Pages, buildDir string) {
	config, err := rpc.LoadRPCConfig()

	if err != nil {
		// Error loading config
		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Error loading RPC config: %v", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("rpc_error_modal")
			})
		pages.AddPage("rpc_error_modal", errorModal, true, true)
	} else if config == nil {
		// No config exists, show setup form in modal
		showRPCSetupModal(app, pages, buildDir)
	} else {
		// Config exists, show main remote control menu
		flex := tview.NewFlex().SetDirection(tview.FlexRow)
		showRemoteControlOptions(app, pages, config, flex, buildDir)
		pages.AddPage("remote_control_menu", flex, true, true)
	}
}

func showRPCSetupModal(app *tview.Application, pages *tview.Pages, buildDir string) {
	setupForm := tview.NewForm()
	setupForm.SetBorder(true).SetTitle("RPC Configuration Setup")
	setupForm.SetBackgroundColor(tcell.ColorDefault)

	setupForm.AddInputField("Username:", "", 30, nil, nil)
	setupForm.AddPasswordField("Password:", "", 30, '*', nil)
	setupForm.AddInputField("WebSocket URL:", "wss://127.0.0.1:8600/", 40, nil, nil)

	setupForm.AddButton("Test Connection", func() {
		// Get values from form fields
		username := setupForm.GetFormItem(0).(*tview.InputField).GetText()
		password := setupForm.GetFormItem(1).(*tview.InputField).GetText()
		wsURL := setupForm.GetFormItem(2).(*tview.InputField).GetText()

		testConfig := &rpc.RPCConfig{
			Username: username,
			Password: password,
			WSURL:    wsURL,
		}
		if err := rpc.TestRPCConnection(testConfig); err != nil {
			errorModal := tview.NewModal().
				SetText(fmt.Sprintf("Connection test failed: %v", err)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("rpc_test_error_modal")
				})
			pages.AddPage("rpc_test_error_modal", errorModal, true, true)
		} else {
			successModal := tview.NewModal().
				SetText("Connection test successful! Saving configuration...").
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("rpc_success_modal")
					newConfig := &rpc.RPCConfig{
						Username: username,
						Password: password,
						WSURL:    wsURL,
					}
					if err := rpc.SaveRPCConfig(newConfig); err != nil {
						errorModal := tview.NewModal().
							SetText(fmt.Sprintf("Error saving config: %v", err)).
							AddButtons([]string{"OK"}).
							SetDoneFunc(func(int, string) {
								pages.RemovePage("rpc_save_error_modal")
							})
						pages.AddPage("rpc_save_error_modal", errorModal, true, true)
					} else {
						// Refresh the page
						pages.RemovePage("rpc_setup_modal")
						RemoteControlMenuPage(app, pages, buildDir)
						pages.SwitchToPage("remote_control_menu")
					}
				})
			pages.AddPage("rpc_success_modal", successModal, true, true)
		}
	})

	setupForm.AddButton("Save", func() {
		// Get values from form fields
		username := setupForm.GetFormItem(0).(*tview.InputField).GetText()
		password := setupForm.GetFormItem(1).(*tview.InputField).GetText()
		wsURL := setupForm.GetFormItem(2).(*tview.InputField).GetText()

		// Basic validation
		if username == "" || password == "" || wsURL == "" {
			errorModal := tview.NewModal().
				SetText("All fields are required. Please fill in username, password, and WebSocket URL.").
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("rpc_validation_error_modal")
				})
			pages.AddPage("rpc_validation_error_modal", errorModal, true, true)
			return
		}

		newConfig := &rpc.RPCConfig{
			Username: username,
			Password: password,
			WSURL:    wsURL,
		}
		if err := rpc.SaveRPCConfig(newConfig); err != nil {
			errorModal := tview.NewModal().
				SetText(fmt.Sprintf("Error saving config: %v", err)).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("rpc_save_error_modal")
				})
			pages.AddPage("rpc_save_error_modal", errorModal, true, true)
		} else {
			successModal := tview.NewModal().
				SetText("Configuration saved successfully!").
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(int, string) {
					pages.RemovePage("rpc_save_success_modal")
					// Refresh the page
					pages.RemovePage("rpc_setup_modal")
					RemoteControlMenuPage(app, pages, buildDir)
					pages.SwitchToPage("remote_control_menu")
				})
			pages.AddPage("rpc_save_success_modal", successModal, true, true)
		}
	})

	setupForm.AddButton("Cancel", func() {
		pages.RemovePage("rpc_setup_modal")
		pages.SwitchToPage("main_menu")
	})

	// Set button alignment to center
	setupForm.SetButtonsAlign(tview.AlignCenter)

	// Create centered modal layout
	formHeight := 12 // Approximate height for form with inputs and buttons
	formWidth := 60  // Approximate width for form

	centeredFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView(), 0, 1, false).
		AddItem(tview.NewFlex().
			AddItem(tview.NewTextView(), 0, 1, false).
			AddItem(setupForm, formWidth, 0, true).
			AddItem(tview.NewTextView(), 0, 1, false), formHeight, 0, true).
		AddItem(tview.NewTextView(), 0, 1, false)

	pages.AddPage("rpc_setup_modal", centeredFlex, true, true)
}

func showRemoteControlOptions(app *tview.Application, pages *tview.Pages, config *rpc.RPCConfig, flex *tview.Flex, buildDir string) {
	// Left: Menu list
	list := tview.NewList()
	list.SetBorder(true)
	list.SetTitle("Remote Control")
	list.SetBorderColor(tcell.ColorGreen)

	list.AddItem("• Channels", "  View and manage channels", 0, nil)
	list.AddItem("• Users", "  View and manage users", 0, nil)
	list.AddItem("• Servers", "  View server information", 0, func() {
		remoteServersPage(app, pages, config)
	})
	list.AddItem("• Server Bans", "  View and manage bans (G-lines, K-lines, etc)", 0, func() {
		remoteServerBansPage(app, pages, config)
	})
	list.AddItem("• Log Streaming", "  Stream server logs in real-time", 0, func() {
		remoteLogStreamingPage(app, pages, config, buildDir)
	})
	list.AddItem("• Configure RPC", "  Update RPC credentials", 0, func() {
		reconfigureRPC(app, pages, buildDir)
	})

	// Right: Dynamic content area
	contentArea := tview.NewFlex().SetDirection(tview.FlexRow)

	// Default info view
	infoView := tview.NewTextView()
	infoView.SetBorder(true)
	infoView.SetTitle("Information")
	infoView.SetDynamicColors(true)
	infoView.SetWordWrap(true)
	infoView.SetText(
		"Select an option to view or manage:\n\n" +
			"• [green]Channels[-] - View all channels, topics, and member lists\n" +
			"• [green]Users[-] - View online users and their details\n" +
			"• [green]Servers[-] - View server information and statistics\n" +
			"• [green]Server Bans[-] - Manage G-lines, K-lines, and Z-lines\n" +
			"• [green]Log Streaming[-] - Stream server logs in real-time\n" +
			"• [green]Configure RPC[-] - Update your connection settings")

	// Users list view
	usersList := tview.NewList()
	usersList.SetBorder(true)
	usersList.SetTitle("Users")
	usersList.SetBorderColor(tcell.ColorBlue)

	// User details view
	userDetailsView := tview.NewTextView()
	userDetailsView.SetBorder(true)
	userDetailsView.SetTitle("User Details")
	userDetailsView.SetDynamicColors(true)
	userDetailsView.SetWordWrap(true)

	// Channels list view
	channelsList := tview.NewList()
	channelsList.SetBorder(true)
	channelsList.SetTitle("Channels")
	channelsList.SetBorderColor(tcell.ColorBlue)

	// Channel details view
	channelDetailsView := tview.NewTextView()
	channelDetailsView.SetBorder(true)
	channelDetailsView.SetTitle("Channel Details")
	channelDetailsView.SetDynamicColors(true)
	channelDetailsView.SetWordWrap(true)

	// Initially show info view
	contentArea.AddItem(infoView, 0, 1, false)

	list.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		// Clear current content
		contentArea.Clear()

		switch mainText {
		case "• Channels":
			// Show channels list and details
			loadChannelsList(app, channelsList, channelDetailsView, config)
			channelsFlex := tview.NewFlex()
			channelsFlex.AddItem(channelsList, 0, 1, true)
			channelsFlex.AddItem(channelDetailsView, 0, 1, false)
			contentArea.AddItem(channelsFlex, 0, 1, false)
		case "• Users":
			// Show users list and details
			loadUsersList(app, usersList, userDetailsView, config)
			userFlex := tview.NewFlex()
			userFlex.AddItem(usersList, 0, 1, true)
			userFlex.AddItem(userDetailsView, 0, 1, false)
			contentArea.AddItem(userFlex, 0, 1, false)
		default:
			// Show info for other items
			descriptions := map[string]string{
				"• Channels":      "Display all channels on the network with topic, user count, and modes.",
				"• Users":         "List all connected users with nick, realname, account, and channel memberships.",
				"• Servers":       "Show server information including uptime, software version, and user count.",
				"• Server Bans":   "View and manage all active bans (G-lines, K-lines, Z-lines, etc).",
				"• Configure RPC": "Update your RPC API credentials for UnrealIRCd connection.",
			}
			if desc, ok := descriptions[mainText]; ok {
				infoView.SetText(desc)
			}
			contentArea.AddItem(infoView, 0, 1, false)
		}
	})

	// Auto-select Channels for debugging
	list.SetCurrentItem(0) // Index 0 is "Channels"

	backBtn := tview.NewButton("Back")
	backBtn.SetSelectedFunc(func() {
		pages.SwitchToPage("main_menu")
	})

	buttonBar := tview.NewFlex()
	buttonBar.AddItem(tview.NewTextView(), 0, 1, false)
	buttonBar.AddItem(backBtn, 20, 0, false)
	buttonBar.AddItem(tview.NewTextView(), 0, 1, false)

	contentFlex := tview.NewFlex()
	contentFlex.AddItem(list, 30, 0, true)
	contentFlex.AddItem(contentArea, 0, 1, false)

	flex.AddItem(tview.NewTextView().SetText(""), 1, 0, false)
	flex.AddItem(contentFlex, 0, 1, true)
	flex.AddItem(buttonBar, 3, 0, false)
}

func loadUsersList(app *tview.Application, usersList *tview.List, userDetailsView *tview.TextView, config *rpc.RPCConfig) {
	// Debug: write to file
	debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	fmt.Fprintf(debugFile, "DEBUG: loadUsersList called\n")
	debugFile.Close()

	// Clear existing items
	usersList.Clear()

	// Show loading message
	userDetailsView.SetText("Loading users...")

	// Fetch users synchronously to avoid UI corruption
	client, err := rpc.NewRPCClient(config)
	if err != nil {
		userDetailsView.SetText(fmt.Sprintf("Error creating RPC client: %v", err))
		return
	}
	defer client.Close()

	if err := client.Connect(); err != nil {
		userDetailsView.SetText(fmt.Sprintf("Error connecting to RPC server: %v", err))
		return
	}

	users, err := client.GetUsers()
	if err != nil {
		userDetailsView.SetText(fmt.Sprintf("Error fetching users: %v", err))
		return
	}

	// Debug: also get details for the first user
	if len(users) > 0 {
		_, err := client.GetUserDetails(users[0].Nick)
		if err != nil {
			// fmt.Printf("DEBUG: GetUserDetails error: %v\n", err)
		}
	}

	// Debug: show what we got
	debugText := fmt.Sprintf("DEBUG: Got %d users\n", len(users))
	for i, user := range users {
		debugText += fmt.Sprintf("User %d: Nick='%s', Channels=%v\n", i, user.Nick, user.Channels)
	}
	if len(users) == 0 {
		debugText += "No users found (empty list)"
	}
	// fmt.Printf("DEBUG UI: %s\n", debugText) // Print to console for debugging

	// Update UI synchronously
	// userDetailsView.SetText(debugText)
	userDetailsView.SetText("Select a user to view details.")

	for _, user := range users {
		// Skip empty/invalid users
		if user.Nick == "" && user.Name == "" {
			continue
		}

		displayName := user.Nick
		if user.Account != "" {
			displayName += fmt.Sprintf(" (%s)", user.Account)
		}
		if displayName == "" {
			displayName = user.Name // fallback to name if nick is empty
		}
		if displayName == "" {
			displayName = "Unknown" // fallback if both are empty
		}
		secondaryText := fmt.Sprintf("  %s (%s) - %d channels", user.Realname, user.IP, len(user.Channels))

		usersList.AddItem(displayName, secondaryText, 0, nil)
	}

	// Set current item to first user
	if len(users) > 0 {
		usersList.SetCurrentItem(0)
	}

	// Set up selection handler for user details
	usersList.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		if index >= 0 && index < len(users) {
			user := users[index]

			// Format account display
			accountDisplay := user.Account
			if accountDisplay == "" || accountDisplay == "none" {
				accountDisplay = "None (not logged in)"
			}

			// Format channels
			channelsStr := ""
			if len(user.Channels) > 0 {
				var colored []string
				for _, ch := range user.Channels {
					colored = append(colored, "  [blue]"+ch+"[white]")
				}
				channelsStr = "\n" + strings.Join(colored, "\n")
			} else {
				channelsStr = "\n  None"
			}

			// Format security groups
			securityGroupsStr := ""
			if len(user.SecurityGroups) > 0 {
				var colored []string
				for _, sg := range user.SecurityGroups {
					colored = append(colored, "  [blue]"+sg+"[white]")
				}
				securityGroupsStr = "\n" + strings.Join(colored, "\n")
			} else {
				securityGroupsStr = "\n  None"
			}

			details := fmt.Sprintf(
				"[green]Nick:[white]\n  %s\n"+
					"[green]Real Name:[white]\n  %s\n"+
					"[green]Account:[white]\n  %s\n"+
					"[green]IP:[white]\n  %s\n"+
					"[green]Username:[white]\n  %s\n"+
					"[green]Vhost:[white]\n  %s\n"+
					"[green]Cloaked Host:[white]\n  %s\n"+
					"[green]Server Name:[white]\n  %s\n"+
					"[green]Reputation:[white]\n  %d\n"+
					"[green]Modes:[white]\n  %s\n"+
					"[green]Security Groups:[white]%s\n"+
					"[green]Channels:[white]%s",
				user.Nick, user.Realname, accountDisplay, user.IP, user.Username, user.Vhost, user.Cloakedhost, user.Servername, user.Reputation, user.Modes, securityGroupsStr, channelsStr)
			userDetailsView.SetText(details)
		}
	})

	// Show first user details by default
	if len(users) > 0 {
		user := users[0]

		// Format account display
		accountDisplay := user.Account
		if accountDisplay == "" || accountDisplay == "none" {
			accountDisplay = "None (not logged in)"
		}

		// Format channels
		channelsStr := ""
		if len(user.Channels) > 0 {
			var colored []string
			for _, ch := range user.Channels {
				colored = append(colored, "  [blue]"+ch+"[white]")
			}
			channelsStr = "\n" + strings.Join(colored, "\n")
		} else {
			channelsStr = "\n  None"
		}

		// Format security groups
		securityGroupsStr := ""
		if len(user.SecurityGroups) > 0 {
			var colored []string
			for _, sg := range user.SecurityGroups {
				colored = append(colored, "  [blue]"+sg+"[white]")
			}
			securityGroupsStr = "\n" + strings.Join(colored, "\n")
		} else {
			securityGroupsStr = "\n  None"
		}

		details := fmt.Sprintf(
			"[green]Nick:[white]\n  %s\n"+
				"[green]Real Name:[white]\n  %s\n"+
				"[green]Account:[white]\n  %s\n"+
				"[green]IP:[white]\n  %s\n"+
				"[green]Username:[white]\n  %s\n"+
				"[green]Vhost:[white]\n  %s\n"+
				"[green]Cloaked Host:[white]\n  %s\n"+
				"[green]Server Name:[white]\n  %s\n"+
				"[green]Reputation:[white]\n  %d\n"+
				"[green]Modes:[white]\n  %s\n"+
				"[green]Security Groups:[white]%s\n"+
				"[green]Channels:[white]%s",
			user.Nick, user.Realname, accountDisplay, user.IP, user.Username, user.Vhost, user.Cloakedhost, user.Servername, user.Reputation, user.Modes, securityGroupsStr, channelsStr)
		userDetailsView.SetText(details)
	}
}

func loadChannelsList(app *tview.Application, channelsList *tview.List, channelDetailsView *tview.TextView, config *rpc.RPCConfig) {
	// Clear existing items
	channelsList.Clear()

	// Show loading message
	channelDetailsView.SetText("Loading channels...")

	// Fetch channels synchronously
	client, err := rpc.NewRPCClient(config)
	if err != nil {
		channelDetailsView.SetText(fmt.Sprintf("Error creating RPC client: %v", err))
		return
	}
	defer client.Close()

	if err := client.Connect(); err != nil {
		channelDetailsView.SetText(fmt.Sprintf("Error connecting to RPC server: %v", err))
		return
	}

	channels, err := client.GetChannels()
	if err != nil {
		channelDetailsView.SetText(fmt.Sprintf("Error fetching channels: %v", err))
		return
	}

	// Update UI
	channelDetailsView.SetText("Select a channel to view details.")

	for _, channel := range channels {
		displayName := channel.Name
		if channel.Topic != "" {
			// Truncate long topics
			topic := channel.Topic
			if len(topic) > 50 {
				topic = topic[:47] + "..."
			}
			displayName += fmt.Sprintf(" - %s", topic)
		}
		secondaryText := fmt.Sprintf("  %d users, modes: %s", channel.UserCount, channel.Modes)

		channelsList.AddItem(displayName, secondaryText, 0, nil)
	}

	// Set current item to first channel
	if len(channels) > 0 {
		channelsList.SetCurrentItem(0)
	}

	// Set up selection handler for channel details
	channelsList.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		if index >= 0 && index < len(channels) {
			channel := channels[index]

			// Show loading message
			channelDetailsView.SetText("Loading channel details...")

			// Get detailed channel info in background
			go func() {
				client, err := rpc.NewRPCClient(config)
				if err != nil {
					app.QueueUpdateDraw(func() {
						channelDetailsView.SetText(fmt.Sprintf("Error creating RPC client: %v", err))
					})
					return
				}
				defer client.Close()

				if err := client.Connect(); err != nil {
					app.QueueUpdateDraw(func() {
						channelDetailsView.SetText(fmt.Sprintf("Error connecting to RPC server: %v", err))
					})
					return
				}

				detailedChannel, err := client.GetChannelDetails(channel.Name)
				if err != nil {
					app.QueueUpdateDraw(func() {
						channelDetailsView.SetText(fmt.Sprintf("Error getting channel details: %v", err))
					})
					return
				}

				// Format users list
				usersStr := ""
				if len(detailedChannel.Users) > 0 {
					var colored []string
					for _, user := range detailedChannel.Users {
						colored = append(colored, "  [blue]"+user+"[white]")
					}
					usersStr = "\n" + strings.Join(colored, "\n")
				} else {
					usersStr = "\n  None"
				}

				details := fmt.Sprintf(
					"[green]Name:[white]\n  %s\n"+
						"[green]Topic:[white]\n  %s\n"+
						"[green]Modes:[white]\n  %s\n"+
						"[green]Created:[white]\n  %s\n"+
						"[green]Users:[white]%s",
					detailedChannel.Name,
					detailedChannel.Topic,
					detailedChannel.Modes,
					time.Unix(detailedChannel.Created, 0).Format("2006-01-02 15:04:05"),
					usersStr)

				// Update UI in main thread
				app.QueueUpdateDraw(func() {
					channelDetailsView.SetText(details)
				})
			}()
		}
	})

	// Show first channel details by default - but need to load detailed info
	if len(channels) > 0 {
		channel := channels[0]

		// Show loading message initially
		channelDetailsView.SetText("Select a channel to view details.")

		// Auto-load first channel details
		go func() {
			client, err := rpc.NewRPCClient(config)
			if err != nil {
				app.QueueUpdateDraw(func() {
					channelDetailsView.SetText(fmt.Sprintf("Error creating RPC client: %v", err))
				})
				return
			}
			defer client.Close()

			if err := client.Connect(); err != nil {
				app.QueueUpdateDraw(func() {
					channelDetailsView.SetText(fmt.Sprintf("Error connecting to RPC server: %v", err))
				})
				return
			}

			detailedChannel, err := client.GetChannelDetails(channel.Name)
			if err != nil {
				app.QueueUpdateDraw(func() {
					channelDetailsView.SetText(fmt.Sprintf("Error getting channel details: %v", err))
				})
				return
			}

			// Format users list
			usersStr := ""
			if len(detailedChannel.Users) > 0 {
				var colored []string
				for _, user := range detailedChannel.Users {
					colored = append(colored, "  [blue]"+user+"[white]")
				}
				usersStr = "\n" + strings.Join(colored, "\n")
			} else {
				usersStr = "\n  None"
			}

			details := fmt.Sprintf(
				"[green]Name:[white]\n  %s\n"+
					"[green]Topic:[white]\n  %s\n"+
					"[green]Modes:[white]\n  %s\n"+
					"[green]Created:[white]\n  %s\n"+
					"[green]Users:[white]%s",
				detailedChannel.Name,
				detailedChannel.Topic,
				detailedChannel.Modes,
				time.Unix(detailedChannel.Created, 0).Format("2006-01-02 15:04:05"),
				usersStr)

			// Update UI in main thread
			app.QueueUpdateDraw(func() {
				channelDetailsView.SetText(details)
			})
		}()
	}
}

func remoteServersPage(app *tview.Application, pages *tview.Pages, config *rpc.RPCConfig) {
	// TODO: Implement servers page
	modal := tview.NewModal().
		SetText("Servers view coming soon...").
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(int, string) {
			pages.RemovePage("servers_page")
		})
	pages.AddPage("servers_page", modal, true, true)
}

func remoteServerBansPage(app *tview.Application, pages *tview.Pages, config *rpc.RPCConfig) {
	// TODO: Implement server bans page
	modal := tview.NewModal().
		SetText("Server Bans view coming soon...").
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(int, string) {
			pages.RemovePage("bans_page")
		})
	pages.AddPage("bans_page", modal, true, true)
}

func reconfigureRPC(app *tview.Application, pages *tview.Pages, buildDir string) {
	// Remove current config and show setup
	rpcConfig, _ := rpc.LoadRPCConfig()
	if rpcConfig != nil {
		// Remove config file
		home, _ := os.UserHomeDir()
		configPath := filepath.Join(home, ".unrealircd_rpc_config")
		os.Remove(configPath)
	}
	pages.RemovePage("remote_control_menu")
	RemoteControlMenuPage(app, pages, buildDir)
}

func remoteLogStreamingPage(app *tview.Application, pages *tview.Pages, config *rpc.RPCConfig, buildDir string) {
	var updateLogDisplay func() // Function to update the log display

	debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	fmt.Fprintf(debugFile, "[DEBUG] remoteLogStreamingPage called\n")
	debugFile.Close()

	client, err := rpc.NewRPCClient(config)
	if err != nil {
		debugFile, _ = os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		fmt.Fprintf(debugFile, "[DEBUG] Failed to create RPC client: %v\n", err)
		debugFile.Close()

		errorModal := tview.NewModal().
			SetText(fmt.Sprintf("Failed to create RPC client: %v", err)).
			AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("rpc_client_error_modal")
				pages.SwitchToPage("remote_control_menu")
			})
		pages.AddPage("rpc_client_error_modal", errorModal, true, true)
		return
	}

	debugFile, _ = os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	fmt.Fprintf(debugFile, "[DEBUG] RPC client created successfully\n")
	debugFile.Close()

	// Create the log streaming page
	flex := tview.NewFlex().SetDirection(tview.FlexRow)

	// Header
	header := createHeader()
	flex.AddItem(header, 3, 0, false)

	// Main content area
	contentFlex := tview.NewFlex().SetDirection(tview.FlexColumn)

	// Right side - Log display
	logFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	logFlex.SetBorder(true).SetTitle("Server Logs")

	logList := tview.NewList()
	logList.SetBorder(false)
	logList.SetMainTextColor(tcell.ColorWhite)
	logList.SetSecondaryTextColor(tcell.ColorGray)
	logList.SetSelectedTextColor(tcell.ColorBlack)
	logList.SetSelectedBackgroundColor(tcell.ColorYellow)
	logList.SetHighlightFullLine(true)
	logList.SetDoneFunc(func() {
		// ESC key handling
	})

	// Initial loading message
	logList.AddItem("Loading logs...", "", 0, nil)

	logFlex.AddItem(logList, 0, 1, false)

	contentFlex.AddItem(logFlex, 0, 1, false)

	// Left side - Controls
	controlsFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	controlsFlex.SetBorder(true).SetTitle("Log Filters")

	// Level selection checkboxes
	levelForm := tview.NewForm()
	levelForm.SetBorder(true).SetTitle("Log Levels")

	// Log levels to filter by
	levels := []string{"error", "warn", "info", "debug", "fatal"}
	var selectedLevelsMutex sync.Mutex                                    // Protect selectedLevels slice
	selectedLevels := []string{"error", "warn", "info", "debug", "fatal"} // All levels selected by default

	for _, level := range levels {
		// Create a local copy of level to avoid closure capture issues
		levelCopy := level
		levelForm.AddCheckbox(level, true, func(checked bool) {
			// Debug: log the checkbox change
			debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			fmt.Fprintf(debugFile, "[DEBUG] Checkbox %s changed to %v\n", levelCopy, checked)
			debugFile.Close()

			// This will trigger filtering update
			selectedLevelsMutex.Lock()
			if checked {
				// Add to selected levels if not already present
				found := false
				for _, l := range selectedLevels {
					if l == levelCopy {
						found = true
						break
					}
				}
				if !found {
					selectedLevels = append(selectedLevels, levelCopy)
					debugFile, _ = os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
					fmt.Fprintf(debugFile, "[DEBUG] Added %s to selectedLevels: %v\n", levelCopy, selectedLevels)
					debugFile.Close()
				}
			} else {
				// Remove from selected levels
				for i, l := range selectedLevels {
					if l == levelCopy {
						selectedLevels = append(selectedLevels[:i], selectedLevels[i+1:]...)
						debugFile, _ = os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
						fmt.Fprintf(debugFile, "[DEBUG] Removed %s from selectedLevels: %v\n", levelCopy, selectedLevels)
						debugFile.Close()
						break
					}
				}
			}
			selectedLevelsMutex.Unlock()
			// Update display in a goroutine to avoid UI thread issues
			go func() {
				if updateLogDisplay != nil {
					updateLogDisplay()
				}
			}()
		})
	}

	controlsFlex.AddItem(levelForm, 0, 1, false)

	// Search input
	searchFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	searchFlex.SetBorder(true).SetTitle("Search")

	// Timer for debouncing search input
	var searchTimer *time.Timer

	searchInput := tview.NewInputField().
		SetLabel("Filter: ").
		SetFieldWidth(30).
		SetChangedFunc(func(text string) {
			// Cancel previous timer if it exists
			if searchTimer != nil {
				searchTimer.Stop()
			}

			// Start a new timer that will trigger the update after 300ms of no changes
			searchTimer = time.AfterFunc(300*time.Millisecond, func() {
				if updateLogDisplay != nil {
					updateLogDisplay()
				}
			})
		}).
		SetDoneFunc(func(key tcell.Key) {
			// Cancel any pending timer and update immediately when user finishes editing
			if searchTimer != nil {
				searchTimer.Stop()
				searchTimer = nil
			}
			if updateLogDisplay != nil {
				updateLogDisplay()
			}
		})

	searchFlex.AddItem(searchInput, 3, 0, false)

	controlsFlex.AddItem(searchFlex, 0, 1, false)

	// Channel for log events
	stopChan := make(chan bool, 1)
	var streamingGoroutineRunning bool
	var allLogEntries []*rpc.FileLogEntry      // Store all log entries
	var filteredLogEntries []*rpc.FileLogEntry // Store filtered entries for display
	var logEntriesMutex sync.Mutex             // Protect logEntries from concurrent access
	const maxLogLines = 1000                   // Keep only last 1000 lines

	// Function to update the log display based on current filters
	updateLogDisplay = func() {
		logEntriesMutex.Lock()
		defer logEntriesMutex.Unlock()

		// Get current search text
		searchText := strings.ToLower(searchInput.GetText())

		// Debug: log total entries
		debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		fmt.Fprintf(debugFile, "[DEBUG] updateLogDisplay: allLogEntries has %d entries\n", len(allLogEntries))
		debugFile.Close()

		// Filter entries based on selected levels and search text
		filteredLogEntries = []*rpc.FileLogEntry{}

		// Debug: log selected levels
		selectedLevelsMutex.Lock()
		debugFile, _ = os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		fmt.Fprintf(debugFile, "[DEBUG] Selected levels: %v\n", selectedLevels)
		debugFile.Close()
		selectedLevelsMutex.Unlock()

		for _, entry := range allLogEntries {
			// Check level filter
			selectedLevelsMutex.Lock()
			levelSelected := false
			entryLevelLower := strings.ToLower(entry.Level)
			for _, selectedLevel := range selectedLevels {
				if strings.ToLower(selectedLevel) == entryLevelLower {
					levelSelected = true
					break
				}
			}
			selectedLevelsMutex.Unlock()

			// Debug: log level check
			debugFile, _ = os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			fmt.Fprintf(debugFile, "[DEBUG] Entry level '%s' (%s), selected: %v\n", entry.Level, entryLevelLower, levelSelected)
			debugFile.Close()

			if !levelSelected {
				continue
			}

			// Check search filter
			if searchText != "" {
				searchableText := strings.ToLower(fmt.Sprintf("%s %s %s %s", entry.Level, entry.Subsystem, entry.EventID, entry.Msg))
				if !strings.Contains(searchableText, searchText) {
					continue
				}
			}

			filteredLogEntries = append(filteredLogEntries, entry)
		}

		// Debug: log filtered count
		debugFile, _ = os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		fmt.Fprintf(debugFile, "[DEBUG] Filtered to %d entries\n", len(filteredLogEntries))
		debugFile.Close()

		// Update UI
		app.QueueUpdateDraw(func() {
			logList.Clear()
			addedCount := 0
			for _, logEntry := range filteredLogEntries {
				timestamp := parseLogTimestamp(logEntry.Timestamp)
				logTime := timestamp.Format("15:04:05")

				var levelColor string
				switch strings.ToLower(logEntry.Level) {
				case "fatal", "error":
					levelColor = "[red]"
				case "warning", "warn":
					levelColor = "[yellow]"
				case "info":
					levelColor = "[green]"
				case "debug":
					levelColor = "[blue]"
				default:
					levelColor = "[white]"
				}

				mainText := fmt.Sprintf("[%s] %s%s[-]: %s: %s", logTime, levelColor, logEntry.Level, logEntry.Subsystem, logEntry.Msg)
				logList.AddItem(mainText, "", 0, nil)
				addedCount++
			}

			// Debug: log how many items were added
			debugFile, _ = os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			fmt.Fprintf(debugFile, "[DEBUG] Added %d items to logList (filteredLogEntries has %d)\n", addedCount, len(filteredLogEntries))
			debugFile.Close()

			if len(filteredLogEntries) > 0 {
				logList.SetCurrentItem(len(filteredLogEntries) - 1) // Show latest log at bottom
			}
		})
	}

	// Auto-start streaming

	// Control buttons (vertical layout)
	buttonFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	buttonFlex.SetBorder(true).SetTitle("Controls")

	// Old buttons removed - now auto-starting

	backBtn := tview.NewButton("Back").SetSelectedFunc(func() {
		debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		fmt.Fprintf(debugFile, "[DEBUG] Back button pressed, streamingGoroutineRunning: %v\n", streamingGoroutineRunning)
		debugFile.Close()

		// Cancel any pending search timer
		if searchTimer != nil {
			searchTimer.Stop()
			searchTimer = nil
		}

		// Stop streaming if active
		if streamingGoroutineRunning {
			select {
			case stopChan <- true:
			default:
			}
			// No need to unsubscribe for file-based streaming
		}
		pages.RemovePage("remote_log_streaming")
		pages.SwitchToPage("remote_control_menu")
	})

	inspectBtn := tview.NewButton("Inspect Log").SetSelectedFunc(func() {
		// Get the currently selected item index
		selectedIndex := logList.GetCurrentItem()

		// Check if we have log entries and a valid selection
		logEntriesMutex.Lock()
		validSelection := len(filteredLogEntries) > 0 && selectedIndex >= 0 && selectedIndex < len(filteredLogEntries)
		logEntriesMutex.Unlock()

		if !validSelection {
			errorModal := tview.NewModal().
				SetText("No log entry selected or no logs available. Please select a log entry first.").
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					pages.RemovePage("no_selection_modal")
				})
			pages.AddPage("no_selection_modal", errorModal, true, true)
			return
		}

		// Get the log entry from filtered entries
		logEntriesMutex.Lock()
		entry := filteredLogEntries[selectedIndex]
		logEntriesMutex.Unlock()

		// Parse the raw JSON to get all fields including nested objects
		var entryData map[string]interface{}
		if err := json.Unmarshal([]byte(entry.RawJSON), &entryData); err != nil {
			errorModal := tview.NewModal().
				SetText("Error parsing log entry JSON: " + err.Error()).
				AddButtons([]string{"OK"}).
				SetDoneFunc(func(buttonIndex int, buttonLabel string) {
					pages.RemovePage("json_error_modal")
				})
			pages.AddPage("json_error_modal", errorModal, true, true)
			return
		}

		// Format as tree structure
		formattedText := formatJSONTree(entryData, "")

		// Create a text view to display the formatted JSON
		jsonView := tview.NewTextView()
		jsonView.SetBorder(true).SetTitle(fmt.Sprintf("Log Entry %d", selectedIndex+1))
		jsonView.SetDynamicColors(true) // Enable color tags for syntax highlighting
		jsonView.SetWordWrap(true)
		jsonView.SetScrollable(true)
		jsonView.SetText(formattedText)

		// Create modal with the JSON view
		jsonFlex := tview.NewFlex().SetDirection(tview.FlexRow)
		jsonFlex.AddItem(jsonView, 0, 1, true)
		closeBtn := tview.NewButton("Close").SetSelectedFunc(func() {
			pages.RemovePage("json_inspect_modal")
		})
		jsonFlex.AddItem(closeBtn, 3, 0, false)

		pages.AddPage("json_inspect_modal", jsonFlex, true, true)
	})

	// Button flex
	actionsFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	actionsFlex.SetBorder(true).SetTitle("Actions")
	actionsFlex.AddItem(inspectBtn, 3, 0, false)
	actionsFlex.AddItem(backBtn, 3, 0, false)

	controlsFlex.AddItem(actionsFlex, 0, 1, false)

	contentFlex.AddItem(controlsFlex, 30, 0, false)

	flex.AddItem(contentFlex, 0, 1, false)

	// Auto-start streaming
	go func() {
		debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		fmt.Fprintf(debugFile, "[DEBUG] Auto-starting file-based log streaming\n")
		debugFile.Close()

		streamingGoroutineRunning = true
		allLogEntries = []*rpc.FileLogEntry{} // Start with empty list

		// Start tailing the log file with all sources
		logChan, err := client.TailLogFile(buildDir, []string{"*"})
		if err != nil {
			app.QueueUpdateDraw(func() {
				errorModal := tview.NewModal().
					SetText(fmt.Sprintf("Failed to start log tailing: %v", err)).
					AddButtons([]string{"OK"}).
					SetDoneFunc(func(int, string) {
						pages.RemovePage("log_tailing_error_modal")
					})
				pages.AddPage("log_tailing_error_modal", errorModal, true, true)
			})
			return
		}

		var historicTimer *time.Timer

		go func() {
			defer func() {
				if historicTimer != nil {
					historicTimer.Stop()
				}
				streamingGoroutineRunning = false
				debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
				fmt.Fprintf(debugFile, "[DEBUG] File-based log streaming goroutine exited\n")
				debugFile.Close()
			}()

			var pendingEntries []*rpc.FileLogEntry
			lastUpdate := time.Now()
			updateInterval := 200 * time.Millisecond // Update UI every 200ms max
			historicLogsDone := false
			historicLoadTimeout := 500 * time.Millisecond // Wait 500ms after last historic log

			debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			fmt.Fprintf(debugFile, "[DEBUG] File-based streaming loop started\n")
			debugFile.Close()

			for {
				select {
				case entry, ok := <-logChan:
					if !ok {
						// Channel closed, exit
						debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
						fmt.Fprintf(debugFile, "[DEBUG] Log channel closed, exiting\n")
						debugFile.Close()
						return
					}

					pendingEntries = append(pendingEntries, entry)

					// Reset the historic timer whenever we receive an entry
					if historicTimer != nil {
						historicTimer.Stop()
					}
					historicTimer = time.AfterFunc(historicLoadTimeout, func() {
						if !historicLogsDone {
							historicLogsDone = true

							// Render all historic logs at once
							logEntriesMutex.Lock()
							allLogEntries = append(allLogEntries, pendingEntries...)
							pendingEntries = pendingEntries[:0] // Clear pending

							// Sort by timestamp (oldest first)
							sortLogEntries(allLogEntries)

							// Trim to max lines to prevent memory issues
							if len(allLogEntries) > maxLogLines {
								allLogEntries = allLogEntries[len(allLogEntries)-maxLogLines:]
							}
							logEntriesMutex.Unlock()

							// Update display with filtered logs
							updateLogDisplay()

							debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
							fmt.Fprintf(debugFile, "[DEBUG] Rendered %d historic logs at once\n", len(allLogEntries))
							debugFile.Close()
						}
					})

					// For real-time logs (after historic logs are done), update UI if enough time has passed or batch is getting large
					if historicLogsDone && (time.Since(lastUpdate) >= updateInterval || len(pendingEntries) >= 10) {
						logEntriesMutex.Lock()
						allLogEntries = append(allLogEntries, pendingEntries...)
						pendingEntries = pendingEntries[:0] // Clear pending

						// Sort by timestamp (oldest first)
						sortLogEntries(allLogEntries)

						// Trim to max lines to prevent memory issues
						if len(allLogEntries) > maxLogLines {
							allLogEntries = allLogEntries[len(allLogEntries)-maxLogLines:]
						}
						logEntriesMutex.Unlock()

						// Update display
						updateLogDisplay()
						lastUpdate = time.Now()
					}

				case <-stopChan:
					debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
					fmt.Fprintf(debugFile, "[DEBUG] Received stop signal, exiting\n")
					debugFile.Close()
					return
				}
			}
		}()
	}()

	pages.AddPage("remote_log_streaming", flex, true, true)
}
