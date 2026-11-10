"use client";

import { useEffect, useState } from "react";
import Link from "next/link";

type Evidence = { id: string; address: string; project?: string; unit_id?: string; data_mode: string; area?: { value: number; unit: string; basis: string }; price?: { amount: number; type: string }; source: { name: string; url: string }; retrieved_at: string; warnings: string[] };
type Decision = { left_id: string; right_id: string; kind: string; conflicts: string[]; reasons: string[] };
type Packet = { id: string; package: { records: Evidence[]; status: string; source_errors: string[]; warnings: string[]; missing_capabilities: string[]; reconciliation?: { decisions: Decision[]; duplicate_groups: { record_ids: string[] }[]; retained_ids: string[] }; comparable_selection?: { candidates: { evidence: Evidence; rank_score: number; reasons: string[] }[]; exclusions: { evidence_id: string; reason: string }[] }; location_context?: { counts: Record<string, number>; pois: { id: string; name: string; category: string; distance_m: number }[]; source: { provider: string; retrieved_at: string; snapshot_id: string }; dataset_release?: string }; registry_project?: { status: string; projects: { name: string; registration_number: string; district: string }[] } } };
type Bootstrap = { tools: number; version: string; firecrawl: { configured: boolean; authentication: string; permitted_domains: string[] } };
type Registry = { status: string; cache_hit: boolean; retrieved_at: string; projects: { name: string; registration_number: string; district: string; pincode: string }[] };
type Discovery = { discovery_id: string; status: string; discovery: { query: string; locality: string; candidates: { id: string; url: string; domain: string; title: string; description: string; likely_collection: boolean }[] } };

async function api(action: string, values: Record<string, unknown> = {}) {
  const response = await fetch("/api/prism", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ action, ...values }) });
  const result = await response.json();
  if (!result.ok) throw new Error(result.error || "The request failed");
  return result.data;
}
const money = (n: number) => new Intl.NumberFormat("en-IN", { style: "currency", currency: "INR", maximumFractionDigits: 0 }).format(n);
const readable = (s: string) => s.replaceAll("_", " ");

