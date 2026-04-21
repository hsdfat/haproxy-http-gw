// Copyright 2019 HAProxy Technologies LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package overload implements per-path overload protection for a frontend.
//
// Design
//
//   - A single stick-table backend is created per frontend, keyed by request path.
//   - A map file on disk holds "<path_prefix> <limit>" entries; HAProxy's map_beg
//     converter looks up the limit at request time.
//   - Four http-request rules are installed at the top of the frontend:
//     (1) set-var(txn.ol_path) path
//     (2) set-var(txn.ol_limit) var(txn.ol_path),map_beg(<file>)  -- no default, so
//         var stays unset on a miss (paths with no rule)
//     (3) track-sc0 var(txn.ol_path) table <tbl> if { var(txn.ol_limit) -m found }
//     (4) deny if { var(txn.ol_limit) -m found } and
//                 { sc_http_req_rate(0),sub(txn.ol_limit) -m int gt 0 }
//
// Rules are "global-per-path": all clients hitting the same path share one counter.
// Adding/updating/removing a limit is a map update — no HAProxy reload required
// after the one-time bootstrap.
package overload

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/haproxytech/client-native/v6/models"
	"github.com/haproxytech/kubernetes-ingress/pkg/haproxy/api"
	"github.com/haproxytech/kubernetes-ingress/pkg/utils"
)

var logger = utils.GetLogger()

const (
	DefaultPeriod     = "10s"
	DefaultTableSize  = int64(100000)
	DefaultKeyLen     = int64(256)
	DefaultDenyStatus = int64(429)
	DefaultMapsDir    = "/etc/haproxy/maps"
)

