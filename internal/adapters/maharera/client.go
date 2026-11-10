// Package maharera reads the public, exact-registration project search form.
// It does not access authenticated applications, solve CAPTCHAs or crawl pages.
package maharera

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
	"prism/internal/coverage"
	"prism/internal/storage"
)

const Endpoint = "https://www.maharera.maharashtra.gov.in/projects-search-result"
const Policy = "https://www.maharera.maharashtra.gov.in/copyright-policy"

var registration = regexp.MustCompile(`^P[0-9]{11}$`)
var pincode = regexp.MustCompile(`^[0-9]{6}$`)

const parserVersion = "maharera_card_v2"

type Client struct {
	store               storage.Store
	http                *http.Client
	endpoint, userAgent string
	gate                chan struct{}
}

func New(store storage.Store, endpoint, userAgent string) (*Client, error) {
	if strings.TrimSpace(userAgent) == "" || strings.ContainsAny(userAgent, "\r\n") {
		return nil, fmt.Errorf("valid application User-Agent required")
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"))) || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("invalid registry endpoint")
	}
	jar, _ := cookiejar.New(nil)
	return &Client{store: store, endpoint: endpoint, userAgent: userAgent, gate: make(chan struct{}, 1), http: &http.Client{Timeout: 30 * time.Second, Jar: jar, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || req.URL.Host != via[0].URL.Host || req.URL.Scheme != via[0].URL.Scheme {
			return fmt.Errorf("registry redirect outside configured origin")
		}
		return nil
	}}}, nil
}

type Input struct {
	State              string `json:"state"`
	RegistrationNumber string `json:"registration_number"`
	AsOf               string `json:"as_of,omitempty"`
}
type Project struct {
	RegistrationNumber string `json:"registration_number"`
	Name               string `json:"name"`
	Promoter           string `json:"promoter,omitempty"`
	State              string `json:"state"`
	District           string `json:"district"`
	Pincode            string `json:"pincode"`
	LastModified       string `json:"last_modified,omitempty"`
	EntityLevel        string `json:"entity_level"`
}
type Result struct {
	Status             string    `json:"status"`
	RegistrationNumber string    `json:"registration_number"`
	Projects           []Project `json:"projects"`
	SourceURL          string    `json:"source_url"`
	PolicyURL          string    `json:"policy_url"`
	Attribution        string    `json:"attribution"`
	RetrievedAt        string    `json:"retrieved_at"`
	SourceSnapshotIDs  []string  `json:"source_snapshot_ids"`
	Warnings           []string  `json:"warnings"`
}
type Output struct {
	Result
	CacheHit bool `json:"cache_hit"`
}
type cache struct {
	ID      string    `json:"id"`
	Expires time.Time `json:"expires"`
}

