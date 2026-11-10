package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Exercise actual stdio framing, discovery, structured output, tool errors,
// package persistence and readback using an SDK client and the built binary.
func TestStdioEndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "prism-mcp")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/prism-mcp")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "prism-test", Version: "1.0.0"}, nil)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/projects" {
			if r.Method == "GET" {
				fmt.Fprint(w, `<form id="projects-search-page-form"><input type="hidden" name="form_id" value="projects_search_page_form"><input type="hidden" name="form_build_id" value="fixture"></form>`)
			} else {
				fmt.Fprint(w, `<div class="row shadow"><p># P50500000005</p><h4>Fixture Project</h4><p>Fixture Promoter</p><div>State</div><p>MAHARASHTRA</p><div>District</div><p>Nagpur</p><div>Pincode</div><p>441108</p></div>`)
			}
		} else if r.URL.Path == "/search" {
			fmt.Fprint(w, `{"results":[{"id":1,"name":"Pune","country_code":"IN","admin1":"Maharashtra","latitude":18.52,"longitude":73.85}]}`)
		} else {
			fmt.Fprint(w, `{"version":0.6,"osm3s":{"timestamp_osm_base":"2026-11-07T00:00:00Z"},"elements":[]}`)
		}
	}))
	defer source.Close()
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.CommandContext(ctx, binary, "-data-dir", t.TempDir(), "-geocoding-url", source.URL+"/search", "-overpass-url", source.URL+"/overpass", "-maharera-url", source.URL+"/projects")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 14 {
		t.Fatalf("got %d tools", len(tools.Tools))
	}
	call := func(name string, args any) map[string]any {
		t.Helper()
		out, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		if out.IsError {
			t.Fatalf("%s failed: %+v", name, out.Content)
		}
		b, err := json.Marshal(out.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var result map[string]any
		if err = json.Unmarshal(b, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	out := call("get_market_coverage", map[string]any{})
	if len(out["entries"].([]any)) != 36 {
		t.Fatal("national coverage missing")
	}
	call("resolve_property", map[string]any{"state": "RJ", "property_type": "plot", "address": "Example Village"})
	q := map[string]any{"state": "KA", "property_type": "office"}
	call("search_property_evidence", map[string]any{"query": q})
	pkg := call("build_evidence_package", map[string]any{"query": q})
	call("get_evidence", map[string]any{"id": pkg["id"]})
	geo := call("geocode_location", map[string]any{"place": "Pune", "state": "MH"})
	if len(geo["candidates"].([]any)) != 1 {
		t.Fatal("geocoding tool failed")
	}
	loc := map[string]any{"latitude": 18.52, "longitude": 73.85}
	call("get_location_context", loc)
	a := call("build_evidence_package", map[string]any{"query": q, "location": loc})
	b := call("build_evidence_package", map[string]any{"query": q, "location": loc})
	if a["id"] != b["id"] {
		t.Fatal("live cached context changes snapshot identity")
	}
	registry := call("search_registered_projects", map[string]any{"state": "MH", "registration_number": "P50500000005"})
	if registry["status"] != "found" {
		t.Fatal("registry tool failed")
	}
	call("build_evidence_package", map[string]any{"query": map[string]any{"state": "MH", "property_type": "apartment"}, "project_registration_number": "P50500000005"})
	bad, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "resolve_property", Arguments: map[string]any{"state": "Atlantis", "address": "x", "property_type": "plot"}})
	if err == nil && !bad.IsError {
		t.Fatal("unknown jurisdiction accepted")
	}
}
