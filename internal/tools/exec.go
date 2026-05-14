package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

const maxScanBuffer = 1 << 20

type StreamOptions struct {
	Binary string
	Args   []string
	Stdin  []string
	Env    []string
}

var lookPathCache sync.Map

func resolveBinary(name string) error {
	if v, ok := lookPathCache.Load(name); ok {
		if v == nil {
			return nil
		}
		return v.(error)
	}
	_, err := exec.LookPath(name)
	if err != nil {
		wrapped := fmt.Errorf("%s not found in PATH: install with `make tools`", name)
		lookPathCache.Store(name, wrapped)
		return wrapped
	}
	lookPathCache.Store(name, nil)
	return nil
}

func startCmd(ctx context.Context, opts StreamOptions) (*exec.Cmd, *bufio.Scanner, error) {
	if err := resolveBinary(opts.Binary); err != nil {
		return nil, nil, err
	}
	cmd := exec.CommandContext(ctx, opts.Binary, opts.Args...)
	if len(opts.Env) > 0 {
		cmd.Env = append(os.Environ(), opts.Env...)
	}
	if len(opts.Stdin) > 0 {
		cmd.Stdin = strings.NewReader(strings.Join(opts.Stdin, "\n") + "\n")
	}
	cmd.Stderr = newPrefixedWriter(opts.Binary)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("%s stdout pipe: %w", opts.Binary, err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("%s start: %w", opts.Binary, err)
	}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64<<10), maxScanBuffer)
	return cmd, sc, nil
}

func killAndDrain(cmd *exec.Cmd, sc *bufio.Scanner) {
	_ = cmd.Process.Kill()
	for sc.Scan() {
	}
	_ = cmd.Wait()
}

func waitForExit(cmd *exec.Cmd, binary string) error {
	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 0 {
			return nil
		}
		return fmt.Errorf("%s wait: %w", binary, err)
	}
	return nil
}

func RunJSONL[T any](ctx context.Context, opts StreamOptions, emit func(T) error) error {
	cmd, sc, err := startCmd(ctx, opts)
	if err != nil {
		return err
	}
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var v T
		if err := json.Unmarshal(line, &v); err != nil {
			log.Debug().Err(err).Str("binary", opts.Binary).Str("line", string(line)).Msg("json parse skipped")
			continue
		}
		if err := emit(v); err != nil {
			killAndDrain(cmd, sc)
			return err
		}
	}
	if err := sc.Err(); err != nil {
		killAndDrain(cmd, sc)
		return fmt.Errorf("%s scan: %w", opts.Binary, err)
	}
	return waitForExit(cmd, opts.Binary)
}

func RunLines(ctx context.Context, opts StreamOptions, emit func(string) error) error {
	cmd, sc, err := startCmd(ctx, opts)
	if err != nil {
		return err
	}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if err := emit(line); err != nil {
			killAndDrain(cmd, sc)
			return err
		}
	}
	if err := sc.Err(); err != nil {
		killAndDrain(cmd, sc)
		return fmt.Errorf("%s scan: %w", opts.Binary, err)
	}
	return waitForExit(cmd, opts.Binary)
}

type prefixedWriter struct {
	prefix string
	buf    strings.Builder
}

func newPrefixedWriter(prefix string) io.Writer {
	return &prefixedWriter{prefix: prefix}
}

func (w *prefixedWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	s := w.buf.String()
	for {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(s[:i], "\r")
		if line != "" {
			log.Debug().Str("tool", w.prefix).Msg(line)
		}
		s = s[i+1:]
	}
	w.buf.Reset()
	w.buf.WriteString(s)
	return len(p), nil
}
