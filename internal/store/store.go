package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/pajarori/assetlas/internal/platform"
)

const (
	platformsSubdir  = "platforms"
	aggregatesSubdir = "aggregates"
	enumSubdir       = "enum"
	indexFile        = "index.json"
)

type Store struct {
	resultsDir string
}

type Index struct {
	Counts      map[string]int `json:"counts"`
	Scopes      map[string]int `json:"scopes"`
	Total       int            `json:"total"`
	TotalScopes int            `json:"total_scopes"`
}

func New(resultsDir string) *Store {
	return &Store{resultsDir: resultsDir}
}

func (s *Store) PlatformsDir() string  { return filepath.Join(s.resultsDir, platformsSubdir) }
func (s *Store) AggregatesDir() string { return filepath.Join(s.resultsDir, aggregatesSubdir) }
func (s *Store) EnumDir() string       { return filepath.Join(s.resultsDir, enumSubdir) }
func (s *Store) IndexPath() string     { return filepath.Join(s.resultsDir, indexFile) }
func (s *Store) ProgramsPath(plat string) string {
	return filepath.Join(s.PlatformsDir(), plat+".json")
}

func (s *Store) WritePrograms(plat string, programs []platform.Program) error {
	if err := os.MkdirAll(s.PlatformsDir(), 0o755); err != nil {
		return fmt.Errorf("mkdir platforms dir: %w", err)
	}
	if programs == nil {
		programs = []platform.Program{}
	}
	platform.SortPrograms(programs)
	return writeJSON(s.ProgramsPath(plat), programs)
}

func (s *Store) ReadPrograms(plat string) ([]platform.Program, error) {
	path := s.ProgramsPath(plat)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var programs []platform.Program
	if err := json.Unmarshal(data, &programs); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return programs, nil
}

func (s *Store) WriteAggregates(all map[string][]platform.Program) error {
	if err := os.MkdirAll(s.AggregatesDir(), 0o755); err != nil {
		return fmt.Errorf("mkdir aggregates dir: %w", err)
	}
	domains := map[string]struct{}{}
	wildcards := map[string]struct{}{}
	apis := map[string]struct{}{}
	mobileAndroid := map[string]struct{}{}
	mobileIOS := map[string]struct{}{}

	for _, programs := range all {
		for _, p := range programs {
			for _, sc := range p.Scopes {
				if !sc.Eligible {
					continue
				}
				ident := platform.NormalizeWildcard(sc.Identifier)
				if ident == "" {
					continue
				}
				switch sc.Type {
				case platform.AssetURL:
					host := platform.StripURLPath(ident)
					if host != "" {
						domains[host] = struct{}{}
					}
				case platform.AssetWildcard:
					wildcards[ident] = struct{}{}
				case platform.AssetAPI:
					apis[ident] = struct{}{}
				case platform.AssetAndroid:
					mobileAndroid[ident] = struct{}{}
				case platform.AssetIOS:
					mobileIOS[ident] = struct{}{}
				}
			}
		}
	}

	pairs := []struct {
		name string
		set  map[string]struct{}
	}{
		{"domains.txt", domains},
		{"wildcards.txt", wildcards},
		{"api.txt", apis},
		{"android.txt", mobileAndroid},
		{"ios.txt", mobileIOS},
	}
	for _, p := range pairs {
		if err := writeSortedLines(filepath.Join(s.AggregatesDir(), p.name), p.set); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) WriteIndex(all map[string][]platform.Program) error {
	if err := os.MkdirAll(s.resultsDir, 0o755); err != nil {
		return fmt.Errorf("mkdir results dir: %w", err)
	}
	idx := Index{Counts: map[string]int{}, Scopes: map[string]int{}}
	for _, p := range platform.AllPlatforms {
		progs, ok := all[p.Key]
		if !ok {
			progs, _ = s.ReadPrograms(p.Key)
		}
		idx.Counts[p.Key] = len(progs)
		idx.Total += len(progs)
		scopeCount := 0
		for _, prog := range progs {
			scopeCount += len(prog.Scopes)
		}
		idx.Scopes[p.Key] = scopeCount
		idx.TotalScopes += scopeCount
	}
	return writeJSON(s.IndexPath(), idx)
}

func (s *Store) ReadIndex() (*Index, error) {
	data, err := os.ReadFile(s.IndexPath())
	if err != nil {
		return nil, err
	}
	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse %s: %w", s.IndexPath(), err)
	}
	return &idx, nil
}

func writeJSON(path string, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return WriteIfChanged(path, buf.Bytes())
}

func writeSortedLines(path string, set map[string]struct{}) error {
	lines := make([]string, 0, len(set))
	for k := range set {
		lines = append(lines, k)
	}
	sort.Strings(lines)
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	for _, l := range lines {
		w.WriteString(l)
		w.WriteByte('\n')
	}
	w.Flush()
	return WriteIfChanged(path, buf.Bytes())
}

func WriteIfChanged(path string, content []byte) error {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, content) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	return os.Rename(tmp, path)
}
