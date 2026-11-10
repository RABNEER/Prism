package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"prism/internal/coverage"
	"prism/internal/domain"
	"prism/internal/matching"
	"prism/internal/storage"
)

type ReconcileInput struct {
	Query           domain.Query `json:"query"`
	IncludeFixtures bool         `json:"include_fixtures,omitempty"`
}
type DuplicateGroup struct {
	RepresentativeID string   `json:"representative_id"`
	RecordIDs        []string `json:"record_ids"`
}
type Reconciliation struct {
	WorkerVersion   string              `json:"worker_version"`
	Decisions       []matching.Decision `json:"decisions"`
	DuplicateGroups []DuplicateGroup    `json:"duplicate_groups"`
	RetainedIDs     []string            `json:"retained_ids"`
	Warnings        []string            `json:"warnings"`
}

func observationKey(e domain.Evidence) string {
	e.ID = ""
	e.RetrievedAt = ""
	e.SourceSnapshotID = ""
	e.Warnings = nil
	b, _ := json.Marshal(e)
	return storage.Digest(b)
}

func (s Service) Reconcile(ctx context.Context, in ReconcileInput) (Reconciliation, error) {
	q, err := s.canonicalQuery(in.Query)
	if err != nil {
		return Reconciliation{}, err
	}
	records, err := s.matching(ctx, q, in.IncludeFixtures)
	if err != nil {
		return Reconciliation{}, err
	}
	return s.reconcileRecords(ctx, records)
}

func (s Service) reconcileRecords(ctx context.Context, records []domain.Evidence) (Reconciliation, error) {
	if s.Matcher == nil {
		return Reconciliation{}, fmt.Errorf("Rust matching worker unavailable; build/configure prism-matching")
	}
	if len(records) > 200 {
		return Reconciliation{}, fmt.Errorf("reconciliation supports at most 200 records; narrow the query")
	}
	pairs := []matching.Pair{}
	byID := map[string]domain.Evidence{}
	for i, a := range records {
		byID[a.ID] = a
		for _, b := range records[i+1:] {
			pairs = append(pairs, matching.Pair{Left: matching.FromEvidence(a), Right: matching.FromEvidence(b)})
		}
	}
	result, err := s.Matcher.Compare(ctx, pairs)
	if err != nil {
		return Reconciliation{}, err
	}
	for i, d := range result.Decisions {
		if d.Kind == "same_source_observation" && observationKey(byID[d.LeftID]) != observationKey(byID[d.RightID]) {
			result.Decisions[i].Kind = "source_revision_or_conflict"
			result.Decisions[i].Reasons = append(d.Reasons, "other_observation_fields_differ")
		}
	}
	out := Reconciliation{WorkerVersion: result.WorkerVersion, Decisions: result.Decisions, DuplicateGroups: []DuplicateGroup{}, RetainedIDs: []string{}, Warnings: []string{"Similarity is an uncalibrated address-overlap heuristic, not a probability or valuation confidence.", "Only repeated identical source observations are collapsed. Cross-source matches remain candidates; repeated transactions and conflicting attributes are retained."}}
	groups := map[string][]string{}
	for _, e := range records {
		key := observationKey(e)
		groups[key] = append(groups[key], e.ID)
	}
	for _, ids := range groups {
		sort.Strings(ids)
		out.RetainedIDs = append(out.RetainedIDs, ids[0])
		if len(ids) > 1 {
			out.DuplicateGroups = append(out.DuplicateGroups, DuplicateGroup{RepresentativeID: ids[0], RecordIDs: ids})
		}
	}
	sort.Strings(out.RetainedIDs)
	sort.Slice(out.DuplicateGroups, func(i, j int) bool {
		return out.DuplicateGroups[i].RepresentativeID < out.DuplicateGroups[j].RepresentativeID
	})
	return out, nil
}

type Subject struct {
	Query    domain.Query        `json:"query"`
	Address  string              `json:"address"`
	Project  string              `json:"project,omitempty"`
	Building string              `json:"building,omitempty"`
	UnitID   string              `json:"unit_id,omitempty"`
	Area     domain.Area         `json:"area"`
	Location *domain.Coordinates `json:"location,omitempty"`
	Bedrooms *int                `json:"bedrooms,omitempty"`
}
type ComparablesInput struct {
	Subject         Subject `json:"subject"`
	PriceType       string  `json:"price_type" jsonschema:"registered_consideration, asking_price or monthly_rent; pools are never mixed"`
	Limit           int     `json:"limit,omitempty"`
	MaxDistanceM    int     `json:"max_distance_m,omitempty"`
	MaxAgeDays      int     `json:"max_age_days,omitempty"`
	AsOf            string  `json:"as_of" jsonschema:"YYYY-MM-DD required to make selection reproducible"`
	IncludeFixtures bool    `json:"include_fixtures,omitempty"`
}
type Comparable struct {
	Evidence              domain.Evidence `json:"evidence"`
	RankScore             float64         `json:"rank_score"`
	AreaDifferencePercent float64         `json:"area_difference_percent"`
	DistanceM             *float64        `json:"distance_m,omitempty"`
	AgeDays               int             `json:"age_days"`
	Reasons               []string        `json:"reasons"`
}
type Exclusion struct {
	EvidenceID string `json:"evidence_id"`
	Reason     string `json:"reason"`
}
type Comparables struct {
	PriceType  string       `json:"price_type"`
	Candidates []Comparable `json:"candidates"`
	Exclusions []Exclusion  `json:"exclusions"`
	Warnings   []string     `json:"warnings"`
}

