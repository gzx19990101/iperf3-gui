package model

type Protocol string

const (
	ProtocolTCP Protocol = "tcp"
	ProtocolUDP Protocol = "udp"
)

type Direction string

const (
	DirectionUpload        Direction = "upload"
	DirectionDownload      Direction = "download"
	DirectionBidirectional Direction = "bidirectional"
)

type RunStatus string

const (
	RunStatusCreated     RunStatus = "created"
	RunStatusStarting    RunStatus = "starting"
	RunStatusRunning     RunStatus = "running"
	RunStatusStopping    RunStatus = "stopping"
	RunStatusStopped     RunStatus = "stopped"
	RunStatusCompleted   RunStatus = "completed"
	RunStatusFailed      RunStatus = "failed"
	RunStatusInterrupted RunStatus = "interrupted"
)

const (
	ErrIPerfNotFound         = "IPERF_NOT_FOUND"
	ErrIPerfProbeFailed      = "IPERF_PROBE_FAILED"
	ErrVersionUnsupported    = "IPERF_VERSION_UNSUPPORTED"
	ErrInvalidHost           = "CONFIG_INVALID_HOST"
	ErrInvalidPort           = "CONFIG_INVALID_PORT"
	ErrInvalidValue          = "CONFIG_INVALID_VALUE"
	ErrInvalidBind           = "CONFIG_INVALID_BIND"
	ErrConfigConflict        = "CONFIG_CONFLICT"
	ErrUnsupportedDirection  = "CONFIG_UNSUPPORTED_DIRECTION"
	ErrRunAlreadyActive      = "RUN_ALREADY_ACTIVE"
	ErrRunNotFound           = "RUN_NOT_FOUND"
	ErrStateConflict         = "RUN_STATE_CONFLICT"
	ErrProcessSpawnFailed    = "PROCESS_SPAWN_FAILED"
	ErrProcessPortInUse      = "PROCESS_PORT_IN_USE"
	ErrProcessDNSFailed      = "PROCESS_DNS_FAILED"
	ErrProcessConnectTimeout = "PROCESS_CONNECT_TIMEOUT"
	ErrProcessConnectFailed  = "PROCESS_CONNECT_FAILED"
	ErrProcessStoppedByUser  = "PROCESS_STOPPED_BY_USER"
	ErrProcessUnexpectedExit = "PROCESS_UNEXPECTED_EXIT"
	ErrParseUnsupported      = "PARSE_UNSUPPORTED"
	ErrParseFailed           = "PARSE_FAILED"
	ErrDBOpenFailed          = "DB_OPEN_FAILED"
	ErrDBMigrationFailed     = "DB_MIGRATION_FAILED"
	ErrDBWriteFailed         = "DB_WRITE_FAILED"
	ErrDBDeleteFailed        = "DB_DELETE_FAILED"
	ErrHistoryNotFound       = "HISTORY_NOT_FOUND"
	ErrExportWriteFailed     = "EXPORT_WRITE_FAILED"
	ErrFileDialogFailed      = "FILE_DIALOG_FAILED"
	ErrWebView2Missing       = "WEBVIEW2_MISSING"
)

type ClientConfig struct {
	Host                      string    `json:"host"`
	Port                      int       `json:"port"`
	Protocol                  Protocol  `json:"protocol"`
	Direction                 Direction `json:"direction"`
	DurationSeconds           int       `json:"durationSeconds"`
	IntervalSeconds           int       `json:"intervalSeconds"`
	ParallelStreams           int       `json:"parallelStreams"`
	BindAddress               string    `json:"bindAddress,omitempty"`
	IPVersion                 int       `json:"ipVersion,omitempty"`
	UDPBandwidthBitsPerSecond int64     `json:"udpBandwidthBitsPerSecond,omitempty"`
	UDPDatagramLengthBytes    int       `json:"udpDatagramLengthBytes,omitempty"`
}

type ServerConfig struct {
	Port        int    `json:"port"`
	BindAddress string `json:"bindAddress,omitempty"`
	OneOff      bool   `json:"oneOff"`
}

type Interval struct {
	RunID                 string    `json:"runId"`
	Sequence              int       `json:"sequence"`
	Direction             Direction `json:"direction"`
	StartSeconds          float64   `json:"startSeconds"`
	EndSeconds            float64   `json:"endSeconds"`
	BitsPerSecond         float64   `json:"bitsPerSecond"`
	Bytes                 int64     `json:"bytes,omitempty"`
	TCPRetransmits        int64     `json:"tcpRetransmits,omitempty"`
	UDPJitterMilliseconds float64   `json:"udpJitterMilliseconds,omitempty"`
	UDPLostDatagrams      int64     `json:"udpLostDatagrams,omitempty"`
	UDPTotalDatagrams     int64     `json:"udpTotalDatagrams,omitempty"`
}

