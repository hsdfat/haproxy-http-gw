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

package overload

import (
	"reflect"
	"testing"
)

func TestStoreUpsertGetListDelete(t *testing.T) {
	s := NewStore()
	s.Upsert(Rule{FrontendName: "fe1", Path: "/a", Limit: 10})
	s.Upsert(Rule{FrontendName: "fe1", Path: "/b", Limit: 20})
	s.Upsert(Rule{FrontendName: "fe2", Path: "/a", Limit: 30})

	// Get
	r, ok := s.Get("fe1", "/a")
	if !ok || r.Limit != 10 {
		t.Fatalf("expected fe1:/a limit=10, got ok=%v r=%+v", ok, r)
	}

	// List is sorted
	list := s.List("fe1")
	if len(list) != 2 || list[0].Path != "/a" || list[1].Path != "/b" {
		t.Fatalf("unexpected list: %+v", list)
	}

	// Update preserves CreatedAt
	first := r.CreatedAt
	s.Upsert(Rule{FrontendName: "fe1", Path: "/a", Limit: 11})
	r2, _ := s.Get("fe1", "/a")
	if !r2.CreatedAt.Equal(first) {
		t.Fatalf("CreatedAt should be preserved on update, got %v vs %v", r2.CreatedAt, first)
	}
	if r2.Limit != 11 {
		t.Fatalf("update did not change limit: %+v", r2)
	}

	// Delete
	if !s.Delete("fe1", "/a") {
		t.Fatalf("delete should return true")
	}
	if s.Delete("fe1", "/a") {
		t.Fatalf("double-delete should return false")
	}
}

func TestRenderMapLinesLongestFirst(t *testing.T) {
	rules := []Rule{
		{Path: "/a", Limit: 1},
		{Path: "/api/v1", Limit: 2},
		{Path: "/api", Limit: 3},
	}
	lines := renderMapLines(rules)
	want := []string{"/api/v1 2", "/api 3", "/a 1"}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("unexpected order: %v want %v", lines, want)
	}
}

func TestPeriodToMillis(t *testing.T) {
	cases := map[string]int64{
		"500ms": 500,
		"10s":   10_000,
		"1m":    60_000,
		"2h":    2 * 3600 * 1000,
	}
	for in, want := range cases {
		got, err := periodToMillis(in)
		if err != nil {
			t.Errorf("%s: unexpected error %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("%s: got %d want %d", in, got, want)
		}
	}
	if _, err := periodToMillis("bad"); err == nil {
		t.Errorf("expected error for %q", "bad")
	}
	if _, err := periodToMillis("10x"); err == nil {
		t.Errorf("expected error for %q", "10x")
	}
}

func TestParseStickTable(t *testing.T) {
	raw := "# table: fe_overload_tbl, type: string, size:100000, used:2\n" +
		"0x7f: key=/api/orders use=0 exp=9876 http_req_rate(10000)=42\n" +
		"0x80: key=/other use=0 exp=100 http_req_rate(10000)=1\n"
	lines := parseStickTable(raw)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0].Path != "/api/orders" || lines[0].HTTPReqRate != 42 || lines[0].Expire != 9876 {
		t.Fatalf("unexpected first line: %+v", lines[0])
	}
}

func TestSanitize(t *testing.T) {
	if got := sanitize("frontend-api"); got != "frontend_api" {
		t.Fatalf("sanitize: got %q", got)
	}
	if got := sanitize("a.b/c"); got != "a_b_c" {
		t.Fatalf("sanitize: got %q", got)
	}
}