func (c *Client) Search(ctx context.Context, in Input) (Output, error) {
	j, err := coverage.Resolve(in.State)
	if err != nil {
		return Output{}, err
	}
	if j.Name != "Maharashtra" {
		return Output{}, fmt.Errorf("project registry adapter is not yet integrated for %s", j.Name)
	}
	if !registration.MatchString(in.RegistrationNumber) {
		return Output{}, fmt.Errorf("registration_number must be P followed by 11 digits")
	}
	if in.AsOf != "" {
		return Output{}, fmt.Errorf("historical project registry lookup unsupported")
	}
	ctx, cancel := context.WithTimeout(ctx, 65*time.Second)
	defer cancel()
	select {
	case c.gate <- struct{}{}:
		defer func() { <-c.gate }()
	case <-ctx.Done():
		return Output{}, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	path := filepath.Join(c.store.Root, "source-cache", "maharera_"+storage.Digest([]byte(parserVersion+"\n"+c.endpoint+"\n"+in.RegistrationNumber))+".json")
	var cached cache
	if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &cached) == nil && time.Now().Before(cached.Expires) {
		b, err := c.store.Get(cached.ID)
		if err != nil {
			return Output{}, err
		}
		var result Result
		if json.Unmarshal(b, &result) != nil || result.RegistrationNumber != in.RegistrationNumber || result.SourceURL != c.endpoint {
			return Output{}, fmt.Errorf("invalid registry cache")
		}
		return Output{Result: result, CacheHit: true}, nil
	}
	page, initialID, err := c.page(ctx, http.MethodGet, nil)
	if err != nil {
		return Output{}, err
	}
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return Output{}, err
	}
	form := find(doc, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "form" && attr(n, "id") == "projects-search-page-form"
	})
	if form == nil {
		return Output{}, fmt.Errorf("public registry form unavailable or changed; no authentication/CAPTCHA bypass attempted")
	}
	values := url.Values{"project_type": {"0"}, "project_name": {in.RegistrationNumber}, "project_location": {""}, "project_completion_date": {""}, "project_state": {""}, "project_district": {""}, "op": {"Search"}}
	walk(form, func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "input" && attr(n, "type") == "hidden" {
			name := attr(n, "name")
			if name == "form_build_id" || name == "form_id" || name == "form_token" {
				values.Set(name, attr(n, "value"))
			}
		}
	})
	if values.Get("form_id") != "projects_search_page_form" || values.Get("form_build_id") == "" {
		return Output{}, fmt.Errorf("registry form contract changed")
	}
	// Deliberately one form submission per explicit lookup, no pagination/crawl.
	timer := time.NewTimer(1100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		return Output{}, ctx.Err()
	}
	page, responseID, err := c.page(ctx, http.MethodPost, values)
	if err != nil {
		return Output{}, err
	}
	projects, err := parseProjects(page, in.RegistrationNumber)
	if err != nil {
		return Output{}, err
	}
	result := Result{Status: "no_match", RegistrationNumber: in.RegistrationNumber, Projects: projects, SourceURL: c.endpoint, PolicyURL: Policy, Attribution: "Maharashtra Real Estate Regulatory Authority (MahaRERA), public registered-project search", RetrievedAt: time.Now().UTC().Format(time.RFC3339Nano), SourceSnapshotIDs: []string{initialID, responseID}, Warnings: []string{"Current public project metadata only; not a title check, unit identity, completed sale, asking-price label or verified valuation.", "Last-modified date is a source update date, not a sale date or proof of historical availability.", "Exact registration lookup only; no national bulk registry or historical records are provided."}}
	result.Warnings = append(result.Warnings, "No-match is the portal's response for this request, not proof that a registration does not exist; upstream responses can be inconsistent.")
	if len(projects) > 0 {
		result.Status = "found"
	}
	id, err := c.store.PutPackage(result)
	if err != nil {
		return Output{}, err
	}
	ttl := 24 * time.Hour
	if len(projects) == 0 {
		ttl = 5 * time.Minute
	}
	b, _ := json.Marshal(cache{ID: id, Expires: time.Now().Add(ttl)})
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return Output{}, err
	}
	if err = os.WriteFile(path, b, 0600); err != nil {
		return Output{}, err
	}
	return Output{Result: result}, nil
}