type DirectionSummary struct {
	BitsPerSecond         float64 `json:"bitsPerSecond"`
	Bytes                 int64   `json:"bytes"`
	Seconds               float64 `json:"seconds"`
	TCPRetransmits        int64   `json:"tcpRetransmits,omitempty"`
	UDPJitterMilliseconds float64 `json:"udpJitterMilliseconds,omitempty"`
	UDPLostDatagrams      int64   `json:"udpLostDatagrams,omitempty"`
	UDPTotalDatagrams     int64   `json:"udpTotalDatagrams,omitempty"`
	UDPLostPercent        float64 `json:"udpLostPercent,omitempty"`
}

type Summary struct {
	Direction Direction         `json:"direction"`
	Upload    *DirectionSummary `json:"upload,omitempty"`
	Download  *DirectionSummary `json:"download,omitempty"`
}

type AppError struct {
	Code    string `json:"code"`
	Stage   string `json:"stage"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
	RunID   string `json:"runId,omitempty"`
}

type EngineCapabilities struct {
	JSONStream           bool `json:"jsonStream"`
	JSONStreamFullOutput bool `json:"jsonStreamFullOutput"`
	BidirTCP             bool `json:"bidirTcp"`
	BidirUDP             bool `json:"bidirUdp"`
	UDP                  bool `json:"udp"`
}

type NetworkInterface struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Family  string `json:"family"`
}

type EngineStatus struct {
	Available    bool               `json:"available"`
	Path         string             `json:"path,omitempty"`
	Version      string             `json:"version,omitempty"`
	Capabilities EngineCapabilities `json:"capabilities"`
	Interfaces   []NetworkInterface `json:"interfaces"`
	Error        *AppError          `json:"error,omitempty"`
	Probing      bool               `json:"probing,omitempty"`
}

type RunSnapshot struct {
	RunID           string       `json:"runId"`
	Status          RunStatus    `json:"status"`
	Config          ClientConfig `json:"config"`
	StartedAt       string       `json:"startedAt,omitempty"`
	EndedAt         string       `json:"endedAt,omitempty"`
	LastSequence    int          `json:"lastSequence"`
	LatestIntervals []Interval   `json:"latestIntervals,omitempty"`
	Summary         *Summary     `json:"summary,omitempty"`
	ExitCode        *int         `json:"exitCode,omitempty"`
	Error           *AppError    `json:"error,omitempty"`
	Message         string       `json:"message,omitempty"`
	RawOutput       string       `json:"rawOutput,omitempty"`
	RawTruncated    bool         `json:"rawTruncated,omitempty"`
	StderrSummary   string       `json:"stderrSummary,omitempty"`
	StderrTruncated bool         `json:"stderrTruncated,omitempty"`
	HistoryID       string       `json:"historyId,omitempty"`
}

type HistoryFilter struct {
	Query     string `json:"query"`
	Protocol  string `json:"protocol"`
	Direction string `json:"direction"`
	Page      int    `json:"page"`
	PageSize  int    `json:"pageSize"`
}

type HistoryItem struct {
	ID                    string    `json:"id"`
	CreatedAt             string    `json:"createdAt"`
	StartedAt             string    `json:"startedAt,omitempty"`
	EndedAt               string    `json:"endedAt,omitempty"`
	Status                RunStatus `json:"status"`
	Protocol              Protocol  `json:"protocol"`
	Direction             Direction `json:"direction"`
	Host                  string    `json:"host"`
	Port                  int       `json:"port"`
	DurationSeconds       int       `json:"durationSeconds"`
	UploadBitsPerSecond   float64   `json:"uploadBitsPerSecond,omitempty"`
	DownloadBitsPerSecond float64   `json:"downloadBitsPerSecond,omitempty"`
	ExitCode              *int      `json:"exitCode,omitempty"`
}

type HistoryPage struct {
	Items    []HistoryItem `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"pageSize"`
	Error    *AppError     `json:"error,omitempty"`
}

type RunRecord struct {
	ID            string       `json:"id"`
	CreatedAt     string       `json:"createdAt"`
	StartedAt     string       `json:"startedAt,omitempty"`
	EndedAt       string       `json:"endedAt,omitempty"`
	Status        RunStatus    `json:"status"`
	Protocol      Protocol     `json:"protocol"`
	Direction     Direction    `json:"direction"`
	Host          string       `json:"host"`
	Port          int          `json:"port"`
	Config        ClientConfig `json:"config"`
	OutputMode    string       `json:"outputMode,omitempty"`
	ParserVersion string       `json:"parserVersion,omitempty"`
	ExitCode      *int         `json:"exitCode,omitempty"`
	Summary       *Summary     `json:"summary,omitempty"`
}

type HistoryResult struct {
	Run             RunRecord  `json:"run"`
	Intervals       []Interval `json:"intervals"`
	IntervalTotal   int        `json:"intervalTotal"`
	Page            int        `json:"page"`
	PageSize        int        `json:"pageSize"`
	RawOutput       string     `json:"rawOutput,omitempty"`
	RawTruncated    bool       `json:"rawTruncated,omitempty"`
	StderrSummary   string     `json:"stderrSummary,omitempty"`
	StderrTruncated bool       `json:"stderrTruncated,omitempty"`
	Error           *AppError  `json:"error,omitempty"`
}

type StartRunResult struct {
	RunID string    `json:"runId,omitempty"`
	Error *AppError `json:"error,omitempty"`
}

type StopResult struct {
	Status RunStatus `json:"status"`
	Error  *AppError `json:"error,omitempty"`
}

type StartServerResult struct {
	ServerID string    `json:"serverId,omitempty"`
	Error    *AppError `json:"error,omitempty"`
}

type ExportResultInfo struct {
	Canceled bool      `json:"canceled"`
	Path     string    `json:"path,omitempty"`
	Error    *AppError `json:"error,omitempty"`
}

type DeleteResult struct {
	Deleted int       `json:"deleted"`
	Error   *AppError `json:"error,omitempty"`
}

type ServerConnection struct {
	Client                string    `json:"client"`
	Protocol              string    `json:"protocol"`
	Direction             Direction `json:"direction,omitempty"`
	StartedAt             string    `json:"startedAt,omitempty"`
	EndedAt               string    `json:"endedAt,omitempty"`
	Status                string    `json:"status"`
	UploadBitsPerSecond   float64   `json:"uploadBitsPerSecond,omitempty"`
	DownloadBitsPerSecond float64   `json:"downloadBitsPerSecond,omitempty"`
}

type ServerTestSnapshot struct {
	Client          string     `json:"client,omitempty"`
	Protocol        Protocol   `json:"protocol"`
	Direction       Direction  `json:"direction"`
	Status          RunStatus  `json:"status"`
	StartedAt       string     `json:"startedAt,omitempty"`
	EndedAt         string     `json:"endedAt,omitempty"`
	DurationSeconds int        `json:"durationSeconds,omitempty"`
	Intervals       []Interval `json:"intervals,omitempty"`
	Summary         *Summary   `json:"summary,omitempty"`
}

type ServerSnapshot struct {
	ServerID    string              `json:"serverId"`
	Sequence    int                 `json:"sequence"`
	Status      RunStatus           `json:"status"`
	BindAddress string              `json:"bindAddress,omitempty"`
	Port        int                 `json:"port"`
	StartedAt   string              `json:"startedAt,omitempty"`
	EndedAt     string              `json:"endedAt,omitempty"`
	ExitCode    *int                `json:"exitCode,omitempty"`
	Connections []ServerConnection  `json:"connections"`
	CurrentTest *ServerTestSnapshot `json:"currentTest,omitempty"`
	Logs        []string            `json:"logs"`
	Message     string              `json:"message,omitempty"`
	Error       *AppError           `json:"error,omitempty"`
}

type RunStateEvent struct {
	RunID     string    `json:"runId"`
	Sequence  int       `json:"sequence"`
	Status    RunStatus `json:"status"`
	StartedAt string    `json:"startedAt,omitempty"`
	Message   string    `json:"message,omitempty"`
}

type RunIntervalEvent struct {
	RunID     string     `json:"runId"`
	Sequence  int        `json:"sequence"`
	Intervals []Interval `json:"intervals"`
}

type RunCompletedEvent struct {
	RunID     string   `json:"runId"`
	Sequence  int      `json:"sequence"`
	Summary   *Summary `json:"summary,omitempty"`
	ExitCode  *int     `json:"exitCode,omitempty"`
	HistoryID string   `json:"historyId,omitempty"`
}

type RunFailedEvent struct {
	RunID         string    `json:"runId"`
	Sequence      int       `json:"sequence"`
	Stage         string    `json:"stage"`
	Error         *AppError `json:"error"`
	StderrSummary string    `json:"stderrSummary,omitempty"`
}

type ServerLogEvent struct {
	ServerID  string `json:"serverId"`
	Sequence  int    `json:"sequence"`
	Timestamp string `json:"timestamp"`
	Line      string `json:"line"`
}
