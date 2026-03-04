package executor

type Request struct {
	Language         string
	Source           string
	Stdin            string
	TimeLimitMs      int
	MemoryLimitKb    int
	RuntimeOverrides map[string]string
	RequestID        string

	CompileCmd     string
	RunCmd         string
	MaxOutputBytes int
	AllowNetwork   bool
}

type ExecResult struct {
	ExitCode        int    `json:"exitCode"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	TimedOut        bool   `json:"timedOut"`
	DurationMs      int    `json:"durationMs"`
	MemoryKb        int    `json:"memoryKb,omitempty"`
	OutputTruncated bool   `json:"outputTruncated"`
}

type DetailedResponse struct {
	Compile *ExecResult `json:"compile,omitempty"`
	Run     *ExecResult `json:"run,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type ExecuteResponse struct {
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	ExitCode        int    `json:"exit_code"`
	TimeMs          int    `json:"time_ms"`
	MemoryKb        int    `json:"memory_kb"`
	Status          string `json:"status"`
	SandboxError    string `json:"sandbox_error,omitempty"`
	OutputTruncated bool   `json:"output_truncated"`
}

type Options struct {
	TmpDir           string
	CompileTimeoutMs int
	MaxOutputBytes   int
	MaxSourceBytes   int
	MaxStdinBytes    int

	GoMinMemoryKb   int
	JsMinMemoryKb   int
	JavaMinMemoryKb int
	JavaDisableAS   bool

	KeepWorkdir bool
	UseNsjail   bool
	NsjailPath  string

	NsjailConfigPath string
	NsjailTemplate   string
}
