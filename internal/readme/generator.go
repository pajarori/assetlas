package readme

import (
	"fmt"
	"strings"

	"github.com/pajarori/assetlas/internal/platform"
	"github.com/pajarori/assetlas/internal/store"
)

func Generate(path string, all map[string][]platform.Program, _ string) error {
	var b strings.Builder

	b.WriteString(`<div align="center">

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

`)
	b.WriteString("```bash\n")
	b.WriteString("go install github.com/pajarori/assetlas@main\n")
	b.WriteString("```\n\n")
	b.WriteString("or download from [releases](https://github.com/pajarori/assetlas/releases).\n\n")

	b.WriteString("Optional enumeration tools (subfinder, httpx, naabu, dnsx, tlsx):\n\n")
	b.WriteString("```bash\n")
	b.WriteString("make tools\n")
	b.WriteString("```\n\n")

	b.WriteString("## Usage\n\n")
	b.WriteString("```bash\n")
	b.WriteString("# Scrape all platforms into results/\n")
	b.WriteString("assetlas scrape\n\n")
	b.WriteString("# Scrape one platform\n")
	b.WriteString("assetlas scrape --platform yeswehack\n\n")
	b.WriteString("# Run asset enumeration on one program\n")
	b.WriteString("assetlas enum --handle security --platform hackerone\n\n")
	b.WriteString("# Full enum pipeline with ports, DNS, TLS\n")
	b.WriteString("assetlas enum --ports --dns --tls\n\n")
	b.WriteString("# Quick status of scraped data\n")
	b.WriteString("assetlas status\n\n")
	b.WriteString("# Regenerate this README\n")
	b.WriteString("assetlas readme\n")
	b.WriteString("```\n\n")

	b.WriteString("## Options\n\n")
	b.WriteString("| Flag | Description |\n")
	b.WriteString("|------|-------------|\n")
	b.WriteString("| `-p, --platform` | Limit scrape to specific platforms (repeatable) |\n")
	b.WriteString("| `--handle` | Limit enum to one program handle |\n")
	b.WriteString("| `--ports` | Enable naabu port scan |\n")
	b.WriteString("| `--dns` | Enable dnsx DNS records lookup |\n")
	b.WriteString("| `--tls` | Enable tlsx TLS info collection |\n")
	b.WriteString("| `--concurrency` | Programs processed in parallel (default 4) |\n")
	b.WriteString("| `--threads` | Threads passed to enum tools (default 50) |\n")
	b.WriteString("| `--limit` | Process at most N programs (0 = all) |\n")
	b.WriteString("| `--silent` | Suppress info logs |\n")
	b.WriteString("| `--verbose` | Debug logs with per-request timing |\n")
	b.WriteString("| `--config` | Path to YAML config |\n\n")

	b.WriteString("## Data\n\n")

	total := 0
	b.WriteString("| Platform | Programs | File |\n")
	b.WriteString("|----------|---------:|------|\n")
	for _, p := range platform.AllPlatforms {
		count := len(all[p.Key])
		total += count
		b.WriteString(fmt.Sprintf("| %s | %d | [`results/platforms/%s.json`](results/platforms/%s.json) |\n", p.Name, count, p.Key, p.Key))
	}
	b.WriteString(fmt.Sprintf("| **Total** | **%d** | |\n\n", total))

	b.WriteString("### Aggregates\n\n")
	b.WriteString("- [`results/aggregates/domains.txt`](results/aggregates/domains.txt) — eligible plain URLs\n")
	b.WriteString("- [`results/aggregates/wildcards.txt`](results/aggregates/wildcards.txt) — eligible wildcards\n")
	b.WriteString("- [`results/aggregates/api.txt`](results/aggregates/api.txt) — eligible API endpoints\n")
	b.WriteString("- [`results/aggregates/android.txt`](results/aggregates/android.txt) — Android assets\n")
	b.WriteString("- [`results/aggregates/ios.txt`](results/aggregates/ios.txt) — iOS assets\n\n")

	b.WriteString("### Per-program enumeration\n\n")
	b.WriteString("Enum output lands under `results/enum/<platform>/<handle>/`:\n\n")
	b.WriteString("- `hostnames.txt` — subdomains discovered by subfinder\n")
	b.WriteString("- `alive.jsonl` — httpx output (status, title, tech, IP, ASN)\n")
	b.WriteString("- `ports.jsonl` — naabu open ports (with `--ports`)\n")
	b.WriteString("- `dns.jsonl` — dnsx records (with `--dns`)\n")
	b.WriteString("- `tls.jsonl` — tlsx certificate info (with `--tls`)\n")
	b.WriteString("- `meta.json` — program metadata + first-seen / last-seen timestamps\n\n")

	b.WriteString("## Credits & References\n\n")
	b.WriteString("- [arkadiyt/bounty-targets-data](https://github.com/arkadiyt/bounty-targets-data) — pioneering bug bounty scope dataset\n")
	b.WriteString("- [trickest/inventory](https://github.com/trickest/inventory) — continuous enumeration inspiration\n")
	b.WriteString("- [trickest/resolvers](https://github.com/trickest/resolvers) — curated DNS resolver list\n")
	b.WriteString("- [ProjectDiscovery](https://github.com/projectdiscovery) — subfinder, httpx, naabu, dnsx, tlsx\n\n")

	b.WriteString("## License\n\nMIT License\n")
	return store.WriteIfChanged(path, []byte(b.String()))
}
