package tools

import (
	"context"
	"strconv"
)

type HttpxResult struct {
	URL           string   `json:"url"`
	Host          string   `json:"host"`
	Input         string   `json:"input"`
	StatusCode    int      `json:"status_code"`
	ContentLength int      `json:"content_length"`
	Title         string   `json:"title"`
	Webserver     string   `json:"webserver"`
	Tech          []string `json:"tech"`
	IP            string   `json:"ip"`
	CNAME         []string `json:"cname"`
	ASN           struct {
		Number  string `json:"as_number"`
		Name    string `json:"as_name"`
		Country string `json:"as_country"`
	} `json:"asn"`
	ResponseTime string `json:"time"`
	Scheme       string `json:"scheme"`
	Port         string `json:"port"`
}

type HttpxOptions struct {
	Hosts     []string
	Threads   int
	Timeout   int
	RateLimit int
	NoFollow  bool
}

func RunHttpx(ctx context.Context, opts HttpxOptions, emit func(HttpxResult) error) error {
	if len(opts.Hosts) == 0 {
		return nil
	}
	args := []string{"-silent", "-nc", "-json", "-status-code", "-title", "-tech-detect", "-ip", "-cname", "-asn", "-server"}
	if opts.Threads > 0 {
		args = append(args, "-threads", strconv.Itoa(opts.Threads))
	}
	if opts.Timeout > 0 {
		args = append(args, "-timeout", strconv.Itoa(opts.Timeout))
	}
	if opts.RateLimit > 0 {
		args = append(args, "-rl", strconv.Itoa(opts.RateLimit))
	}
	if opts.NoFollow {
		args = append(args, "-no-fallback-scheme")
	}
	return RunJSONL(ctx, StreamOptions{Binary: "httpx", Args: args, Stdin: opts.Hosts}, emit)
}