func areaSqFt(a domain.Area) float64 {
	if a.Unit == "sq_m" {
		return a.Value * 10.76391041671
	}
	if a.Unit == "acre" {
		return a.Value * 43560
	}
	if a.Unit == "hectare" {
		return a.Value * 107639.1041671
	}
	return a.Value
}

func (s Service) Comparables(ctx context.Context, in ComparablesInput) (Comparables, error) {
	if in.Subject.Query.AsOf != "" && in.Subject.Query.AsOf != in.AsOf {
		return Comparables{}, fmt.Errorf("subject and selection as_of must agree")
	}
	in.Subject.Query.AsOf = in.AsOf
	q, err := s.canonicalQuery(in.Subject.Query)
	if err != nil {
		return Comparables{}, err
	}
	date, err := time.Parse(time.DateOnly, in.AsOf)
	if err != nil {
		return Comparables{}, fmt.Errorf("as_of is required as YYYY-MM-DD")
	}
	if in.PriceType != "registered_consideration" && in.PriceType != "asking_price" && in.PriceType != "monthly_rent" {
		return Comparables{}, fmt.Errorf("unsupported comparable price_type")
	}
	a := in.Subject.Area
	if a.Value <= 0 || math.IsNaN(a.Value) || math.IsInf(a.Value, 0) || (a.Unit != "sq_ft" && a.Unit != "sq_m" && a.Unit != "acre" && a.Unit != "hectare") || (a.Basis != "carpet" && a.Basis != "built_up" && a.Basis != "super_built_up" && a.Basis != "land") {
		return Comparables{}, fmt.Errorf("subject needs positive area, known unit and known area basis")
	}
	if q.District == "" || q.Locality == "" {
		return Comparables{}, fmt.Errorf("district and locality are required for comparable selection")
	}
	if in.Subject.Bedrooms != nil && (*in.Subject.Bedrooms < 0 || *in.Subject.Bedrooms > 100) {
		return Comparables{}, fmt.Errorf("invalid subject bedrooms")
	}
	if in.Limit == 0 {
		in.Limit = 10
	}
	if in.Limit < 1 || in.Limit > 50 {
		return Comparables{}, fmt.Errorf("limit must be 1-50")
	}
	if in.MaxAgeDays == 0 {
		in.MaxAgeDays = 365
	}
	if in.MaxAgeDays < 1 || in.MaxAgeDays > 3650 {
		return Comparables{}, fmt.Errorf("max_age_days must be 1-3650")
	}
	if in.MaxDistanceM == 0 {
		in.MaxDistanceM = 5000
	}
	if in.MaxDistanceM < 100 || in.MaxDistanceM > 50000 {
		return Comparables{}, fmt.Errorf("max_distance_m must be 100-50000")
	}
	if l := in.Subject.Location; l != nil && (math.IsNaN(l.Latitude) || math.IsNaN(l.Longitude) || l.Latitude < -90 || l.Latitude > 90 || l.Longitude < -180 || l.Longitude > 180) {
		return Comparables{}, fmt.Errorf("invalid subject coordinates")
	}
	records, err := s.matching(ctx, q, in.IncludeFixtures)
	if err != nil {
		return Comparables{}, err
	}
	out := Comparables{PriceType: in.PriceType, Candidates: []Comparable{}, Exclusions: []Exclusion{}, Warnings: []string{"Candidates are selected by transparent rules, not validated as equivalent market transactions. No valuation is generated.", "Rank scores are uncalibrated ranking heuristics. Same-locality candidates without coordinates have no measured distance."}}
	if in.IncludeFixtures {
		out.Warnings = append(out.Warnings, "Synthetic fixtures are enabled; do not use them for lending or accuracy claims.")
	}
	seen := map[string]bool{}
	sourceVersions := map[string]map[string]bool{}
	for _, e := range records {
		key := e.Source.ID + "|" + e.ProviderRecordID + "|" + e.EventDate
		if sourceVersions[key] == nil {
			sourceVersions[key] = map[string]bool{}
		}
		sourceVersions[key][observationKey(e)] = true
	}
	for _, e := range records {
		reason := ""
		if len(sourceVersions[e.Source.ID+"|"+e.ProviderRecordID+"|"+e.EventDate]) > 1 {
			out.Exclusions = append(out.Exclusions, Exclusion{e.ID, "conflicting_source_record_versions"})
			continue
		}
		event := e.EventDate
		if e.Price == nil || e.Price.Type != in.PriceType {
			reason = "price_type_or_price_missing"
		} else if in.PriceType == "registered_consideration" && e.Category != "transaction" {
			reason = "not_a_sale_transaction"
		} else if e.EntityLevel != "unit" {
			reason = "not_unit_level_evidence"
		} else if e.Area == nil || e.Area.Basis != a.Basis {
			reason = "area_basis_mismatch_or_missing"
		}
		if event == "" {
			if in.PriceType == "registered_consideration" {
				reason = "transaction_date_missing"
			} else {
				event = e.PublishedDate
			}
		}
		when, parseErr := time.Parse(time.DateOnly, event)
		age := int(date.Sub(when).Hours() / 24)
		if parseErr != nil || age < 0 || age > in.MaxAgeDays {
			reason = "outside_date_window_or_unknown_date"
		}
		if e.UnitID != "" && in.Subject.UnitID != "" && coverage.Normalize(e.UnitID) == coverage.Normalize(in.Subject.UnitID) && ((e.Project != "" && coverage.Normalize(e.Project) == coverage.Normalize(in.Subject.Project)) || (e.Building != "" && coverage.Normalize(e.Building) == coverage.Normalize(in.Subject.Building))) {
			reason = "same_subject_unit"
		}
		if e.Address != "" && coverage.Normalize(e.Address) == coverage.Normalize(in.Subject.Address) && in.Subject.Address != "" {
			reason = "same_subject_address"
		}
		if reason != "" {
			out.Exclusions = append(out.Exclusions, Exclusion{e.ID, reason})
			continue
		}
		key := observationKey(e)
		if seen[key] {
			out.Exclusions = append(out.Exclusions, Exclusion{e.ID, "repeated_source_observation"})
			continue
		}
		seen[key] = true
		diff := math.Abs(areaSqFt(*e.Area)-areaSqFt(a)) / areaSqFt(a)
		if diff > 0.35 {
			out.Exclusions = append(out.Exclusions, Exclusion{e.ID, "area_difference_exceeds_35_percent"})
			continue
		}
		comp := Comparable{Evidence: e, AreaDifferencePercent: math.Round(diff*10000) / 100, AgeDays: age, Reasons: []string{"same_state_district_locality_and_property_type", "same_area_basis", "same_price_type"}}
		distancePenalty := 0.25
		if in.Subject.Location != nil && e.Location != nil {
			distance := distanceM(*in.Subject.Location, *e.Location)
			if distance > float64(in.MaxDistanceM) {
				out.Exclusions = append(out.Exclusions, Exclusion{e.ID, "beyond_distance_limit"})
				continue
			}
			distance = math.Round(distance*10) / 10
			comp.DistanceM = &distance
			distancePenalty = distance / float64(in.MaxDistanceM)
			comp.Reasons = append(comp.Reasons, "within_straight_line_distance_limit")
		} else {
			comp.Reasons = append(comp.Reasons, "distance_unknown_same_locality_only")
		}
		if in.Subject.Bedrooms != nil && e.Bedrooms != nil && *in.Subject.Bedrooms != *e.Bedrooms {
			out.Exclusions = append(out.Exclusions, Exclusion{e.ID, "bedroom_mismatch"})
			continue
		}
		comp.RankScore = math.Round((1-(0.5*diff/0.35+0.3*float64(age)/float64(in.MaxAgeDays)+0.2*distancePenalty))*10000) / 10000
		out.Candidates = append(out.Candidates, comp)
	}
	sort.Slice(out.Candidates, func(i, j int) bool {
		if out.Candidates[i].RankScore == out.Candidates[j].RankScore {
			return out.Candidates[i].Evidence.ID < out.Candidates[j].Evidence.ID
		}
		return out.Candidates[i].RankScore > out.Candidates[j].RankScore
	})
	if len(out.Candidates) > in.Limit {
		out.Candidates = out.Candidates[:in.Limit]
	}
	return out, nil
}

func distanceM(a, b domain.Coordinates) float64 {
	r := math.Pi / 180
	dy := (b.Latitude - a.Latitude) * r
	dx := (b.Longitude - a.Longitude) * r
	v := math.Sin(dy/2)*math.Sin(dy/2) + math.Cos(a.Latitude*r)*math.Cos(b.Latitude*r)*math.Sin(dx/2)*math.Sin(dx/2)
	return 6371008.8 * 2 * math.Atan2(math.Sqrt(v), math.Sqrt(math.Max(0, 1-v)))
}
