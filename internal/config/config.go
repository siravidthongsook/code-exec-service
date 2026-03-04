package config

import (
	"os"
	"runtime"
	"strconv"
	"strings"

	"codeexecservice/internal/executor"
)

type Config struct {
	Addr              string
	ReadHeaderTimeout int
	MaxBodyBytes      int64
	Executor          executor.Options
}

func Load() Config {
	port := firstNonEmpty(os.Getenv("CODE_EXEC_PORT"), "7070")
	addr := firstNonEmpty(os.Getenv("CODE_EXEC_ADDR"), ":"+port)
	tmpDir := firstNonEmpty(os.Getenv("CODE_EXEC_TMP_DIR"), "/tmp/code-exec")

	compileTimeoutMs := envInt("CODE_EXEC_COMPILE_TIMEOUT_MS", 10000)
	maxOutputBytes := envInt("CODE_EXEC_MAX_OUTPUT_BYTES", 65536)
	maxSourceBytes := envInt("CODE_EXEC_MAX_SOURCE_BYTES", 262144)
	maxStdinBytes := envInt("CODE_EXEC_MAX_STDIN_BYTES", 262144)

	goMinMemoryKb := envInt("CODE_EXEC_GO_MIN_MEMORY_KB", 2097152)
	jsMinMemoryKb := envInt("CODE_EXEC_JS_MIN_MEMORY_KB", 393216)
	javaMinMemoryKb := envInt("CODE_EXEC_JAVA_MIN_MEMORY_KB", 4194304)
	javaDisableAS := envBool("CODE_EXEC_JAVA_DISABLE_RLIMIT_AS", true)

	keepWorkdir := envBool("CODE_EXEC_KEEP_WORKDIR", false)
	useNsjail := envBool("CODE_EXEC_USE_NSJAIL", runtime.GOOS == "linux")
	nsjailPath := firstNonEmpty(os.Getenv("CODE_EXEC_NSJAIL_PATH"), "nsjail")
	nsjailConfig := firstNonEmpty(os.Getenv("CODE_EXEC_NSJAIL_CONFIG"), "./nsjail/nsjail.cfg")

	readHeaderTimeout := envInt("CODE_EXEC_READ_HEADER_TIMEOUT_SEC", 5)
	maxBodyBytes := int64(envInt("CODE_EXEC_MAX_BODY_BYTES", 2*1024*1024))

	return Config{
		Addr:              addr,
		ReadHeaderTimeout: readHeaderTimeout,
		MaxBodyBytes:      maxBodyBytes,
		Executor: executor.Options{
			TmpDir:           tmpDir,
			CompileTimeoutMs: compileTimeoutMs,
			MaxOutputBytes:   maxOutputBytes,
			MaxSourceBytes:   maxSourceBytes,
			MaxStdinBytes:    maxStdinBytes,
			GoMinMemoryKb:    goMinMemoryKb,
			JsMinMemoryKb:    jsMinMemoryKb,
			JavaMinMemoryKb:  javaMinMemoryKb,
			JavaDisableAS:    javaDisableAS,
			KeepWorkdir:      keepWorkdir,
			UseNsjail:        useNsjail,
			NsjailPath:       nsjailPath,
			NsjailConfigPath: nsjailConfig,
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
