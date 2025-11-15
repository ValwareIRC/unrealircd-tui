package rpc

// Server info
type ServerInfo struct {
	Name     string `json:"name"`
	Uptime   int64  `json:"uptime"`
	Software string `json:"software"`
	Users    int    `json:"users"`
}

// User info
type UserInfo struct {
	Name           string   `json:"name"`
	Nick           string   `json:"nick"`
	Realname       string   `json:"realname"`
	Account        string   `json:"account"`
	IP             string   `json:"ip"`
	Channels       []string `json:"channels"`
	Username       string   `json:"username"`
	Vhost          string   `json:"vhost"`
	Cloakedhost    string   `json:"cloakedhost"`
	Servername     string   `json:"servername"`
	Reputation     int      `json:"reputation"`
	Modes          string   `json:"modes"`
	SecurityGroups []string `json:"security-groups"`
}

// Channel info
type ChannelInfo struct {
	Name      string   `json:"name"`
	Topic     string   `json:"topic"`
	Users     []string `json:"users"`
	UserCount int      `json:"num_users"`
	Modes     string   `json:"modes"`
	Created   int64    `json:"created"`
}

// Server ban info
type ServerBanInfo struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Duration  int64  `json:"duration"`
	Setby     string `json:"setby"`
	CreatedAt int64  `json:"created_at"`
}

// Log entry from server logs
type LogEntry struct {
	Time    int64  `json:"time"`
	Level   string `json:"level"`
	Source  string `json:"source"`
	Message string `json:"message"`
}

// File-based log entry from JSON log file
type FileLogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Subsystem string                 `json:"subsystem"`
	EventID   string                 `json:"event_id"`
	LogSource string                 `json:"log_source"`
	Msg       string                 `json:"msg"`
	Client    map[string]interface{} `json:"client,omitempty"`
	Channel   map[string]interface{} `json:"channel,omitempty"`
	User      map[string]interface{} `json:"user,omitempty"`
	TLS       map[string]interface{} `json:"tls,omitempty"`
	RawJSON   string                 `json:"-"` // Store the full raw JSON line
}
