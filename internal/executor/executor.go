// Package executor runs OS commands and streams their output line-by-line.
package executor

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// waitDelay is how long to wait for I/O to drain after the process exits/is
// killed before forcibly closing the pipes. Prevents goroutine leaks when a
// process ignores signals or produces huge amounts of buffered output.
const waitDelay = 5 * time.Second

// LineType classifies a streamed output line.
type LineType string

const (
	LineTypeStdout LineType = "stdout"
	LineTypeStderr LineType = "stderr"
	LineTypeStart  LineType = "start"
	LineTypeExit   LineType = "exit"
	LineTypeError  LineType = "error"
)

// Line is a single unit of streaming output sent over WebSocket.
type Line struct {
	Type      LineType `json:"type"`
	Data      string   `json:"data,omitempty"`
	Timestamp int64    `json:"timestamp"`
	ExitCode  *int     `json:"exit_code,omitempty"`
}

// Options configures a command execution.
type Options struct {
	Command []string
	Dir     string
	Env     []string
	Stdout  bool
	Stderr  bool
}

// Run starts the command described by opts and returns a channel that receives
// output lines in real time. The channel is closed when the process exits.
// The caller must drain the channel to avoid blocking the scanner goroutines.
func Run(ctx context.Context, opts Options) (<-chan Line, error) {
	if len(opts.Command) == 0 {
		return nil, fmt.Errorf("no command specified")
	}

	cmd := exec.CommandContext(ctx, opts.Command[0], opts.Command[1:]...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	if len(opts.Env) > 0 {
		cmd.Env = opts.Env
	}
	// After context cancellation, forcibly close pipes after waitDelay so
	// scanner goroutines are never stranded waiting for output that won't come.
	cmd.WaitDelay = waitDelay

	out := make(chan Line, 256)

	var wg sync.WaitGroup

	if opts.Stdout {
		pipe, err := cmd.StdoutPipe()
		if err != nil {
			return nil, fmt.Errorf("stdout pipe: %w", err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := bufio.NewScanner(pipe)
			for sc.Scan() {
				select {
				case out <- Line{
					Type:      LineTypeStdout,
					Data:      sc.Text(),
					Timestamp: time.Now().UnixMilli(),
				}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	if opts.Stderr {
		pipe, err := cmd.StderrPipe()
		if err != nil {
			return nil, fmt.Errorf("stderr pipe: %w", err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := bufio.NewScanner(pipe)
			for sc.Scan() {
				select {
				case out <- Line{
					Type:      LineTypeStderr,
					Data:      sc.Text(),
					Timestamp: time.Now().UnixMilli(),
				}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start command: %w", err)
	}

	// Signal that the process launched successfully.
	out <- Line{Type: LineTypeStart, Timestamp: time.Now().UnixMilli()}

	go func() {
		defer close(out)
		wg.Wait()
		err := cmd.Wait()
		code := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			} else {
				// Non-exit error (e.g. process killed by signal with no ExitError).
				select {
				case out <- Line{
					Type:      LineTypeError,
					Data:      err.Error(),
					Timestamp: time.Now().UnixMilli(),
				}:
				default:
				}
			}
		}
		out <- Line{Type: LineTypeExit, ExitCode: &code, Timestamp: time.Now().UnixMilli()}
	}()

	return out, nil
}
