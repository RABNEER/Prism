package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"prism/internal/adapters/firecrawl"
	"prism/internal/adapters/geography"
	"prism/internal/adapters/maharera"
	"prism/internal/matching"
	"prism/internal/mcpserver"
	"prism/internal/services"
	"prism/internal/storage"
)

func main() {
	data := flag.String("data-dir", "data", "Local development evidence and snapshot directory")
	live := flag.Bool("live-sources", true, "Enable on-demand live geographic and project-registry sources for prototype use")
	defaults := geography.Defaults()
	geocodingURL := flag.String("geocoding-url", defaults.GeocodingURL, "Operator-configured Open-Meteo-compatible geocoding endpoint")
	overpassURL := flag.String("overpass-url", defaults.OverpassURL, "Operator-configured Overpass endpoint")
	userAgent := flag.String("source-user-agent", defaults.UserAgent, "Application identifier for upstream requests")
	registryURL := flag.String("maharera-url", maharera.Endpoint, "Operator-configured MahaRERA-compatible public search endpoint")
	exe, _ := os.Executable()
	workerName := "prism-matching"
	if runtime.GOOS == "windows" {
		workerName += ".exe"
	}
	worker := flag.String("matching-worker", filepath.Join(filepath.Dir(exe), workerName), "Rust JSON-lines matching worker executable")
	repo := filepath.Dir(filepath.Dir(exe))
	amenityScript := flag.String("amenity-script", filepath.Join(repo, "scripts", "fetch_public_amenities.py"), "Python Overture retrieval worker; empty disables")
	python := flag.String("python", "python", "Python executable for public data retrieval")
	pythonModules := flag.String("python-modules", filepath.Join(repo, ".cache", "python"), "Workspace-isolated Python dependencies")
	firecrawlEndpoint := flag.String("firecrawl-url", "https://api.firecrawl.dev/v2/scrape", "Operator-configured Firecrawl API endpoint")
	firecrawlSources := flag.String("firecrawl-sources", "", "JSON map of permitted source hostnames to permission references")
	anonFirecrawl := flag.Bool("firecrawl-anonymous", false, "Explicitly enable documented anonymous trial access")
	selfHostedFirecrawl := flag.Bool("firecrawl-self-hosted", false, "Use an explicitly configured loopback Firecrawl server without hosted credentials")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	service := services.Service{Store: storage.Store{Root: *data}}
	permitted := map[string]string{}
	if *firecrawlSources != "" {
		b, err := os.ReadFile(*firecrawlSources)
		if err != nil {
			log.Fatal(err)
		}
		if err = json.Unmarshal(b, &permitted); err != nil {
			log.Fatal(err)
		}
	}
	fc, err := firecrawl.New(firecrawl.Config{Endpoint: *firecrawlEndpoint, APIKey: os.Getenv("FIRECRAWL_API_KEY"), AllowAnonymous: *anonFirecrawl, SelfHosted: *selfHostedFirecrawl, Sources: permitted}, service.Store)
	if err != nil {
		log.Fatal(err)
	}
	if *live {
		service.Firecrawl = fc
	}
	if info, err := os.Stat(*worker); err == nil && !info.IsDir() {
		service.Matcher = &matching.Client{Path: *worker}
	}
	if *live {
		config := geography.Config{GeocodingURL: *geocodingURL, OverpassURL: *overpassURL, UserAgent: *userAgent}
		if info, err := os.Stat(*amenityScript); *amenityScript != "" && err == nil && !info.IsDir() {
			config.OvertureScript = *amenityScript
			config.OverturePython = *python
			config.PythonModules = *pythonModules
		}
		client, err := geography.New(config, service.Store)
		if err != nil {
			log.Fatal(err)
		}
		service.Geography = client
		registry, err := maharera.New(service.Store, *registryURL, *userAgent)
		if err != nil {
			log.Fatal(err)
		}
		service.Registry = registry
	}
	server := mcpserver.New(service)
	log.SetOutput(os.Stderr)
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
