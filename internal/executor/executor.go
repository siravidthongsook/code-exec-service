package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"codeexecservice/internal/languages"
)

var oomWordRe = regexp.MustCompile(`\boom\b`)

type Executor struct {
	opts Options
}

func New(opts Options) (*Executor, error) {
	if strings.TrimSpace(opts.TmpDir) == "" {
		return nil, fmt.Errorf("tmp dir is required")
	}
	if opts.CompileTimeoutMs <= 0 {
		opts.CompileTimeoutMs = 10000
	}
	if opts.MaxOutputBytes <= 0 {
		opts.MaxOutputBytes = 65536
	}
	if opts.MaxSourceBytes <= 0 {
		opts.MaxSourceBytes = 256 * 1024
	}
	if opts.MaxStdinBytes <= 0 {
		opts.MaxStdinBytes = 256 * 1024
	}
	if opts.GoMinMemoryKb <= 0 {
		opts.GoMinMemoryKb = 2097152
	}
	if opts.JsMinMemoryKb <= 0 {
		opts.JsMinMemoryKb = 393216
	}
	if opts.JavaMinMemoryKb <= 0 {
		opts.JavaMinMemoryKb = 4194304
	}
	if strings.TrimSpace(opts.NsjailPath) == "" {
		opts.NsjailPath = "nsjail"
	}

	if err := os.MkdirAll(opts.TmpDir, 0o755); err != nil {
		return nil, fmt.Errorf("create tmp dir: %w", err)
	}

	if opts.UseNsjail {
		if _, err := exec.LookPath(opts.NsjailPath); err != nil {
			return nil, fmt.Errorf("nsjail not found at %q", opts.NsjailPath)
		}
		if strings.TrimSpace(opts.NsjailTemplate) == "" {
			if strings.TrimSpace(opts.NsjailConfigPath) == "" {
				return nil, fmt.Errorf("nsjail config path is required when nsjail is enabled")
			}
			raw, err := os.ReadFile(opts.NsjailConfigPath)
			if err != nil {
				return nil, fmt.Errorf("read nsjail config: %w", err)
			}
			opts.NsjailTemplate = string(raw)
		}
	}

	return &Executor{opts: opts}, nil
}

func (e *Executor) Execute(ctx context.Context, req Request) ExecuteResponse {
	detailed := e.RunDetailed(ctx, req)
	if detailed.Error != "" {
		return ExecuteResponse{
			Status:       "INTERNAL",
			SandboxError: detailed.Error,
			ExitCode:     -1,
		}
	}

	if detailed.Compile != nil && (detailed.Compile.ExitCode != 0 || detailed.Compile.TimedOut) {
		stderr := strings.TrimSpace(detailed.Compile.Stderr)
		if stderr == "" {
			stderr = strings.TrimSpace(detailed.Compile.Stdout)
		}
		return ExecuteResponse{
			Stdout:          detailed.Compile.Stdout,
			Stderr:          stderr,
			ExitCode:        detailed.Compile.ExitCode,
			TimeMs:          detailed.Compile.DurationMs,
			MemoryKb:        detailed.Compile.MemoryKb,
			Status:          "CE",
			OutputTruncated: detailed.Compile.OutputTruncated,
		}
	}

	if detailed.Run == nil {
		return ExecuteResponse{
			Status:       "INTERNAL",
			SandboxError: "no run result",
			ExitCode:     -1,
		}
	}

	status := mapRunStatus(detailed.Run)
	return ExecuteResponse{
		Stdout:          detailed.Run.Stdout,
		Stderr:          detailed.Run.Stderr,
		ExitCode:        detailed.Run.ExitCode,
		TimeMs:          detailed.Run.DurationMs,
		MemoryKb:        detailed.Run.MemoryKb,
		Status:          status,
		OutputTruncated: detailed.Run.OutputTruncated,
	}
}