// Rule is a per-path overload limit on a frontend.
type Rule struct {
	FrontendName string    `json:"frontend_name"`
	Path         string    `json:"path"`       // path prefix (match: path_beg)
	Limit        int64     `json:"limit"`      // requests allowed per frontend Period
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Options governs the per-frontend bootstrap.
type Options struct {
	Period     string // "10s", "1m", ... — shared for the frontend's stick-table
	TableSize  int64
	KeyLen     int64
	DenyStatus int64
	MapsDir    string
}

func (o *Options) applyDefaults() {
	if o.Period == "" {
		o.Period = DefaultPeriod
	}
	if o.TableSize == 0 {
		o.TableSize = DefaultTableSize
	}
	if o.KeyLen == 0 {
		o.KeyLen = DefaultKeyLen
	}
	if o.DenyStatus == 0 {
		o.DenyStatus = DefaultDenyStatus
	}
	if o.MapsDir == "" {
		o.MapsDir = DefaultMapsDir
	}
}

// ---------------- Store ----------------

// Store holds overload rules per frontend in-memory.
type Store struct {
	mu    sync.RWMutex
	rules map[string]map[string]Rule // frontendName -> path -> rule
}

func NewStore() *Store {
	return &Store{rules: make(map[string]map[string]Rule)}
}

// Upsert inserts or updates a rule.
func (s *Store) Upsert(r Rule) Rule {
	s.mu.Lock()
	defer s.mu.Unlock()

	fe, ok := s.rules[r.FrontendName]
	if !ok {
		fe = make(map[string]Rule)
		s.rules[r.FrontendName] = fe
	}

	now := time.Now()
	if existing, exists := fe[r.Path]; exists {
		r.CreatedAt = existing.CreatedAt
	} else {
		r.CreatedAt = now
	}
	r.UpdatedAt = now

	fe[r.Path] = r
	return r
}

// Get returns the rule for (frontend, path) if present.
func (s *Store) Get(frontendName, path string) (Rule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fe, ok := s.rules[frontendName]
	if !ok {
		return Rule{}, false
	}
	r, ok := fe[path]
	return r, ok
}

// List returns all rules for a frontend, sorted by path.
func (s *Store) List(frontendName string) []Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fe, ok := s.rules[frontendName]
	if !ok {
		return nil
	}
	out := make([]Rule, 0, len(fe))
	for _, r := range fe {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Delete removes a rule. Returns true if it existed.
func (s *Store) Delete(frontendName, path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	fe, ok := s.rules[frontendName]
	if !ok {
		return false
	}
	if _, exists := fe[path]; !exists {
		return false
	}
	delete(fe, path)
	return true
}

// ---------------- HAProxy integration ----------------

// TableName returns the HAProxy backend name that holds the stick-table for this frontend.
func TableName(frontendName string) string {
	return fmt.Sprintf("%s_overload_tbl", sanitize(frontendName))
}

// MapBasename is the identifier used with the runtime socket (no .map suffix).
func MapBasename(frontendName string) string {
	return fmt.Sprintf("%s_overload", sanitize(frontendName))
}

// MapFilePath returns the absolute path of the map file on disk.
func MapFilePath(opts Options, frontendName string) string {
	opts.applyDefaults()
	return filepath.Join(opts.MapsDir, MapBasename(frontendName)+".map")
}

// Bootstrap idempotently installs the overload table, map file, and http-request rules
// for the frontend. The caller does NOT need to wrap this in a transaction — Bootstrap
// starts and commits its own transaction. First bootstrap triggers an HAProxy reload;
// re-running on an already-bootstrapped frontend is a no-op.
func Bootstrap(client api.HAProxyClient, frontendName string, opts Options) error {
	opts.applyDefaults()

	mapPath := MapFilePath(opts, frontendName)
	if err := ensureEmptyMapFile(mapPath); err != nil {
		return fmt.Errorf("ensure map file: %w", err)
	}

	// Idempotency: if the stick-table backend already exists, bootstrap has run.
	tblName := TableName(frontendName)
	if _, err := client.BackendGet(tblName); err == nil {
		logger.Debugf("[OVERLOAD] frontend %s already bootstrapped, skipping", frontendName)
		return nil
	}

	if err := client.APIStartTransaction(); err != nil {
		return fmt.Errorf("start transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			client.APIDisposeTransaction()
		}
	}()

	// 1) Stick-table backend.
	period := opts.Period
	expireMS, err := periodToMillis(period)
	if err != nil {
		return fmt.Errorf("invalid period %q: %w", period, err)
	}
	tbl := models.Backend{
		BackendBase: models.BackendBase{
			Name: tblName,
			StickTable: &models.ConfigStickTable{
				Type:   "string",
				Size:   utils.PtrInt64(opts.TableSize),
				Keylen: utils.PtrInt64(opts.KeyLen),
				Expire: utils.PtrInt64(expireMS),
				Store:  fmt.Sprintf("http_req_rate(%s)", period),
			},
		},
	}
	client.BackendCreatePermanently(tbl)

	// 2) http-request rules. Insert at id=0 in reverse so final order is:
	//      set-var(txn.ol_path) path
	//      set-var(txn.ol_limit) var(txn.ol_path),map_beg(<mapPath>)
	//      track-sc0 var(txn.ol_path) table <tbl> if { var(txn.ol_limit) -m found }
	//      deny deny_status <status>
	//           if { var(txn.ol_limit) -m found }
	//              { sc_http_req_rate(0),sub(txn.ol_limit) -m int gt 0 }
	//
	// Notes:
	//   - map_beg without a default makes the sample chain fail on a miss, so
	//     set-var never assigns ol_limit. "-m found" is the cleanest way to
	//     gate on "a rule matched" — no negative sentinels to confuse the ACL
	//     integer parser.
	//   - HAProxy ACL ops (gt/lt/eq) only take literals, so we compute
	//     rate - limit via the sub() converter (which accepts a scoped var
	//     name) and compare to 0.
	denyRule := models.HTTPRequestRule{
		Type:       "deny",
		DenyStatus: utils.PtrInt64(opts.DenyStatus),
		Cond:       "if",
		CondTest:   "{ var(txn.ol_limit) -m found } { sc_http_req_rate(0),sub(txn.ol_limit) -m int gt 0 }",
	}
	trackRule := models.HTTPRequestRule{
		Type:                "track-sc",
		TrackScStickCounter: utils.PtrInt64(0),
		TrackScKey:          "var(txn.ol_path)",
		TrackScTable:        tblName,
		Cond:                "if",
		CondTest:            "{ var(txn.ol_limit) -m found }",
	}
	setLimit := models.HTTPRequestRule{
		Type:     "set-var",
		VarName:  "ol_limit",
		VarScope: "txn",
		VarExpr:  fmt.Sprintf("var(txn.ol_path),map_beg(%s)", mapPath),
	}
	setPath := models.HTTPRequestRule{
		Type:     "set-var",
		VarName:  "ol_path",
		VarScope: "txn",
		VarExpr:  "path",
	}

	for _, r := range []models.HTTPRequestRule{denyRule, trackRule, setLimit, setPath} {
		if err := client.FrontendHTTPRequestRuleCreate(0, frontendName, r, ""); err != nil {
			return fmt.Errorf("create http-request rule (%s): %w", r.Type, err)
		}
	}

	if err := client.APIFinalCommitTransaction(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	committed = true

	logger.Infof("[OVERLOAD] Bootstrapped frontend %s: table=%s map=%s period=%s",
		frontendName, tblName, mapPath, period)
	return nil
}

// Sync rewrites the frontend's map file on disk to match `rules` and pushes the
// new content through the runtime socket. It is safe to call concurrently with
// request traffic; the runtime update is atomic.
func Sync(client api.HAProxyClient, frontendName string, opts Options, rules []Rule) error {
	opts.applyDefaults()

	mapPath := MapFilePath(opts, frontendName)
	lines := renderMapLines(rules)

	// 1) Write the file on disk so the state survives a reload.
	if err := writeMapFile(mapPath, lines); err != nil {
		return fmt.Errorf("write map file: %w", err)
	}

	time.Sleep(100 * time.Millisecond)

	// 2) Push to the runtime socket so the change is live without a reload.
	//    SetMapContent wants chunks; one chunk is fine for our expected sizes.
	payload := []string{strings.Join(lines, "\n")}
	if err := client.SetMapContent(MapBasename(frontendName), payload); err != nil {
		return fmt.Errorf("runtime map update: %w", err)
	}
	return nil
}

// StatsLine holds one row of rate data from `show table`.
type StatsLine struct {
	Path        string `json:"path"`
	HTTPReqRate int64  `json:"http_req_rate"`
	Expire      int64  `json:"expire_ms"`
}

// Stats queries the frontend's stick-table via the runtime socket and returns
// current rate per tracked path.
func Stats(client api.HAProxyClient, frontendName string) ([]StatsLine, error) {
	raw, err := client.ExecuteRaw(fmt.Sprintf("show table %s", TableName(frontendName)))
	if err != nil {
		return nil, fmt.Errorf("show table: %w", err)
	}
	return parseStickTable(raw), nil
}

// ---------------- helpers ----------------

func sanitize(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
			b.WriteRune(c)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func renderMapLines(rules []Rule) []string {
	if len(rules) == 0 {
		return nil
	}
	// Sort by path length desc so map_beg picks the most specific prefix first.
	sorted := make([]Rule, len(rules))
	copy(sorted, rules)
	sort.Slice(sorted, func(i, j int) bool {
		if len(sorted[i].Path) != len(sorted[j].Path) {
			return len(sorted[i].Path) > len(sorted[j].Path)
		}
		return sorted[i].Path < sorted[j].Path
	})
	out := make([]string, 0, len(sorted))
	for _, r := range sorted {
		out = append(out, fmt.Sprintf("%s %d", r.Path, r.Limit))
	}
	return out
}

func ensureEmptyMapFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

func writeMapFile(path string, lines []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if len(lines) > 0 {
		if _, err := f.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// periodToMillis converts HAProxy time strings ("10s", "500ms", "1m") to milliseconds.
func periodToMillis(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty period")
	}
	// Split numeric prefix and unit suffix.
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9') {
		i++
	}
	if i == 0 {
		return 0, fmt.Errorf("missing number in %q", s)
	}
	numStr, unit := s[:i], s[i:]
	var mult int64
	switch unit {
	case "", "ms":
		mult = 1
	case "s":
		mult = 1000
	case "m":
		mult = 60 * 1000
	case "h":
		mult = 3600 * 1000
	case "d":
		mult = 24 * 3600 * 1000
	default:
		return 0, fmt.Errorf("unknown unit %q", unit)
	}
	var n int64
	for _, c := range numStr {
		n = n*10 + int64(c-'0')
	}
	return n * mult, nil
}

// parseStickTable parses the output of `show table <name>`. Sample line:
//   0x55...: key=/api/orders use=0 exp=9876 http_req_rate(10000)=42
func parseStickTable(raw string) []StatsLine {
	var out []StatsLine
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var key string
		var rate, exp int64
		for _, tok := range strings.Fields(line) {
			switch {
			case strings.HasPrefix(tok, "key="):
				key = strings.TrimPrefix(tok, "key=")
			case strings.HasPrefix(tok, "exp="):
				fmt.Sscanf(tok, "exp=%d", &exp)
			case strings.HasPrefix(tok, "http_req_rate("):
				// http_req_rate(10000)=42
				if eq := strings.IndexByte(tok, '='); eq > 0 {
					fmt.Sscanf(tok[eq+1:], "%d", &rate)
				}
			}
		}
		if key != "" {
			out = append(out, StatsLine{Path: key, HTTPReqRate: rate, Expire: exp})
		}
	}
	return out
}
