package rpc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ObsidianIRC/unrealircd-rpc-golang"
)

type RPCClient struct {
	conn *unrealircd.Connection
}

func NewRPCClient(config *RPCConfig) (*RPCClient, error) {
	apiLogin := config.Username + ":" + config.Password
	conn, err := unrealircd.NewConnection(config.WSURL, apiLogin, &unrealircd.Options{
		TLSVerify: false, // For development, you might want to set this to true in production
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create RPC connection: %w", err)
	}

	return &RPCClient{
		conn: conn,
	}, nil
}

func (r *RPCClient) Connect() error {
	// Connection is established in NewConnection, but we might want to test it
	// For now, just return nil as the connection should be ready
	return nil
}

func (r *RPCClient) Close() error {
	// The library doesn't seem to have a Close method, so we'll just return nil
	return nil
}

func (r *RPCClient) GetUsers() ([]UserInfo, error) {
	debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	defer debugFile.Close()

	// Try detail level 0 to see if it returns strings
	usersData, err := r.conn.User().GetAll(0) // Detail level 0
	if err != nil {
		fmt.Fprintf(debugFile, "DEBUG RPC: GetAll failed: %v\n", err)
		return nil, fmt.Errorf("failed to get users: %w", err)
	}

	fmt.Fprintf(debugFile, "DEBUG RPC: GetAll returned type %T\n", usersData)

	var users []UserInfo

	// Check if it's a list of interfaces
	if userList, ok := usersData.([]interface{}); ok {
		fmt.Fprintf(debugFile, "DEBUG RPC: userList has %d items\n", len(userList))
		if len(userList) == 0 {
			fmt.Fprintf(debugFile, "DEBUG RPC: empty user list\n")
			return users, nil
		}

		for i, u := range userList {
			fmt.Fprintf(debugFile, "DEBUG RPC: item %d type: %T\n", i, u)
			if userMap, ok := u.(map[string]interface{}); ok {
				fmt.Fprintf(debugFile, "DEBUG RPC: parsing userMap with keys: %v\n", getKeys(userMap))
				// GetAll returned full user objects, but channels are not included, so get full details
				if name, ok := userMap["name"].(string); ok {
					fmt.Fprintf(debugFile, "DEBUG RPC: found nick: %s, about to call GetUserDetails\n", name)
					userDetails, err := r.GetUserDetails(name)
					fmt.Fprintf(debugFile, "DEBUG RPC: GetUserDetails returned, err=%v, userDetails=%v\n", err, userDetails)
					if err != nil {
						fmt.Fprintf(debugFile, "DEBUG RPC: error getting details for %s: %v\n", name, err)
						continue
					}
					if userDetails != nil && userDetails.Nick != "" {
						fmt.Fprintf(debugFile, "DEBUG RPC: adding user from details: %s with %d channels\n", userDetails.Nick, len(userDetails.Channels))
						users = append(users, *userDetails)
					}
				}
			} else if nickStr, ok := u.(string); ok {
				fmt.Fprintf(debugFile, "DEBUG RPC: got nickname string: %s\n", nickStr)
				// GetAll returned just nicknames, get full details for each
				userDetails, err := r.GetUserDetails(nickStr)
				if err != nil {
					fmt.Fprintf(debugFile, "DEBUG RPC: error getting details for %s: %v\n", nickStr, err)
					continue
				}
				if userDetails != nil && (userDetails.Nick != "" || userDetails.Name != "") {
					fmt.Fprintf(debugFile, "DEBUG RPC: adding user from details: %s\n", userDetails.Nick)
					users = append(users, *userDetails)
				}
			}
		}
	} else {
		fmt.Fprintf(debugFile, "DEBUG RPC: usersData is not []interface{}, it's %T\n", usersData)
		return nil, fmt.Errorf("unexpected response format: %T", usersData)
	}

	fmt.Fprintf(debugFile, "DEBUG RPC: returning %d users\n", len(users))
	return users, nil
}

func (r *RPCClient) GetChannels() ([]ChannelInfo, error) {
	debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	defer debugFile.Close()

	// Try detail level 1 to get basic channel info for the list
	channelsData, err := r.conn.Channel().GetAll(1)
	if err != nil {
		fmt.Fprintf(debugFile, "DEBUG RPC: Channel().GetAll failed: %v\n", err)
		return nil, fmt.Errorf("failed to get channels: %w", err)
	}

	fmt.Fprintf(debugFile, "DEBUG RPC: Channel().GetAll returned type %T\n", channelsData)

	var channels []ChannelInfo

	// Check if it's a list of interfaces
	if channelList, ok := channelsData.([]interface{}); ok {
		fmt.Fprintf(debugFile, "DEBUG RPC: channelList has %d items\n", len(channelList))
		if len(channelList) == 0 {
			fmt.Fprintf(debugFile, "DEBUG RPC: empty channel list\n")
			return channels, nil
		}

		for i, ch := range channelList {
			fmt.Fprintf(debugFile, "DEBUG RPC: item %d type: %T\n", i, ch)
			if channelMap, ok := ch.(map[string]interface{}); ok {
				fmt.Fprintf(debugFile, "DEBUG RPC: parsing channelMap with keys: %v\n", getKeys(channelMap))

				channel := ChannelInfo{}

				if name, ok := channelMap["name"].(string); ok {
					channel.Name = name
				}
				if topic, ok := channelMap["topic"].(string); ok {
					channel.Topic = topic
				}
				if numUsers, ok := channelMap["num_users"].(float64); ok {
					channel.UserCount = int(numUsers)
				}
				if modes, ok := channelMap["modes"].(string); ok {
					channel.Modes = modes
				}
				if created, ok := channelMap["created"].(float64); ok {
					channel.Created = int64(created)
				}
				// Extract users list - try different possible keys
				usersFound := false
				for _, key := range []string{"users", "members", "occupants", "nicks"} {
					if usersData, ok := channelMap[key]; ok {
						fmt.Fprintf(debugFile, "DEBUG RPC: found users under key '%s', type: %T, value: %v\n", key, usersData, usersData)
						if usersArray, ok := usersData.([]interface{}); ok {
							fmt.Fprintf(debugFile, "DEBUG RPC: usersArray has %d items\n", len(usersArray))
							for i, user := range usersArray {
								fmt.Fprintf(debugFile, "DEBUG RPC: user %d type: %T, value: %v\n", i, user, user)
								if userStr, ok := user.(string); ok {
									channel.Users = append(channel.Users, userStr)
									fmt.Fprintf(debugFile, "DEBUG RPC: added user: %s\n", userStr)
								} else if userMap, ok := user.(map[string]interface{}); ok {
									// Maybe users are objects with nick field
									if nick, ok := userMap["nick"].(string); ok {
										channel.Users = append(channel.Users, nick)
										fmt.Fprintf(debugFile, "DEBUG RPC: added user from object: %s\n", nick)
									} else if name, ok := userMap["name"].(string); ok {
										channel.Users = append(channel.Users, name)
										fmt.Fprintf(debugFile, "DEBUG RPC: added user from object name: %s\n", name)
									}
								}
							}
							usersFound = true
							break
						} else {
							fmt.Fprintf(debugFile, "DEBUG RPC: usersData under key '%s' is not []interface{}, it's %T\n", key, usersData)
						}
					}
				}
				if !usersFound {
					fmt.Fprintf(debugFile, "DEBUG RPC: no users key found in channelMap\n")
				}

				channels = append(channels, channel)
			} else if channelName, ok := ch.(string); ok {
				fmt.Fprintf(debugFile, "DEBUG RPC: got channel name string: %s\n", channelName)
				// If it's just a string, create a basic ChannelInfo
				channel := ChannelInfo{Name: channelName}
				channels = append(channels, channel)
			}
		}
	} else {
		fmt.Fprintf(debugFile, "DEBUG RPC: channelsData is not []interface{}, it's %T\n", channelsData)
		return nil, fmt.Errorf("unexpected response format: %T", channelsData)
	}

	fmt.Fprintf(debugFile, "DEBUG RPC: returning %d channels\n", len(channels))
	return channels, nil
}

func (r *RPCClient) GetChannelDetails(channelName string) (*ChannelInfo, error) {
	debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	defer debugFile.Close()

	fmt.Fprintf(debugFile, "DEBUG: GetChannelDetails called for channel: %s\n", channelName)

	channelData, err := r.conn.Channel().Get(channelName, 4)
	if err != nil {
		fmt.Fprintf(debugFile, "DEBUG: Channel().Get failed for %s: %v\n", channelName, err)
		return nil, fmt.Errorf("failed to get channel details for %s: %w", channelName, err)
	}

	fmt.Fprintf(debugFile, "DEBUG: Channel().Get returned type %T\n", channelData)

	if channelMap, ok := channelData.(map[string]interface{}); ok {
		fmt.Fprintf(debugFile, "DEBUG: parsing channelMap with keys: %v\n", getKeys(channelMap))

		// The actual channel data might be under "channel" key
		if channelData, ok := channelMap["channel"].(map[string]interface{}); ok {
			fmt.Fprintf(debugFile, "DEBUG: parsing channel data with keys: %v\n", getKeys(channelData))
			channelMap = channelData
		}

		channel := &ChannelInfo{}

		if name, ok := channelMap["name"].(string); ok {
			channel.Name = name
		}
		if topic, ok := channelMap["topic"].(string); ok {
			channel.Topic = topic
		}
		if numUsers, ok := channelMap["num_users"].(float64); ok {
			channel.UserCount = int(numUsers)
		}
		if modes, ok := channelMap["modes"].(string); ok {
			channel.Modes = modes
		}
		if created, ok := channelMap["created"].(float64); ok {
			channel.Created = int64(created)
		}

		// Extract users list with detailed info - try different possible keys
		usersFound := false
		for _, key := range []string{"users", "members", "occupants", "nicks", "userlist"} {
			if usersData, ok := channelMap[key]; ok {
				fmt.Fprintf(debugFile, "DEBUG RPC: found users under key '%s', type: %T, value: %v\n", key, usersData, usersData)
				if usersArray, ok := usersData.([]interface{}); ok {
					fmt.Fprintf(debugFile, "DEBUG RPC: usersArray has %d items\n", len(usersArray))
					for i, user := range usersArray {
						fmt.Fprintf(debugFile, "DEBUG RPC: user %d type: %T, value: %v\n", i, user, user)
						if userMap, ok := user.(map[string]interface{}); ok {
							// Parse detailed user info
							userInfo := ""

							// Get user level/prefix
							prefix := ""
							if level, ok := userMap["level"].(string); ok {
								if strings.Contains(level, "q") {
									prefix = "~"
								} else if strings.Contains(level, "a") {
									prefix = "&"
								} else if strings.Contains(level, "o") {
									prefix = "@"
								} else if strings.Contains(level, "h") {
									prefix = "%"
								} else if strings.Contains(level, "v") {
									prefix = "+"
								}
							}

							// Get nickname
							nick := ""
							if nickVal, ok := userMap["nick"].(string); ok {
								nick = nickVal
							} else if nameVal, ok := userMap["name"].(string); ok {
								nick = nameVal
							}

							// Get username and host
							userHost := ""
							if username, ok := userMap["username"].(string); ok && username != "" {
								if hostname, ok := userMap["hostname"].(string); ok && hostname != "" {
									userHost = fmt.Sprintf(" (%s@%s)", username, hostname)
								} else if ip, ok := userMap["ip"].(string); ok && ip != "" {
									userHost = fmt.Sprintf(" (%s@%s)", username, ip)
								}
							}

							// Get channel count
							channelCount := ""
							if channels, ok := userMap["channels"].(float64); ok {
								channelCount = fmt.Sprintf(" [%d channel(s)]", int(channels))
							}

							userInfo = fmt.Sprintf("%s%s%s%s", prefix, nick, userHost, channelCount)
							if userInfo != "" {
								channel.Users = append(channel.Users, userInfo)
								fmt.Fprintf(debugFile, "DEBUG RPC: added detailed user: %s\n", userInfo)
							}
						} else if userStr, ok := user.(string); ok {
							// Fallback for simple string
							channel.Users = append(channel.Users, userStr)
							fmt.Fprintf(debugFile, "DEBUG RPC: added simple user: %s\n", userStr)
						}
					}
					usersFound = true
					break
				} else {
					fmt.Fprintf(debugFile, "DEBUG RPC: usersData under key '%s' is not []interface{}, it's %T\n", key, usersData)
				}
			}
		}
		if !usersFound {
			fmt.Fprintf(debugFile, "DEBUG RPC: no users key found in channelMap\n")
		}

		return channel, nil
	}

	return nil, fmt.Errorf("unexpected response format: %T", channelData)
}

func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func (r *RPCClient) GetUserDetails(nick string) (*UserInfo, error) {
	debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	defer debugFile.Close()

	fmt.Fprintf(debugFile, "DEBUG: GetUserDetails called for nick: %s\n", nick)

	userData, err := r.conn.User().Get(nick, 4)
	if err != nil {
		fmt.Fprintf(debugFile, "DEBUG: User().Get failed for %s: %v\n", nick, err)
		return nil, fmt.Errorf("failed to get user details for %s: %w", nick, err)
	}

	fmt.Fprintf(debugFile, "DEBUG: User().Get returned type %T\n", userData)

	if userMap, ok := userData.(map[string]interface{}); ok {
		fmt.Fprintf(debugFile, "DEBUG: parsing userMap with keys: %v\n", getKeys(userMap))

		user := &UserInfo{}

		// The actual user data is under "user" key
		if userData, ok := userMap["user"].(map[string]interface{}); ok {
			fmt.Fprintf(debugFile, "DEBUG: parsing user data with keys: %v\n", getKeys(userData))

			if name, ok := userMap["name"].(string); ok { // name is at top level
				user.Name = name
				user.Nick = name
			}
			if realname, ok := userData["realname"].(string); ok {
				user.Realname = realname
			}
			if account, ok := userData["account"].(string); ok {
				user.Account = account
			}
			if ip, ok := userMap["ip"].(string); ok { // IP is at top level
				user.IP = ip
			}
			if username, ok := userData["username"].(string); ok {
				user.Username = username
			}
			if vhost, ok := userData["vhost"].(string); ok {
				user.Vhost = vhost
			}
			if cloakedhost, ok := userData["cloakedhost"].(string); ok {
				user.Cloakedhost = cloakedhost
			}
			if servername, ok := userData["servername"].(string); ok {
				user.Servername = servername
			}
			if reputation, ok := userData["reputation"].(float64); ok {
				user.Reputation = int(reputation)
			}
			if modes, ok := userData["modes"].(string); ok {
				user.Modes = modes
			}
			// Extract channels
			if channelsData, ok := userData["channels"].([]interface{}); ok {
				for _, ch := range channelsData {
					if chMap, ok := ch.(map[string]interface{}); ok {
						if chName, ok := chMap["name"].(string); ok {
							user.Channels = append(user.Channels, chName)
						}
					}
				}
			}
			// Extract security-groups
			if sgData, ok := userData["security-groups"].([]interface{}); ok {
				for _, sg := range sgData {
					if sgStr, ok := sg.(string); ok {
						user.SecurityGroups = append(user.SecurityGroups, sgStr)
					}
				}
			}
		} else {
			// Fallback to top level if no "user" key
			if name, ok := userMap["name"].(string); ok {
				user.Name = name
				user.Nick = name
			}
			if realname, ok := userMap["realname"].(string); ok {
				user.Realname = realname
			}
			if account, ok := userMap["account"].(string); ok {
				user.Account = account
			}
			if ip, ok := userMap["ip"].(string); ok {
				user.IP = ip
			}

			// Extract channels
			if channelsData, ok := userMap["channels"].([]interface{}); ok {
				for _, ch := range channelsData {
					if chStr, ok := ch.(string); ok {
						user.Channels = append(user.Channels, chStr)
					}
				}
			}
		}

		fmt.Fprintf(debugFile, "DEBUG: parsed user: %+v\n", user)
		return user, nil
	} else {
		fmt.Fprintf(debugFile, "DEBUG: userData is not map[string]interface{}, it's %T\n", userData)
		return nil, fmt.Errorf("unexpected user details format: %T", userData)
	}
}

// SubscribeToLogs subscribes to log events from specified sources
func (r *RPCClient) SubscribeToLogs(sources []string) error {
	_, err := r.conn.Log().Subscribe(sources)
	return err
}

// UnsubscribeFromLogs unsubscribes from log events
func (r *RPCClient) UnsubscribeFromLogs() error {
	_, err := r.conn.Log().Unsubscribe()
	return err
}

// GetLogEvent waits for and returns the next log event
func (r *RPCClient) GetLogEvent() (*LogEntry, error) {
	// Try EventLoop with a timeout to avoid blocking forever
	type result struct {
		event interface{}
		err   error
	}

	resultChan := make(chan result, 1)

	go func() {
		event, err := r.conn.EventLoop()
		resultChan <- result{event: event, err: err}
	}()

	// Wait for event or timeout after 5 seconds
	select {
	case res := <-resultChan:
		if res.err != nil {
			// Check if this is a "could not parse as log event" error for nil events
			if strings.Contains(res.err.Error(), "could not parse as log event") && strings.Contains(res.err.Error(), "<nil>") {
				// This is a nil event, ignore it and continue polling
				debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
				fmt.Fprintf(debugFile, "DEBUG: Ignoring nil event, continuing to poll\n")
				debugFile.Close()
				return nil, nil
			}
			return nil, res.err
		}

		// Handle nil events gracefully
		if res.event == nil {
			debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			fmt.Fprintf(debugFile, "DEBUG: Received nil event, continuing to poll\n")
			debugFile.Close()
			return nil, nil
		}

		// Log raw event data for debugging
		debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		fmt.Fprintf(debugFile, "DEBUG: Raw event received: %+v\n", res.event)
		debugFile.Close()

		// Parse the event data
		if eventMap, ok := res.event.(map[string]interface{}); ok {
			entry := &LogEntry{}

			if timeVal, ok := eventMap["time"].(float64); ok {
				entry.Time = int64(timeVal)
			}
			if level, ok := eventMap["level"].(string); ok {
				entry.Level = level
			}
			if source, ok := eventMap["source"].(string); ok {
				entry.Source = source
			}
			if message, ok := eventMap["message"].(string); ok {
				entry.Message = message
			}

			if entry.Time != 0 || entry.Level != "" || entry.Source != "" || entry.Message != "" {
				return entry, nil
			}
		}

		return nil, nil // No valid log event

	case <-time.After(5 * time.Second):
		// Timeout - no events available
		return nil, nil
	}
}

// TailLogFile starts tailing the JSON log file and returns a channel of parsed log entries
func (r *RPCClient) TailLogFile(buildDir string, sources []string) (<-chan *FileLogEntry, error) {
	logFilePath := filepath.Join(buildDir, "logs", "ircd.json.log")

	// Check if log file exists
	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("log file does not exist: %s", logFilePath)
	}

	logChan := make(chan *FileLogEntry, 100)

	go func() {
		defer close(logChan)

		// First, read existing logs from the file and filter by sources
		if len(sources) > 0 {
			file, err := os.Open(logFilePath)
			if err != nil {
				debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
				fmt.Fprintf(debugFile, "[DEBUG] Failed to open log file for historic reading: %v\n", err)
				debugFile.Close()
			} else {
				defer file.Close()

				scanner := bufio.NewScanner(file)
				var historicLogs []*FileLogEntry

				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())
					if line == "" {
						continue
					}

					// First parse into a map to get all fields
					var rawData map[string]interface{}
					if err := json.Unmarshal([]byte(line), &rawData); err != nil {
						continue // Skip invalid lines
					}

					// Then parse into FileLogEntry struct
					var entry FileLogEntry
					if err := json.Unmarshal([]byte(line), &entry); err != nil {
						continue // Skip invalid lines
					}

					// Store the raw JSON
					entry.RawJSON = line

					// Filter by selected sources
					sourceMatch := false
					for _, source := range sources {
						if source == "*" || entry.Subsystem == source {
							sourceMatch = true
							break
						}
					}

					if sourceMatch {
						historicLogs = append(historicLogs, &entry)
					}
				}

				if err := scanner.Err(); err != nil {
					debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
					fmt.Fprintf(debugFile, "[DEBUG] Scanner error reading historic logs: %v\n", err)
					debugFile.Close()
				}

				// Reverse historicLogs so oldest logs come first
				for i, j := 0, len(historicLogs)-1; i < j; i, j = i+1, j-1 {
					historicLogs[i], historicLogs[j] = historicLogs[j], historicLogs[i]
				}

				// Send all historic logs at once (now in chronological order: oldest first)
				for _, entry := range historicLogs {
					select {
					case logChan <- entry:
					default:
						// Channel is full, skip this entry
					}
				}

				debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
				fmt.Fprintf(debugFile, "[DEBUG] Finished reading %d historic logs\n", len(historicLogs))
				debugFile.Close()
			}
		}

		// Now start tail -f for new logs
		cmd := exec.Command("tail", "-f", "-n", "0", logFilePath)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			fmt.Fprintf(debugFile, "[DEBUG] Failed to create stdout pipe: %v\n", err)
			debugFile.Close()
			return
		}

		if err := cmd.Start(); err != nil {
			debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			fmt.Fprintf(debugFile, "[DEBUG] Failed to start tail command: %v\n", err)
			debugFile.Close()
			return
		}

		debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		fmt.Fprintf(debugFile, "[DEBUG] Started tailing log file: %s\n", logFilePath)
		debugFile.Close()

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			var entry FileLogEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
				fmt.Fprintf(debugFile, "[DEBUG] Failed to parse JSON log entry: %v\n", err)
				debugFile.Close()
				continue
			}

			// Store the raw JSON
			entry.RawJSON = line

			// Filter by selected sources (for new logs too)
			if len(sources) > 0 && sources[0] != "*" {
				sourceMatch := false
				for _, source := range sources {
					if entry.Subsystem == source {
						sourceMatch = true
						break
					}
				}
				if !sourceMatch {
					continue
				}
			}

			select {
			case logChan <- &entry:
			default:
				// Channel is full, skip this entry
				debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
				fmt.Fprintf(debugFile, "[DEBUG] Log channel full, skipping entry\n")
				debugFile.Close()
			}
		}

		if err := scanner.Err(); err != nil {
			debugFile, _ := os.OpenFile("/tmp/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			fmt.Fprintf(debugFile, "[DEBUG] Scanner error: %v\n", err)
			debugFile.Close()
		}

		// Wait for command to finish
		cmd.Wait()
	}()

	return logChan, nil
}
