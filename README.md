<div align="center">

# assetlas

Continuously updated bug bounty asset inventory — scope + enumeration.

[![Go](https://img.shields.io/badge/go-1.24+-00ADD8.svg?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE.md)
[![Stars](https://img.shields.io/github/stars/pajarori/assetlas?style=flat&logo=github)](https://github.com/pajarori/assetlas/stargazers)
[![Forks](https://img.shields.io/github/forks/pajarori/assetlas?style=flat&logo=github)](https://github.com/pajarori/assetlas/network/members)
[![Issues](https://img.shields.io/github/issues/pajarori/assetlas?style=flat&logo=github)](https://github.com/pajarori/assetlas/issues)
[![Last Commit](https://img.shields.io/github/last-commit/pajarori/assetlas?style=flat&logo=github)](https://github.com/pajarori/assetlas/commits/main)

</div>

## Installation

```bash
go install github.com/pajarori/assetlas@main
```

or download from [releases](https://github.com/pajarori/assetlas/releases).

Optional enumeration tools (subfinder, httpx, naabu, dnsx, tlsx):

```bash
make tools
```

## Usage

```bash
# Scrape all platforms into results/
assetlas scrape

# Scrape one platform
assetlas scrape --platform yeswehack

# Run asset enumeration on one program
assetlas enum --handle security --platform hackerone

# Full enum pipeline with ports, DNS, TLS
assetlas enum --ports --dns --tls

# Quick status of scraped data
assetlas status

# Regenerate this README
assetlas readme
```

## Options

| Flag | Description |
|------|-------------|
| `-p, --platform` | Limit scrape to specific platforms (repeatable) |
| `--handle` | Limit enum to one program handle |
| `--ports` | Enable naabu port scan |
| `--dns` | Enable dnsx DNS records lookup |
| `--tls` | Enable tlsx TLS info collection |
| `--concurrency` | Programs processed in parallel (default 4) |
| `--threads` | Threads passed to enum tools (default 50) |
| `--limit` | Process at most N programs (0 = all) |
| `--silent` | Suppress info logs |
| `--verbose` | Debug logs with per-request timing |
| `--config` | Path to YAML config |

## Data

| Platform | Programs | File |
|----------|---------:|------|
| HackerOne | 0 | [`results/platforms/hackerone.json`](results/platforms/hackerone.json) |
| Bugcrowd | 260 | [`results/platforms/bugcrowd.json`](results/platforms/bugcrowd.json) |
| Intigriti | 139 | [`results/platforms/intigriti.json`](results/platforms/intigriti.json) |
| YesWeHack | 63 | [`results/platforms/yeswehack.json`](results/platforms/yeswehack.json) |
| HackenProof | 311 | [`results/platforms/hackenproof.json`](results/platforms/hackenproof.json) |
| **Total** | **773** | |

### Aggregates

- [`results/aggregates/domains.txt`](results/aggregates/domains.txt) — eligible plain URLs
- [`results/aggregates/wildcards.txt`](results/aggregates/wildcards.txt) — eligible wildcards
- [`results/aggregates/api.txt`](results/aggregates/api.txt) — eligible API endpoints
- [`results/aggregates/android.txt`](results/aggregates/android.txt) — Android assets
- [`results/aggregates/ios.txt`](results/aggregates/ios.txt) — iOS assets

### Per-program enumeration

Enum output lands under `results/enum/<platform>/<handle>/`:

- `hostnames.txt` — subdomains discovered by subfinder
- `alive.jsonl` — httpx output (status, title, tech, IP, ASN)
- `ports.jsonl` — naabu open ports (with `--ports`)
- `dns.jsonl` — dnsx records (with `--dns`)
- `tls.jsonl` — tlsx certificate info (with `--tls`)
- `meta.json` — program metadata + first-seen / last-seen timestamps

## Credits & References

- [arkadiyt/bounty-targets-data](https://github.com/arkadiyt/bounty-targets-data) — pioneering bug bounty scope dataset
- [trickest/inventory](https://github.com/trickest/inventory) — continuous enumeration inspiration
- [trickest/resolvers](https://github.com/trickest/resolvers) — curated DNS resolver list
- [ProjectDiscovery](https://github.com/projectdiscovery) — subfinder, httpx, naabu, dnsx, tlsx

## License

MIT License
