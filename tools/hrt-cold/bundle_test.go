package main

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

const minimalBundle = `{"format":"oyama-hrt-complete-backup","formatVersion":1,"createdAt":"2030-01-01T00:00:00Z","mode":"transfem","modes":{"transfem":{},"transmasc":{}}}`

func TestValidateSyntheticBundle(t *testing.T) {
	raw, err := os.ReadFile("testdata/synthetic-bundle.json")
	if err != nil {
		t.Fatal(err)
	}
	info, err := validateBundle(raw)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode != "transfem" || info.CreatedAt != "2030-01-01T00:00:00.000Z" {
		t.Fatalf("unexpected bundle info: %+v", info)
	}
	if _, err := validateBundle([]byte(minimalBundle)); err != nil {
		t.Fatalf("minimal bundle rejected: %v", err)
	}
}

func TestValidateBundleAllowsUnknownFields(t *testing.T) {
	raw, err := os.ReadFile("testdata/synthetic-bundle.json")
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	object["futureTopLevel"] = map[string]any{"value": 1}
	modes := object["modes"].(map[string]any)
	transfem := modes["transfem"].(map[string]any)
	transfem["futureCategory"] = []any{}
	transfem["events"].([]any)[0].(map[string]any)["futureRecordField"] = true
	modified, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validateBundle(modified); err != nil {
		t.Fatalf("unknown fields rejected: %v", err)
	}
}

func TestValidateBundleRejectsInvalidInput(t *testing.T) {
	replace := func(old, new string) []byte {
		t.Helper()
		if !strings.Contains(minimalBundle, old) {
			t.Fatalf("fixture does not contain %q", old)
		}
		return []byte(strings.Replace(minimalBundle, old, new, 1))
	}
	withTopLevel := func(extra string) []byte {
		t.Helper()
		return []byte(strings.TrimSuffix(minimalBundle, "}") + "," + extra + "}")
	}
	tests := []struct {
		name string
		in   []byte
		want error
	}{
		{"empty", nil, errBundleSchema},
		{"not json", []byte("{"), errBundleSchema},
		{"top-level array", []byte("[]"), errBundleSchema},
		{"top-level null", []byte("null"), errBundleSchema},
		{"wrong format", replace(bundleFormat, "other-format"), errBundleSchema},
		{"wrong version", replace(`"formatVersion":1`, `"formatVersion":2`), errBundleSchema},
		{"string version", replace(`"formatVersion":1`, `"formatVersion":"1"`), errBundleSchema},
		{"float version", replace(`"formatVersion":1`, `"formatVersion":1.0`), errBundleSchema},
		{"missing createdAt", []byte(strings.Replace(minimalBundle, `"createdAt":"2030-01-01T00:00:00Z",`, "", 1)), errBundleSchema},
		{"bad createdAt", replace("2030-01-01T00:00:00Z", "not-a-date"), errBundleSchema},
		{"bad mode", replace(`"mode":"transfem"`, `"mode":"other"`), errBundleSchema},
		{"missing modes", []byte(`{"format":"oyama-hrt-complete-backup","formatVersion":1,"createdAt":"2030-01-01T00:00:00Z","mode":"transfem"}`), errBundleSchema},
		{"null modes", replace(`"modes":{"transfem":{},"transmasc":{}}`, `"modes":null`), errBundleSchema},
		{"missing transmasc", replace(`"transmasc":{}`, `"transmasc":null`), errBundleSchema},
		{"null mode block", replace(`"transfem":{}`, `"transfem":null`), errBundleSchema},
		{"array mode block", replace(`"transfem":{}`, `"transfem":[]`), errBundleSchema},
		{"category object", replace(`"transfem":{}`, `"transfem":{"events":{}}`), errBundleSchema},
		{"category null", replace(`"transfem":{}`, `"transfem":{"events":null}`), errBundleSchema},
		{"category item null", replace(`"transfem":{}`, `"transfem":{"events":[null]}`), errBundleSchema},
		{"category item number", replace(`"transfem":{}`, `"transfem":{"events":[1]}`), errBundleSchema},
		{"deletions array", replace(`"transfem":{}`, `"transfem":{"deletions":[]}`), errBundleSchema},
		{"calibration bad method", withTopLevel(`"calibration":{"method":"x","historyMode":"forward"}`), errBundleSchema},
		{"calibration bad history", withTopLevel(`"calibration":{"method":"ekf","historyMode":"x"}`), errBundleSchema},
		{"preferences array", withTopLevel(`"preferences":[]`), errBundleSchema},
		{"negative weight", withTopLevel(`"weight":-1`), errBundleSchema},
		{"weight string", withTopLevel(`"weight":"70"`), errBundleSchema},
		{"pkParams array", withTopLevel(`"pkParams":[]`), errBundleSchema},
		{"duplicate top-level key", []byte(`{"format":"oyama-hrt-complete-backup","format":"oyama-hrt-complete-backup","formatVersion":1,"createdAt":"2030-01-01T00:00:00Z","mode":"transfem","modes":{"transfem":{},"transmasc":{}}}`), errDuplicateKey},
		{"duplicate nested key", replace(`"transfem":{}`, `"transfem":{"events":[{"id":"one","id":"two"}]}`), errDuplicateKey},
		{"trailing value", []byte(minimalBundle + `{}`), errBundleSchema},
		{"too deep", []byte(strings.Repeat("[", maxJSONDepth+1) + "0" + strings.Repeat("]", maxJSONDepth+1)), errDepthTooDeep},
		{"long string", []byte(`{"format":"` + strings.Repeat("a", maxJSONStringBytes+1) + `"}`), errBundleSchema},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := validateBundle(test.in)
			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
	if _, err := validateBundle(make([]byte, maxBundleBytes+1)); !errors.Is(err, errTooLarge) {
		t.Fatalf("oversized bundle: got %v", err)
	}
}