func (e *Executor) RunDetailed(ctx context.Context, req Request) DetailedResponse {
	if err := validateRequest(e.opts, req); err != nil {
		return DetailedResponse{Error: err.Error()}
	}

	lang := languages.Normalize(req.Language)
	spec, ok := languages.Lookup(lang)
	if !ok {
		return DetailedResponse{Error: "unsupported language"}
	}

	applyRuntimeOverrides(&spec, req)
	if strings.TrimSpace(spec.Run) == "" {
		return DetailedResponse{Error: "missing run command"}
	}

	workdir, err := os.MkdirTemp(e.opts.TmpDir, "job-")
	if err != nil {
		return DetailedResponse{Error: "failed to create workdir"}
	}
	if !e.opts.KeepWorkdir {
		defer func() { _ = os.RemoveAll(workdir) }()
	}

	sourcePath := filepath.Join(workdir, filenameForLang(lang))
	if err := os.WriteFile(sourcePath, []byte(req.Source), 0o644); err != nil {
		return DetailedResponse{Error: "failed to write source"}
	}
	exePath := filepath.Join(workdir, "main.out")

	maxOutput := req.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = e.opts.MaxOutputBytes
	}

	effectiveMemoryKb := e.effectiveMemoryLimit(lang, req.MemoryLimitKb)

	var compileResult *ExecResult
	if strings.TrimSpace(spec.Compile) != "" {
		cmd := expandCmd(spec.Compile, sourcePath, exePath, workdir)
		res := runCommandDirect(ctx, cmd, workdir, "", e.opts.CompileTimeoutMs, maxOutput)
		compileResult = &res
		if res.ExitCode != 0 || res.TimedOut {
			return DetailedResponse{Compile: compileResult}
		}
	}

	runTimeout := req.TimeLimitMs
	if runTimeout <= 0 {
		runTimeout = 2000
	}
	runCmd := expandCmd(spec.Run, sourcePath, exePath, workdir)
	runRes := e.runCommand(ctx, runCmd, workdir, req.Stdin, runTimeout, maxOutput, effectiveMemoryKb, req.AllowNetwork)

	return DetailedResponse{Compile: compileResult, Run: &runRes}
}

func (e *Executor) runCommand(
	ctx context.Context,
	cmdLine,
	dir,
	stdin string,
	timeoutMs,
	maxOutput,
	memoryKb int,
	allowNetwork bool,
) ExecResult {
	if e.opts.UseNsjail {
		return runCommandNsjail(ctx, e.opts, cmdLine, dir, stdin, timeoutMs, maxOutput, memoryKb, allowNetwork)
	}
	return runCommandDirect(ctx, cmdLine, dir, stdin, timeoutMs, maxOutput)
}

func (e *Executor) effectiveMemoryLimit(lang string, requested int) int {
	effective := requested
	if lang == "go" && effective > 0 && effective < e.opts.GoMinMemoryKb {
		effective = e.opts.GoMinMemoryKb
	}
	if (lang == "js" || lang == "javascript") && effective > 0 && effective < e.opts.JsMinMemoryKb {
		effective = e.opts.JsMinMemoryKb
	}
	if lang == "java" && effective > 0 && effective < e.opts.JavaMinMemoryKb {
		effective = e.opts.JavaMinMemoryKb
	}
	if lang == "java" && e.opts.JavaDisableAS {
		effective = 0
	}
	return effective
}

func validateRequest(opts Options, req Request) error {
	if strings.TrimSpace(req.Language) == "" {
		return fmt.Errorf("language is required")
	}
	if len(req.Source) == 0 {
		return fmt.Errorf("source is required")
	}
	if len(req.Source) > opts.MaxSourceBytes {
		return fmt.Errorf("source too large")
	}
	if len(req.Stdin) > opts.MaxStdinBytes {
		return fmt.Errorf("stdin too large")
	}
	return nil
}

