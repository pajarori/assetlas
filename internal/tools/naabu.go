package tools

import (
	"context"
	"strconv"
)

type NaabuResult struct {
	IP       string `json:"ip"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type NaabuOptions struct {
	Hosts     []string
	Ports     string
	TopPorts  string
	Threads   int
	Timeout   int
	RateLimit int
	ScanType  string
}

func RunNaabu(ctx context.Context, opts NaabuOptions, emit func(NaabuResult) error) error {
	if len(opts.Hosts) == 0 {
		return nil
	}
	scanType := opts.ScanType
	if scanType == "" {
		scanType = "c"
	}
	args := []string{"-silent", "-nc", "-json", "-s", scanType}
	switch {
	case opts.Ports != "":
		args = append(args, "-p", opts.Ports)
	case opts.TopPorts != "":
		args = append(args, "-top-ports", opts.TopPorts)
	default:
		args = append(args, "-top-ports", "100")
	}
	if opts.Threads > 0 {
		args = append(args, "-c", strconv.Itoa(opts.Threads))
	}
	if opts.RateLimit > 0 {
		args = append(args, "-rate", strconv.Itoa(opts.RateLimit))
	}
	if opts.Timeout > 0 {
		args = append(args, "-timeout", strconv.Itoa(opts.Timeout))
	}
	return RunJSONL(ctx, StreamOptions{Binary: "naabu", Args: args, Stdin: opts.Hosts}, emit)
}
