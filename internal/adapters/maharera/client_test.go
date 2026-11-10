package maharera

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"prism/internal/storage"
)

const form = `<form id="projects-search-page-form"><input type="hidden" name="form_build_id" value="fixture"><input type="hidden" name="form_id" value="projects_search_page_form"></form>`
const card = `<div class="card"><div><p># P50500000005</p><h4><strong>Fixture Project</strong></h4><p>Fixture Promoter</p><ul><li>Fixture Rural Location</li></ul></div><div><span>State</span><span>MAHARASHTRA</span></div><div><span>Pincode</span><span>441108</span></div><div><span>District</span><span>Nagpur</span></div><div><span>Last Modified</span><span>2017-05-20</span></div></div>`

func TestFormLookupCacheAndSourceContract(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method == "GET" {
			fmt.Fprint(w, form)
			return
		}
		r.ParseForm()
		if r.Form.Get("project_name") != "P50500000005" || r.Form.Get("form_build_id") != "fixture" {
			t.Error("wrong exact-registration submission")
		}
		fmt.Fprint(w, card)
	}))
	defer server.Close()
	c, err := New(storage.Store{Root: t.TempDir()}, server.URL, "PRISM-test")
	if err != nil {
		t.Fatal(err)
	}
	in := Input{State: "MH", RegistrationNumber: "P50500000005"}
	a, err := c.Search(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != "found" || len(a.Projects) != 1 || a.Projects[0].District != "Nagpur" || len(a.SourceSnapshotIDs) != 2 {
		t.Fatal("wrong registry result", a)
	}
	b, err := c.Search(context.Background(), in)
	if err != nil || !b.CacheHit || calls != 2 {
		t.Fatal("registry cache failed", err)
	}
	in.State = "KA"
	if _, err = c.Search(context.Background(), in); err == nil {
		t.Fatal("invented other-state registry coverage")
	}
	in.State = "MH"
	in.AsOf = "2020-01-01"
	if _, err = c.Search(context.Background(), in); err == nil {
		t.Fatal("historical query accepted current source")
	}
}

func TestParseFailsClosed(t *testing.T) {
	if _, err := parseProjects(card, "P52100012200"); err == nil {
		t.Fatal("accepted unrelated registration")
	}
	if _, err := parseProjects("<h1>Service unavailable</h1>", "P50500000005"); err == nil {
		t.Fatal("outage became no records")
	}
	if out, err := parseProjects("<div>No Records Found</div>", "P50500000005"); err != nil || len(out) != 0 {
		t.Fatal("explicit no records not recognized")
	}
	sibling := strings.ReplaceAll(card, "P50500000005", "P50500000099")
	target := `<div class="card"><p># P50500000006</p><h4>Incomplete target</h4></div>`
	if _, err := parseProjects("<main>"+sibling+target+"</main>", "P50500000006"); err == nil {
		t.Fatal("borrowed sibling fields for incomplete target")
	}
	if _, err := parseProjects("<main>"+target+sibling+"</main>", "P50500000006"); err == nil {
		t.Fatal("borrowed following sibling fields")
	}
	if _, err := parseProjects(strings.ReplaceAll(card, "441108", "foobar"), "P50500000005"); err == nil {
		t.Fatal("accepted nonnumeric pincode")
	}
}

func TestLiveMahaRERA(t *testing.T) {
	if os.Getenv("PRISM_LIVE_REGISTRY_TEST") != "1" {
		t.Skip("set PRISM_LIVE_REGISTRY_TEST=1 for one live exact-registration lookup")
	}
	root := os.Getenv("PRISM_LIVE_DATA_DIR")
	if root == "" {
		root = t.TempDir()
	}
	c, err := New(storage.Store{Root: root}, Endpoint, "PRISM-PropertyIntelligence/0.2.0 (educational prototype)")
	if err != nil {
		t.Fatal(err)
	}
	id := os.Getenv("PRISM_REGISTRATION_NUMBER")
	if id == "" {
		id = "P50500000005"
	}
	out, err := c.Search(context.Background(), Input{State: "Maharashtra", RegistrationNumber: id})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != "found" || len(out.Projects) != 1 {
		t.Fatal("expected known public project", out)
	}
	t.Logf("%s: %s district=%s; source snapshots=%v cache=%v", out.Projects[0].RegistrationNumber, out.Projects[0].Name, out.Projects[0].District, out.SourceSnapshotIDs, out.CacheHit)
}
