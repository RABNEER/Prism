package coverage

import (
	"fmt"
	"strings"
)

type Jurisdiction struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Aliases []string `json:"aliases"`
}

// Registry names establish routing only, not source availability or geocoding.
var jurisdictions = []Jurisdiction{
	{"Andhra Pradesh", "state", []string{"AP"}},
	{"Arunachal Pradesh", "state", []string{"AR"}},
	{"Assam", "state", []string{"AS"}},
	{"Bihar", "state", []string{"BR"}},
	{"Chhattisgarh", "state", []string{"CG", "CT"}},
	{"Goa", "state", []string{"GA"}},
	{"Gujarat", "state", []string{"GJ"}},
	{"Haryana", "state", []string{"HR"}},
	{"Himachal Pradesh", "state", []string{"HP"}},
	{"Jharkhand", "state", []string{"JH"}},
	{"Karnataka", "state", []string{"KA"}},
	{"Kerala", "state", []string{"KL"}},
	{"Madhya Pradesh", "state", []string{"MP"}},
	{"Maharashtra", "state", []string{"MH"}},
	{"Manipur", "state", []string{"MN"}},
	{"Meghalaya", "state", []string{"ML"}},
	{"Mizoram", "state", []string{"MZ"}},
	{"Nagaland", "state", []string{"NL"}},
	{"Odisha", "state", []string{"OD", "OR", "Orissa"}},
	{"Punjab", "state", []string{"PB"}},
	{"Rajasthan", "state", []string{"RJ"}},
	{"Sikkim", "state", []string{"SK"}},
	{"Tamil Nadu", "state", []string{"TN"}},
	{"Telangana", "state", []string{"TG", "TS"}},
	{"Tripura", "state", []string{"TR"}},
	{"Uttar Pradesh", "state", []string{"UP"}},
	{"Uttarakhand", "state", []string{"UK", "UT", "Uttaranchal"}},
	{"West Bengal", "state", []string{"WB"}},
	{"Andaman and Nicobar Islands", "union_territory", []string{"AN", "Andaman & Nicobar Islands"}},
	{"Chandigarh", "union_territory", []string{"CH"}},
	{"Dadra and Nagar Haveli and Daman and Diu", "union_territory", []string{"DN", "DH", "Dadra & Nagar Haveli and Daman & Diu"}},
	{"Delhi", "union_territory", []string{"DL", "NCT of Delhi", "New Delhi"}},
	{"Jammu and Kashmir", "union_territory", []string{"JK", "Jammu & Kashmir"}},
	{"Ladakh", "union_territory", []string{"LA"}},
	{"Lakshadweep", "union_territory", []string{"LD"}},
	{"Puducherry", "union_territory", []string{"PY", "Pondicherry"}},
}

func Normalize(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
func All() []Jurisdiction       { return append([]Jurisdiction{}, jurisdictions...) }

func Resolve(value string) (Jurisdiction, error) {
	for _, j := range jurisdictions {
		if Normalize(value) == Normalize(j.Name) {
			return j, nil
		}
		for _, a := range j.Aliases {
			if Normalize(value) == Normalize(a) {
				return j, nil
			}
		}
	}
	return Jurisdiction{}, fmt.Errorf("unknown Indian state/union territory %q; supply a state name or supported alias", value)
}