export default function Home() {
  const [connection, setConnection] = useState<Bootstrap | null>(null);
  const [locality, setLocality] = useState("Baner");
  const [area, setArea] = useState("100");
  const [basis, setBasis] = useState("carpet");
  const [registration, setRegistration] = useState("");
  const [priceType, setPriceType] = useState("asking_price");
  const [date, setDate] = useState("2026-11-07");
  const [withLocation, setWithLocation] = useState(false);
  const [latitude, setLatitude] = useState("18.5590");
  const [longitude, setLongitude] = useState("73.7868");
  const [url, setUrl] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [packet, setPacket] = useState<Packet | null>(null);
  const [exampleMode, setExampleMode] = useState(false);
  const [registry, setRegistry] = useState<Registry | null>(null);
  const [discovery, setDiscovery] = useState<Discovery | null>(null);
  const [tab, setTab] = useState("Evidence");
  const [raw, setRaw] = useState<unknown>(null);

  useEffect(() => { api("bootstrap").then(setConnection).catch((e) => setError(e.message)); }, []);
  async function run(action: "investigate" | "example") {
    setBusy(action); setError(""); setMessage(""); setRaw(null);
    try {
      const args = action === "example" ? { locality: "Baner", area: 100, area_basis: "carpet", as_of: "2026-11-07", price_type: "asking_price" } : { locality, area: Number(area), area_basis: basis, as_of: date, price_type: priceType, registration, with_location: withLocation, latitude: Number(latitude), longitude: Number(longitude) };
      const result = await api(action, args);
      setPacket(result.result); setExampleMode(result.example_mode); setTab("Evidence");
      setMessage(result.stored ? "Evidence package saved and read back through MCP." : "Evidence package created.");
    } catch (e) { setError(e instanceof Error ? e.message : "Request failed"); } finally { setBusy(""); }
  }
  async function ingest() {
    setBusy("ingest"); setError(""); setMessage("");
    try { const result = await api("ingest", { url, locality }); setMessage(`Listing saved as ${result.record.id.slice(0, 18)}… Run the investigation to include it.`); } catch (e) { setError(e instanceof Error ? e.message : "Ingestion failed"); } finally { setBusy(""); }
  }
  async function discover() {
    setBusy("discover"); setError(""); setMessage("");
    try { setDiscovery(await api("discover", { locality, purpose: priceType === "monthly_rent" ? "rent" : "sale" })); }
    catch (e) { setError(e instanceof Error ? e.message : "Discovery failed"); } finally { setBusy(""); }
  }
  async function ingestCandidate(id: string) {
    if (!discovery) return;
    setBusy("ingest"); setError(""); setMessage("");
    try { const result = await api("ingest_discovered", { discovery_id: discovery.discovery_id, candidate_id: id }); setMessage(`Discovered listing saved as ${result.record.id.slice(0, 18)}… Run an investigation for ${discovery.discovery.locality} to include it.`); }
    catch (e) { setError(e instanceof Error ? e.message : "Candidate could not be parsed"); } finally { setBusy(""); }
  }
  async function inspect(id: string) { setBusy("evidence"); setError(""); try { setRaw((await api("evidence", { id, example_mode: exampleMode })).record); setTab("Audit trail"); } catch (e) { setError(e instanceof Error ? e.message : "Read failed"); } finally { setBusy(""); } }
  async function checkRegistry() { setBusy("registry"); setError(""); try { setRegistry(await api("registry_sample")); } catch (e) { setError(e instanceof Error ? e.message : "Registry unavailable"); } finally { setBusy(""); } }
  function download() { if (!packet) return; const a = document.createElement("a"); const objectURL = URL.createObjectURL(new Blob([JSON.stringify(packet, null, 2)], { type: "application/json" })); a.href = objectURL; a.download = "prism-evidence-package.json"; a.click(); URL.revokeObjectURL(objectURL); }
  const pkg = packet?.package;
  const discoveryReady = !!connection && !["missing", "disabled"].includes(connection.firecrawl.authentication);
  const conflicts = pkg?.reconciliation?.decisions.filter((d) => d.kind === "source_revision_or_conflict") || [];

  return <main className="workspace">
    <aside className="rail">
      <Link className="brand" href="/" aria-label="PRISM home"><span className="brand-mark">◈</span><span>PRISM<small>PROPERTY INTELLIGENCE</small></span></Link>
      <div className="rail-label">WORKSPACE</div><div className="nav-item">◫ <span>Evidence desk</span><i /></div>
      <div className="rail-label">SOURCE ADAPTERS</div>
      <div className="source-row"><i className="source-dot" /><span>MahaRERA<small>Public project registry</small></span><b>01</b></div>
      <div className="source-row"><i className="source-dot" /><span>Overture Maps<small>Public place extracts</small></span><b>02</b></div>
      <div className="source-row"><i className={`source-dot ${discoveryReady ? "" : "amber"}`} /><span>Firecrawl<small>{discoveryReady ? "Discovery configured" : "Setup required"}</small></span><b>03</b></div>
      <div className="pilot-note"><div className="eyebrow">PILOT COVERAGE</div><strong>Pune, Maharashtra</strong><p>Residential apartments.<br />Baner & Kharadi first.</p><span>Focused pilot</span></div>
      <div className="rail-footer"><i className="source-dot" />{connection ? `MCP connected · ${connection.tools} tools` : "Connecting to MCP…"}<small>Evidence infrastructure · v{connection?.version || "—"}</small></div>
    </aside>
    <section className="desk">
      <header className="topbar"><div><span>Workspace</span><b>/</b>Evidence desk</div><div className="top-tag">PUNE PILOT <i /></div></header>
      <div className="page-content">
        <div className="heading-row"><div><div className="eyebrow">FROM PROPERTY TO PROVENANCE</div><h1>Every property has a story.<br /><span>Start with the evidence.</span></h1><p className="intro">Gather sources, reconcile records, and build a traceable property evidence package.</p></div><div className="edition">01<span>PROPERTY<br />RESEARCH DESK</span></div></div>
        <div className="main-grid">
          <section className="card input-card">
            <div className="card-heading"><span className="step">01</span><div><h2>Property details</h2><p>Subject attributes supplied by you</p></div><span className="corner-label">INPUT</span></div>
            <div className="form-grid">
              <label>Locality<select value={locality} onChange={(e) => { setLocality(e.target.value); setLatitude(e.target.value === "Baner" ? "18.5590" : "18.5515"); setLongitude(e.target.value === "Baner" ? "73.7868" : "73.9429"); }}><option>Baner</option><option>Kharadi</option></select></label>
              <label>Property type<div className="fixed-input">Residential apartment</div></label>
              <label>Area <span>· sq m</span><input type="number" min="10" max="1000" value={area} onChange={(e) => setArea(e.target.value)} /></label>
              <label>Area basis<select value={basis} onChange={(e) => setBasis(e.target.value)}><option value="carpet">Carpet area</option><option value="built_up">Built-up area</option><option value="super_built_up">Super built-up area</option></select></label>
              <label className="full">MahaRERA registration <span>· optional</span><input placeholder="Enter the subject project’s registration" value={registration} onChange={(e) => setRegistration(e.target.value)} /></label>
              <label>Evidence pool<select value={priceType} onChange={(e) => setPriceType(e.target.value)}><option value="asking_price">Asking prices</option><option value="registered_consideration">Registered sales</option><option value="monthly_rent">Monthly rents</option></select></label>
              <label>Selection date<input type="date" value={date} onChange={(e) => setDate(e.target.value)} /></label>
            </div>
            <label className="check"><input type="checkbox" checked={withLocation} onChange={(e) => setWithLocation(e.target.checked)} /><span>Fetch mapped amenities within 500 m</span></label>
            {withLocation && <div className="coordinates"><label>Latitude<input type="number" step="any" value={latitude} onChange={(e) => setLatitude(e.target.value)} /></label><label>Longitude<input type="number" step="any" value={longitude} onChange={(e) => setLongitude(e.target.value)} /></label><p>Approximate locality defaults. Supply the property’s coordinates for a meaningful proximity search.</p></div>}
            <button className="primary" disabled={!!busy || !connection} onClick={() => run("investigate")}>{busy === "investigate" ? "Gathering evidence…" : "Build evidence package"}<span>↗</span></button>
            <p className="form-note">Source evidence supports review. Property identity and market value are not established by this demo.</p>
          </section>
          <section className="results-area">
            <div className="results-heading"><div className="eyebrow">02 / INVESTIGATION</div>{packet && <button className="text-button" onClick={download}>Download JSON ↓</button>}</div>
            {error && <div role="alert" className="notice error">{error}</div>}
            {message && <div role="status" className="notice success">{message}</div>}
            {busy && <div className="activity" role="status"><i />{busy === "ingest" ? "Extracting a permitted listing through Firecrawl…" : "Working through the MCP…"}<small>First-time public data retrieval may take a minute. Cached sources retain their retrieval timestamps.</small></div>}
            {exampleMode && packet && <div className="notice example"><strong>Synthetic mechanism demo</strong>All property records and asking prices below are artificial. This mode demonstrates duplicate handling and selection rules.</div>}
            {!packet ? <div className="card empty-state"><div className="empty-diagram"><i /><i /><span>◈</span><b /><b /></div><div className="eyebrow">AN EVIDENCE PACKAGE, NOT A GUESS</div><h2>A clear view starts here.</h2><p>Run an investigation to see available records, comparable candidates, data conflicts, and the sources behind them.</p><div className="empty-features"><span>↗ Source provenance</span><span>⌘ Reconciliation</span><span>◷ Reproducible snapshot</span></div><hr /><p className="small">No records yet? Explore the workflow with labeled artificial examples.</p><button className="secondary" disabled={!!busy || !connection} onClick={() => run("example")}>Explore synthetic example <span>→</span></button></div> : <>
              <div className="metrics"><div><span>Source records</span><strong>{pkg?.records.length || 0}</strong><small>{exampleMode ? "Synthetic observations" : "Available observations"}</small></div><div><span>Duplicate groups</span><strong>{pkg?.reconciliation?.duplicate_groups.length ?? "—"}</strong><small>Repeated source observations</small></div><div><span>Review flags</span><strong>{conflicts.length + (pkg?.source_errors.length || 0)}</strong><small>Conflicts & source failures</small></div></div>
              <div className="tabs" role="tablist">{["Evidence", "Reconciliation", "Comparables", "Audit trail"].map((name) => <button key={name} role="tab" aria-selected={tab === name} className={tab === name ? "selected" : ""} onClick={() => setTab(name)}>{name}</button>)}</div>
              <div className="card result-card">
                {tab === "Evidence" && <><div className="section-title"><h2>Property evidence</h2><span className="badge">{readable(pkg?.status || "partial")}</span></div>{!pkg?.records.length && <p className="empty-copy">No listing or transaction records match this query. Ingest a permitted listing or use the synthetic example. Missing evidence is not replaced with invented values.</p>}{pkg?.records.map((e) => <article className="record" key={e.id}><div><span className={`badge ${e.data_mode === "synthetic_fixture" ? "amber-badge" : ""}`}>{e.data_mode === "synthetic_fixture" ? "Synthetic" : "Source observation"}</span><h3>{e.project || e.address}</h3><p>{e.address}</p><div className="record-meta">{e.area && <span>{e.area.value} {e.area.unit.replace("_", " ")} · {readable(e.area.basis)}</span>}<a href={e.source.url} target="_blank" rel="noreferrer">{e.source.name} ↗</a></div></div><div className="record-price">{e.price ? <><strong>{money(e.price.amount)}</strong><small>{readable(e.price.type)}</small></> : <small>Price not established</small>}<button className="text-button" onClick={() => inspect(e.id)}>View source record →</button></div></article>)}{pkg?.registry_project && <div className="context-block"><h3>Registry evidence</h3>{pkg.registry_project.projects.map((p) => <p key={p.registration_number}>{p.name} · {p.registration_number} · {p.district}</p>)}<small>The registration has not been matched automatically to the subject apartment.</small></div>}{pkg?.location_context && <div className="context-block"><h3>Mapped location context</h3><div className="count-row">{Object.entries(pkg.location_context.counts).map(([key, value]) => <span key={key}><strong>{value}</strong>{key}</span>)}</div><small>{pkg.location_context.source.provider} · Release {pkg.location_context.dataset_release || "source snapshot"} · Straight-line proximity</small><button className="text-button" onClick={() => inspect(pkg.location_context!.source.snapshot_id)}>Inspect location source →</button></div>}</>}
                {tab === "Reconciliation" && <><div className="section-title"><h2>Keep the evidence distinct.</h2><span className="badge">Rust matching</span></div><p className="section-copy">Only identical source observations are grouped. Different units, repeated sales, and conflicting records remain separate.</p>{pkg?.reconciliation?.decisions.filter((d) => d.kind !== "unresolved").map((d, i) => <div className="decision" key={i}><span className={d.kind === "source_revision_or_conflict" ? "warn" : ""}>{d.kind === "same_source_observation" ? "=" : d.kind === "different" ? "≠" : "!"}</span><div><h3>{readable(d.kind)}</h3><p>{[...d.reasons, ...d.conflicts].map(readable).join(" · ")}</p><code>{d.left_id.slice(0, 14)}… ↔ {d.right_id.slice(0, 14)}…</code></div></div>)}{!pkg?.reconciliation?.decisions.length && <p className="empty-copy">No record pairs available to reconcile.</p>}</>}
                {tab === "Comparables" && <><div className="section-title"><h2>Comparable candidates</h2><span className="badge">One price pool</span></div><p className="section-copy">Selected by locality, area basis, size, and date. Ranking scores are rule-based similarity, not valuation confidence.</p>{pkg?.comparable_selection?.candidates.map((c, i) => <div className="comparable" key={c.evidence.id}><span>{String(i + 1).padStart(2, "0")}</span><div><h3>{c.evidence.address}</h3><p>{c.reasons.map(readable).join(" · ")}</p></div><strong>{c.evidence.price ? money(c.evidence.price.amount) : "—"}</strong></div>)}{!pkg?.comparable_selection?.candidates.length && <p className="empty-copy">No eligible candidates. Missing area/date information and conflicting source versions are excluded.</p>}<details><summary>Selection exclusions ({pkg?.comparable_selection?.exclusions.length || 0})</summary>{pkg?.comparable_selection?.exclusions.map((e, i) => <p className="exclusion" key={i}><code>{e.evidence_id.slice(0, 16)}…</code>{readable(e.reason)}</p>)}</details></>}
                {tab === "Audit trail" && <><div className="section-title"><h2>Evidence you can trace.</h2><span className="badge">Saved snapshot</span></div><p className="section-copy">Inspect structured records or download the package. Snapshot integrity is checked on read.</p><div className="snapshot-id"><span>PACKAGE ID</span><code>{packet.id}</code></div><pre>{JSON.stringify(raw ?? packet, null, 2)}</pre></>}
              </div>
              {!!pkg?.source_errors.length && <div className="notice error"><strong>Some sources were unavailable</strong>{pkg.source_errors.map((e, i) => <p key={i}>{e}</p>)}</div>}
              <details className="limitations"><summary>Evidence limitations & unresolved capabilities</summary>{pkg?.missing_capabilities.map((v) => <span className="capability" key={v}>{readable(v)}</span>)}{pkg?.warnings.map((v, i) => <p key={i}>{v}</p>)}</details>
            </>}
          </section>
        </div>
        <div className="bottom-grid"><section className="card source-card"><div className="card-heading"><span className="step">↗</span><div><h2>Find property evidence</h2><p>Discover → fetch → validate → MCP</p></div><span className={`badge ${discoveryReady ? "" : "amber-badge"}`}>{discoveryReady ? "Search configured" : "Configure access"}</span></div><button className="secondary" disabled={!!busy || !discoveryReady} onClick={discover}>{busy === "discover" ? "Searching sources…" : "Discover listing sources ↗"}</button><p className="small">Search the selected locality through MCP. No manual source list is needed for discovered candidates.</p>{discovery && <div className="discovery-results"><p className="small">{readable(discovery.status)} · {discovery.discovery.locality}. Search snippets are discovery hints, not price evidence.</p>{discovery.discovery.candidates.map((candidate) => <article key={candidate.id} className="discovery-candidate"><a href={candidate.url} target="_blank" rel="noreferrer">{candidate.title || candidate.domain} ↗</a><p className="small">{candidate.domain} · {candidate.likely_collection ? "Likely collection page; individual listing needed" : "Unverified page; parser validation required"}</p><button className="text-button" disabled={!!busy || candidate.likely_collection} onClick={() => ingestCandidate(candidate.id)}>Fetch and validate listing →</button></article>)}</div>}<label className="sr-only" htmlFor="listing-url">Permitted listing URL</label><div className="url-row"><input id="listing-url" type="url" placeholder="https://permitted-source.example/property" value={url} onChange={(e) => setUrl(e.target.value)} /><button className="secondary" disabled={!!busy || !connection?.firecrawl.configured || !url} onClick={ingest}>Ingest listing ↗</button></div><p className="small">Uses the selected locality and configured source domains. Prices remain asking-price or rent evidence.</p></section><section className="card source-card registry-card"><div className="eyebrow">PUBLIC SOURCE CHECK</div><h2>MahaRERA reference sample</h2><p className="small">Verify a known Pune-district project. This reference sample stays separate from your subject property.</p><button className="text-button" disabled={!!busy || !connection} onClick={checkRegistry}>Check P52100012200 →</button>{registry && <div className="registry-result"><p><strong>{readable(registry.status)}</strong></p>{!registry.projects.length && <p>No matching project returned by the source. Portal responses can vary; this does not establish that a registration is invalid.</p>}{registry.projects.map((p) => <p key={p.registration_number}><strong>{p.name}</strong><br />{p.district} · {p.pincode}</p>)}<small>{registry.cache_hit ? "Cached source" : "Retrieved from source"} · {new Date(registry.retrieved_at).toLocaleString("en-IN")}</small></div>}</section></div>
        <footer className="page-footer"><span>PRISM / Property Research, Intelligence & Smart Modeling</span><span>Evidence first. Every source accounted for.</span></footer>
      </div>
    </section>
  </main>;
}
