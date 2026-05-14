package tools

import (
	"context"
	"strconv"
)

type DnsxResult struct {
	Host     string           `json:"host"`
	A        []string         `json:"a"`
	AAAA     []string         `json:"aaaa"`
	CNAME    []string         `json:"cname"`
	MX       []string         `json:"mx"`
	NS       []string         `json:"ns"`
	TXT      []string         `json:"txt"`
	SOA      []map[string]any `json:"soa"`
	PTR      []string         `json:"ptr"`
	Resolver []string         `json:"resolver"`
	Status   string           `json:"status_code"`
}

type DnsxOptions struct {
	Hosts     []string
	Threads   int
	Timeout   int
	RateLimit int
	Resolvers string
}

func RunDnsx(ctx context.Context, opts DnsxOptions, emit func(DnsxResult) error) error {
	if len(opts.Hosts) == 0 {
		return nil
	}
	args := []string{"-silent", "-nc", "-json", "-a", "-aaaa", "-cname", "-mx", "-ns", "-txt", "-resp"}
	if opts.Threads > 0 {
		args = append(args, "-t", strconv.Itoa(opts.Threads))
	}
	if opts.Timeout > 0 {
		args = append(args, "-timeout", strconv.Itoa(opts.Timeout))
	}
	if opts.RateLimit > 0 {
		args = append(args, "-rl", strconv.Itoa(opts.RateLimit))
	}
	if opts.Resolvers != "" {
		args = append(args, "-r", opts.Resolvers)
	}
	return RunJSONL(ctx, StreamOptions{Binary: "dnsx", Args: args, Stdin: opts.Hosts}, emit)
}
