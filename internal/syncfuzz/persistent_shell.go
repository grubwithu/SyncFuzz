package syncfuzz

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

type persistentShell struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan string
	seq    int
	closed bool
}

// ShellState is a small probe of process-local shell state. This is the state
// class most likely to drift when an agent framework checkpoints graph data but
// keeps a long-lived shell process alive.
type ShellState struct {
	PWD           string   `json:"pwd"`
	PATH          string   `json:"path"`
	GitResolution string   `json:"git_resolution"`
	Aliases       []string `json:"aliases"`
	Raw           []string `json:"raw"`
}

// startPersistentShell creates the long-lived shell used by the poisoning seed.
// It mirrors framework designs where many tool calls share one shell session.
func startPersistentShell(ctx context.Context, dir string) (*persistentShell, error) {
	cmd := exec.CommandContext(ctx, "bash", "--noprofile", "--norc")
	cmd.Dir = dir
	return startPersistentShellCommand(ctx, cmd)
}

func startPersistentShellCommand(ctx context.Context, cmd *exec.Cmd) (*persistentShell, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	shell := &persistentShell{
		cmd:   cmd,
		stdin: stdin,
		lines: make(chan string, 128),
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go scanShellOutput(stdout, shell.lines)
	go scanShellOutput(stderr, shell.lines)

	if _, err := shell.Run(ctx, "shopt -s expand_aliases"); err != nil {
		shell.Close()
		return nil, err
	}
	return shell, nil
}

func scanShellOutput(reader io.Reader, lines chan<- string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		lines <- scanner.Text()
	}
}

func (s *persistentShell) Run(ctx context.Context, command string) ([]string, error) {
	if s.closed {
		return nil, fmt.Errorf("persistent shell already closed")
	}
	s.seq++
	marker := fmt.Sprintf("__SYNCFUZZ_DONE_%d__", s.seq)
	// The marker turns an interactive shell into a simple request/response
	// channel: collect output until the marker announces command completion.
	wrapped := fmt.Sprintf("%s\nprintf '%s:%%s\\n' \"$?\"\n", command, marker)
	if _, err := io.WriteString(s.stdin, wrapped); err != nil {
		return nil, err
	}

	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	var out []string
	for {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-timeout.C:
			return out, fmt.Errorf("persistent shell command timed out")
		case line := <-s.lines:
			if strings.HasPrefix(line, marker+":") {
				return out, nil
			}
			out = append(out, line)
		}
	}
}

func (s *persistentShell) Probe(ctx context.Context) (ShellState, error) {
	// Keep the probe small and explicit. Future probes can add umask, functions,
	// jobs, fds, and namespace state without changing the runner shape.
	lines, err := s.Run(ctx, strings.Join([]string{
		`printf 'PWD=%s\n' "$PWD"`,
		`printf 'PATH=%s\n' "$PATH"`,
		`printf 'GIT=%s\n' "$(command -v git || true)"`,
		`alias -p`,
	}, "\n"))
	if err != nil {
		return ShellState{}, err
	}

	state := ShellState{Raw: lines, Aliases: []string{}}
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "PWD="):
			state.PWD = strings.TrimPrefix(line, "PWD=")
		case strings.HasPrefix(line, "PATH="):
			state.PATH = strings.TrimPrefix(line, "PATH=")
		case strings.HasPrefix(line, "GIT="):
			state.GitResolution = strings.TrimPrefix(line, "GIT=")
		case strings.HasPrefix(line, "alias "):
			state.Aliases = append(state.Aliases, line)
		}
	}
	return state, nil
}

func (s *persistentShell) Close() {
	if s.closed {
		return
	}
	s.closed = true
	_, _ = io.WriteString(s.stdin, "exit\n")
	_ = s.stdin.Close()
	_ = s.cmd.Wait()
}