func (c *Client) page(ctx context.Context, method string, values url.Values) (string, string, error) {
	var body io.Reader
	if values != nil {
		body = strings.NewReader(values.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint, body)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html")
	if values != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("MahaRERA unavailable: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", "", fmt.Errorf("MahaRERA HTTP %d; no bypass or automatic retry", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 4*1024*1024+1))
	if err != nil {
		return "", "", err
	}
	if len(b) > 4*1024*1024 {
		return "", "", fmt.Errorf("registry response exceeds 4 MiB")
	}
	id, err := c.store.PutPackage(struct {
		Kind        string `json:"kind"`
		URL         string `json:"url"`
		Method      string `json:"method"`
		RetrievedAt string `json:"retrieved_at"`
		HTML        string `json:"html"`
	}{"registry_source_page", c.endpoint, method, time.Now().UTC().Format(time.RFC3339Nano), string(b)})
	return string(b), id, err
}

func parseProjects(page, wanted string) ([]Project, error) {
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return nil, err
	}
	out := []Project{}
	headings := 0
	walk(doc, func(n *html.Node) {
		if n.Type != html.ElementNode || (n.Data != "h1" && n.Data != "p") || !registration.MatchString(strings.TrimSpace(strings.TrimPrefix(text(n), "#"))) {
			return
		}
		headings++
		if strings.TrimSpace(strings.TrimPrefix(text(n), "#")) != wanted {
			return
		}
		card := n.Parent
		for card != nil && card.Data != "body" && !isResultCard(card) {
			card = card.Parent
		}
		if card == nil || card.Data == "body" {
			return
		}
		// Never climb out of an incomplete result card into a shared container.
		// That could combine this registration with another project's metadata.
		registrations, names := 0, 0
		walk(card, func(child *html.Node) {
			if child.Type != html.ElementNode {
				return
			}
			if (child.Data == "p" || child.Data == "h1") && registration.MatchString(strings.TrimSpace(strings.TrimPrefix(text(child), "#"))) {
				registrations++
			}
			if child.Data == "h4" {
				names++
			}
		})
		if registrations != 1 || names != 1 {
			return
		}
		nameNode := find(card, func(n *html.Node) bool { return n.Type == html.ElementNode && n.Data == "h4" })
		if nameNode == nil {
			return
		}
		parts := tokens(card)
		name := text(nameNode)
		p := Project{RegistrationNumber: wanted, Name: name, State: "Maharashtra", District: fieldValue(card, "District"), Pincode: fieldValue(card, "Pincode"), LastModified: fieldValue(card, "Last Modified"), EntityLevel: "project"}
		for i, t := range parts {
			if t == name && i+1 < len(parts) {
				p.Promoter = parts[i+1]
				break
			}
		}
		if strings.ToUpper(fieldValue(card, "State")) != "MAHARASHTRA" || p.Name == "" || p.District == "" || !pincode.MatchString(p.Pincode) {
			return
		}
		if p.LastModified != "" {
			if _, err := time.Parse(time.DateOnly, p.LastModified); err != nil {
				p.LastModified = ""
			}
		}
		out = append(out, p)
	})
	if len(out) > 1 {
		return nil, fmt.Errorf("ambiguous duplicate registration results")
	}
	if len(out) == 0 {
		if headings > 0 {
			return nil, fmt.Errorf("registry did not return a parseable exact registration match; source schema/filter may have changed")
		}
		if !strings.Contains(strings.Join(tokens(doc), " "), "No Records Found") {
			return nil, fmt.Errorf("registry result contract changed or unavailable")
		}
	}
	return out, nil
}

func isResultCard(n *html.Node) bool {
	classes := strings.Fields(attr(n, "class"))
	return n.Type == html.ElementNode && (contains(classes, "card") || (contains(classes, "row") && contains(classes, "shadow")))
}

func fieldValue(card *html.Node, label string) string {
	n := find(card, func(n *html.Node) bool {
		return n.Type == html.ElementNode && (n.Data == "div" || n.Data == "span" || n.Data == "dt") && text(n) == label
	})
	if n == nil {
		return ""
	}
	for next := n.NextSibling; next != nil; next = next.NextSibling {
		if next.Type != html.ElementNode {
			continue
		}
		if next.Data != "p" && next.Data != "span" && next.Data != "dd" {
			return ""
		}
		return text(next)
	}
	return ""
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func walk(n *html.Node, f func(*html.Node)) {
	f(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, f)
	}
}
func find(n *html.Node, p func(*html.Node) bool) *html.Node {
	if p(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := find(c, p); found != nil {
			return found
		}
	}
	return nil
}
func tokens(n *html.Node) []string {
	out := []string{}
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style") {
			return
		}
		if n.Type == html.TextNode {
			if t := strings.Join(strings.Fields(n.Data), " "); t != "" {
				out = append(out, t)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(n)
	return out
}
func text(n *html.Node) string { return strings.Join(tokens(n), " ") }
func contains(parts []string, s string) bool {
	for _, p := range parts {
		if p == s {
			return true
		}
	}
	return false
}
func after(parts []string, s string) string {
	for i, p := range parts {
		if p == s && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
