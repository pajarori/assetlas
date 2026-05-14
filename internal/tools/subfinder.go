package tools

import (
	"context"
	"fmt"
	"strconv"
)

type SubfinderResult struct {
	Host   string `json:"host"`
	Source string `json:"source"`
	Input  string `json:"input"`
}

type SubfinderOptions struct {
	Domain     string
	Threads    int
	Timeout    int
	RateLimit  int
	AllSources bool
	Resolvers  string
}

func RunSubfinder(ctx context.Context, opts SubfinderOptions, emit func(SubfinderResult) error) error {
	if opts.Domain == "" {
		return fmt.Errorf("subfinder: domain required")
	}
	args := []string{"-d", opts.Domain, "-silent", "-nc", "-oJ"}
	if opts.Threads > 0 {
		args = append(args, "-t", strconv.Itoa(opts.Threads))
	}
	if opts.Timeout > 0 {
		args = append(args, "-timeout", strconv.Itoa(opts.Timeout))
	}
	if opts.RateLimit > 0 {
		args = append(args, "-rl", strconv.Itoa(opts.RateLimit))
	}
	if opts.AllSources {
		args = append(args, "-all")
	}
	if opts.Resolvers != "" {
		args = append(args, "-rL", opts.Resolvers)
	}
	return RunJSONL(ctx, StreamOptions{Binary: "subfinder", Args: args}, emit)
}