func applyRuntimeOverrides(spec *languages.Spec, req Request) {
	if spec == nil {
		return
	}
	if strings.TrimSpace(req.CompileCmd) != "" {
		spec.Compile = req.CompileCmd
	}
	if strings.TrimSpace(req.RunCmd) != "" {
		spec.Run = req.RunCmd
	}
	if req.RuntimeOverrides == nil {
		return
	}
	if strings.TrimSpace(spec.Compile) == "" {
		if value, ok := req.RuntimeOverrides["compile_cmd"]; ok {
			spec.Compile = strings.TrimSpace(value)
		}
	}
	if value, ok := req.RuntimeOverrides["run_cmd"]; ok && strings.TrimSpace(value) != "" {
		spec.Run = strings.TrimSpace(value)
	}
}

func mapRunStatus(res *ExecResult) string {
	if res == nil {
		return "INTERNAL"
	}
	if res.TimedOut {
		return "TLE"
	}
	if looksLikeMemoryLimit(res.Stderr) {
		return "MLE"
	}
	if res.ExitCode != 0 {
		return "RE"
	}
	return "OK"
}

func looksLikeMemoryLimit(stderr string) bool {
	s := strings.ToLower(strings.TrimSpace(stderr))
	if s == "" {
		return false
	}
	return strings.Contains(s, "memory limit") ||
		strings.Contains(s, "out of memory") ||
		strings.Contains(s, "cannot allocate memory") ||
		strings.Contains(s, "failed to reserve virtual memory for coderange") ||
		strings.Contains(s, "mmap failed") ||
		strings.Contains(s, "killed process") ||
		oomWordRe.MatchString(s)
}

func runCommandDirect(ctx context.Context, cmdLine, dir, stdin string, timeoutMs, maxOutput int) ExecResult {
	if timeoutMs <= 0 {
		timeoutMs = 2000
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "/bin/sh", "-c", cmdLine)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), buildTempEnv(dir)...)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	cmd.Stdin = strings.NewReader(stdin)

	stdoutBuf := &limitedBuffer{max: maxOutput}
	stderrBuf := &limitedBuffer{max: maxOutput}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	start := time.Now()
	err := cmd.Run()
	duration := int(time.Since(start).Milliseconds())

	timedOut := false
	if runCtx.Err() == context.DeadlineExceeded {
		timedOut = true
		if cmd.Process != nil && runtime.GOOS != "windows" {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}

	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	} else if err == nil {
		exitCode = 0
	}

	return ExecResult{
		ExitCode:        exitCode,
		Stdout:          stdoutBuf.String(),
		Stderr:          stderrBuf.String(),
		TimedOut:        timedOut,
		DurationMs:      duration,
		MemoryKb:        0,
		OutputTruncated: stdoutBuf.truncated || stderrBuf.truncated,
	}
}

