package kernel

// SelfLoginResult captures the authoritative evidence chain for login.
type SelfLoginResult struct {
	CheckcodeFetched  bool   `json:"checkcodeFetched"`
	RandomCodeCalled  bool   `json:"randomCodeCalled"`
	VerifyStatus      int    `json:"verifyStatus"`
	VerifyLocation    string `json:"verifyLocation,omitempty"`
	DashboardReadable bool   `json:"dashboardReadable"`
	SessionAlive      bool   `json:"sessionAlive"`
}

// SelfStatus describes current authenticated readability state.
type SelfStatus struct {
	LoggedIn          bool   `json:"loggedIn"`
	DashboardReadable bool   `json:"dashboardReadable"`
	ServiceReadable   bool   `json:"serviceReadable"`
	Reason            string `json:"reason,omitempty"`
}

// OnlineSession represents one online device/session record.
type OnlineSession struct {
	BRASID       string `json:"brasid,omitempty"`
	IP           string `json:"ip,omitempty"`
	LoginTime    string `json:"loginTime,omitempty"`
	MAC          string `json:"mac,omitempty"`
	SessionID    string `json:"sessionId,omitempty"`
	TerminalType string `json:"terminalType,omitempty"`
	UpFlow       string `json:"upFlow,omitempty"`
	DownFlow     string `json:"downFlow,omitempty"`
	UseTime      string `json:"useTime,omitempty"`
	UserID       string `json:"userId,omitempty"`
}

// LoginHistoryEntry preserves the raw row for unstable columns while naming stable ones.
type LoginHistoryEntry struct {
	LoginTime    string        `json:"loginTime,omitempty"`
	LogoutTime   string        `json:"logoutTime,omitempty"`
	IP           string        `json:"ip,omitempty"`
	MAC          string        `json:"mac,omitempty"`
	TerminalFlag string        `json:"terminalFlag,omitempty"`
	TerminalType string        `json:"terminalType,omitempty"`
	Raw          []interface{} `json:"raw,omitempty"`
}

// MauthState reflects refreshMauthType semantics.
type MauthState string

const (
	MauthUnknown MauthState = "unknown"
	MauthOn      MauthState = "on"
	MauthOff     MauthState = "off"
)

// OperatorBinding models the four known binding fields.
type OperatorBinding struct {
	TelecomAccount  string `json:"telecomAccount,omitempty"`
	TelecomPassword string `json:"telecomPassword,omitempty"`
	MobileAccount   string `json:"mobileAccount,omitempty"`
	MobilePassword  string `json:"mobilePassword,omitempty"`
}

// ConsumeProtectState keeps the confirmed business truth fields for consume protect.
type ConsumeProtectState struct {
	CSRFTOKEN       string `json:"csrftoken,omitempty"`
	InstallmentFlag string `json:"installmentFlag,omitempty"`
	CurrentLimit    string `json:"currentLimit,omitempty"`
	CurrentUsage    string `json:"currentUsage,omitempty"`
	Balance         string `json:"balance,omitempty"`
}

// MacListResult keeps the raw page rows because field shapes may vary by environment.
type MacListResult struct {
	Total int                      `json:"total,omitempty"`
	Rows  []map[string]interface{} `json:"rows,omitempty"`
}

// PersonState exposes guarded person-list diagnostics.
type PersonState struct {
	CSRFTOKEN string            `json:"csrftoken,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
	RawHTML   string            `json:"rawHtml,omitempty"`
}

// BillListResult is shared by bill JSON list endpoints.
type BillListResult struct {
	Summary map[string]interface{}   `json:"summary,omitempty"`
	Total   int                      `json:"total,omitempty"`
	Rows    []map[string]interface{} `json:"rows,omitempty"`
}

// Portal802Response captures the normalized 802 JSONP payload.
type Portal802Response struct {
	Result     string `json:"result,omitempty"`
	RetCode    string `json:"retCode,omitempty"`
	Msg        string `json:"msg,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
	RawPayload string `json:"rawPayload,omitempty"`
}
