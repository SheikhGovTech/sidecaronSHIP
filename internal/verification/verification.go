package verification

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/sausheong/sidecar/internal/config"
)

const MaxOutputBytes = 64 * 1024

type Result struct {
	Name      string
	ExitCode  int
	Output    string
	Duration  time.Duration
	Truncated bool
	Err       error
}

type boundedBuffer struct {
	buf bytes.Buffer
	n   int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	remaining := MaxOutputBytes - b.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.buf.Write(p)
	}
	b.n += original
	return original, nil
}

func (b *boundedBuffer) Truncated() bool { return b.n > b.buf.Len() }

func ResolveWorkingDirectory(root, relative string) (string, error) {
	if relative == "" {
		relative = "."
	}
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("working directory must be relative")
	}
	clean := filepath.Clean(relative)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("working directory escapes workspace")
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolving workspace: %w", err)
	}
	target := filepath.Join(rootReal, clean)
	targetReal, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", fmt.Errorf("resolving working directory: %w", err)
	}
	rel, err := filepath.Rel(rootReal, targetReal)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("working directory escapes workspace")
	}
	info, err := os.Stat(targetReal)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("working directory is not a directory")
	}
	return targetReal, nil
}

func commandEnv(pass []string) ([]string, map[string]string) {
	env := []string{}
	values := map[string]string{}
	for _, name := range pass {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
			values[name] = value
		}
	}
	return env, values
}

// Preflight checks working-directory containment and explicitly declared tools
// before the coding model starts.
func Preflight(root string, commands []config.VerificationCommand) (string, error) {
	for _, command := range commands {
		if _, err := ResolveWorkingDirectory(root, command.WorkingDirectory); err != nil {
			return command.Name, fmt.Errorf("verification command %q: %w", command.Name, err)
		}
		_, values := commandEnv(command.PassEnv)
		pathValue := values["PATH"]
		for _, tool := range command.RequiredTools {
			if _, err := lookPath(tool, pathValue); err != nil {
				return command.Name, fmt.Errorf("verification command %q requires unavailable tool %q", command.Name, tool)
			}
		}
	}
	return "", nil
}

func lookPath(file, pathValue string) (string, error) {
	if pathValue == "" {
		return "", exec.ErrNotFound
	}
	for _, dir := range filepath.SplitList(pathValue) {
		candidate := filepath.Join(dir, file)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}

func Run(ctx context.Context, root string, commands []config.VerificationCommand) []Result {
	results := make([]Result, 0, len(commands))
	for _, command := range commands {
		result := RunOne(ctx, root, command)
		results = append(results, result)
		if result.Err != nil || result.ExitCode != 0 {
			break
		}
	}
	return results
}

// RunOne executes one command through /bin/sh -c as the current OS user.
func RunOne(ctx context.Context, root string, command config.VerificationCommand) Result {
	result := Result{Name: command.Name, ExitCode: -1}
	dir, err := ResolveWorkingDirectory(root, command.WorkingDirectory)
	if err != nil {
		result.Err = err
		return result
	}
	timeout := command.ParsedTimeout()
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.Command("/bin/sh", "-c", command.Run)
	cmd.Dir = dir
	cmd.Env, _ = commandEnv(command.PassEnv)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var output boundedBuffer
	cmd.Stdout, cmd.Stderr = &output, &output
	started := time.Now()
	if err := cmd.Start(); err != nil {
		result.Err = err
		return result
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-commandCtx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		err = <-done
		result.Err = fmt.Errorf("verification command timed out after %s", timeout)
	}
	result.Duration = time.Since(started)
	result.Output = redact(output.buf.String(), command.PassEnv)
	result.Truncated = output.Truncated()
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if result.Err == nil && err != nil {
		result.Err = err
	}
	return result
}

func redact(output string, names []string) string {
	for _, name := range names {
		lower := strings.ToLower(name)
		if !strings.Contains(lower, "token") && !strings.Contains(lower, "key") && !strings.Contains(lower, "secret") && !strings.Contains(lower, "password") && !strings.Contains(lower, "credential") && !strings.Contains(lower, "database") && !strings.Contains(lower, "dsn") {
			continue
		}
		if value := os.Getenv(name); value != "" {
			output = strings.ReplaceAll(output, value, "[REDACTED]")
		}
	}
	return output
}

func PromptBlock(commands []config.VerificationCommand) string {
	if len(commands) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Required verification:\n")
	for _, command := range commands {
		dir := command.WorkingDirectory
		if dir == "" {
			dir = "."
		}
		fmt.Fprintf(&b, "- %s: %s (directory: %s, timeout: %s)\n", command.Name, command.Run, dir, command.ParsedTimeout())
	}
	return strings.TrimSpace(b.String())
}