func runCommandNsjail(
	ctx context.Context,
	opts Options,
	cmdLine,
	dir,
	stdin string,
	timeoutMs,
	maxOutput,
	memoryKb int,
	allowNetwork bool,
) ExecResult {
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs+1500)*time.Millisecond)
	defer cancel()

	configPath, err := writeNsjailConfig(opts, dir)
	if err != nil {
		return ExecResult{ExitCode: -1, Stderr: err.Error()}
	}

	args := []string{"--config", configPath, "-B", dir + ":/work"}
	if timeoutMs > 0 {
		seconds := (timeoutMs + 999) / 1000
		args = append(args, "--time_limit", strconv.Itoa(seconds))
	}
	if memoryKb > 0 {
		memoryMiB := (memoryKb + 1023) / 1024
		if memoryMiB < 1 {
			memoryMiB = 1
		}
		args = append(args, "--rlimit_as", strconv.Itoa(memoryMiB))
	}
	if allowNetwork {
		args = append(args, "--disable_clone_newnet")
	}

	tmpDirInJail := "/work/.tmp"
	_ = os.MkdirAll(filepath.Join(dir, ".tmp"), 0o755)
	args = append(
		args,
		"--",
		"/usr/bin/env",
		"TMPDIR="+tmpDirInJail,
		"TMP="+tmpDirInJail,
		"TEMP="+tmpDirInJail,
		"GOTMPDIR="+tmpDirInJail,
		"HOME=/work",
		"XDG_CACHE_HOME="+tmpDirInJail,
		"GOCACHE="+tmpDirInJail+"/go-build-cache",
		"LD_LIBRARY_PATH=/usr/lib/jvm/java-17-openjdk-arm64/lib:/usr/lib/jvm/java-17-openjdk-amd64/lib:/usr/lib/jvm/default-java/lib",
		"/bin/sh",
		"-c",
		cmdLine,
	)

	cmd := exec.CommandContext(runCtx, opts.NsjailPath, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), buildTempEnv(dir)...)
	cmd.Stdin = strings.NewReader(stdin)

	stdoutBuf := &limitedBuffer{max: maxOutput}
	stderrBuf := &limitedBuffer{max: maxOutput}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	start := time.Now()
	err = cmd.Run()
	duration := int(time.Since(start).Milliseconds())

	timedOut := false
	if runCtx.Err() == context.DeadlineExceeded {
		timedOut = true
		if cmd.Process != nil && runtime.GOOS != "windows" {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}

	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	} else if err == nil {
		exitCode = 0
	}

	return ExecResult{
		ExitCode:        exitCode,
		Stdout:          stdoutBuf.String(),
		Stderr:          stderrBuf.String(),
		TimedOut:        timedOut,
		DurationMs:      duration,
		MemoryKb:        processMemoryKB(cmd.ProcessState),
		OutputTruncated: stdoutBuf.truncated || stderrBuf.truncated,
	}
}

func buildTempEnv(workdir string) []string {
	tmpDir := filepath.Join(workdir, ".tmp")
	_ = os.MkdirAll(tmpDir, 0o755)
	return []string{
		"TMPDIR=" + tmpDir,
		"TMP=" + tmpDir,
		"TEMP=" + tmpDir,
		"GOTMPDIR=" + tmpDir,
	}
}

func writeNsjailConfig(opts Options, workdir string) (string, error) {
	if strings.TrimSpace(opts.NsjailTemplate) == "" {
		return "", fmt.Errorf("nsjail template missing")
	}
	rendered := strings.ReplaceAll(opts.NsjailTemplate, "{{WORKDIR}}", workdir)
	path := filepath.Join(workdir, "nsjail.cfg")
	if err := os.WriteFile(path, []byte(rendered), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func processMemoryKB(state *os.ProcessState) int {
	if state == nil {
		return 0
	}
	usage, ok := state.SysUsage().(*syscall.Rusage)
	if !ok || usage == nil {
		return 0
	}
	maxRSS := int(usage.Maxrss)
	if maxRSS <= 0 {
		return 0
	}
	if runtime.GOOS == "darwin" {
		return maxRSS / 1024
	}
	return maxRSS
}

func filenameForLang(lang string) string {
	switch lang {
	case "cpp":
		return "main.cpp"
	case "c":
		return "main.c"
	case "js", "javascript":
		return "main.js"
	case "java":
		return "Main.java"
	case "go":
		return "main.go"
	default:
		return "main.py"
	}
}

func expandCmd(cmd, src, exe, workdir string) string {
	out := strings.ReplaceAll(cmd, "{src}", shellEscape(src))
	out = strings.ReplaceAll(out, "{exe}", shellEscape(exe))
	out = strings.ReplaceAll(out, "{workdir}", shellEscape(workdir))
	return out
}

func shellEscape(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\n\"'\\$") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

type limitedBuffer struct {
	buf       []byte
	max       int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.max <= 0 {
		b.buf = append(b.buf, p...)
		return len(p), nil
	}
	remaining := b.max - len(b.buf)
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		b.buf = append(b.buf, p[:remaining]...)
		b.truncated = true
		return len(p), nil
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	return string(b.buf)
}
