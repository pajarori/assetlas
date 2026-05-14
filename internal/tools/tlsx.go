package tools

import (
	"context"
	"strconv"
)

type TlsxResult struct {
	Host            string   `json:"host"`
	IP              string   `json:"ip"`
	Port            string   `json:"port"`
	TLSVersion      string   `json:"tls_version"`
	Cipher          string   `json:"cipher"`
	NotBefore       string   `json:"not_before"`
	NotAfter        string   `json:"not_after"`
	SubjectCN       string   `json:"subject_cn"`
	SubjectAN       []string `json:"subject_an"`
	IssuerCN        string   `json:"issuer_cn"`
	IssuerOrg       []string `json:"issuer_org"`
	FingerprintHash struct {
		MD5    string `json:"md5"`
		SHA1   string `json:"sha1"`
		SHA256 string `json:"sha256"`
	} `json:"fingerprint_hash"`
	SNI string `json:"sni"`
}

type TlsxOptions struct {
	Hosts   []string
	Threads int
	Timeout int
}

func RunTlsx(ctx context.Context, opts TlsxOptions, emit func(TlsxResult) error) error {
	if len(opts.Hosts) == 0 {
		return nil
	}
	args := []string{"-silent", "-nc", "-json", "-tls-version", "-cipher", "-hash", "sha256"}
	if opts.Threads > 0 {
		args = append(args, "-c", strconv.Itoa(opts.Threads))
	}
	if opts.Timeout > 0 {
		args = append(args, "-timeout", strconv.Itoa(opts.Timeout))
	}
	return RunJSONL(ctx, StreamOptions{Binary: "tlsx", Args: args, Stdin: opts.Hosts}, emit)
}
