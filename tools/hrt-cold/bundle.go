package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"
)

const (
	bundleFormat        = "oyama-hrt-complete-backup"
	bundleFormatVersion = 1
	maxJSONDepth        = 64
	maxJSONStringBytes  = 1 << 20
)

var (
	errDuplicateKey = errors.New("duplicate JSON object key")
	errDepthTooDeep = errors.New("JSON nesting depth exceeds limit")
	errBundleSchema = errors.New("invalid complete-backup bundle")
)

type bundleInfo struct {
	CreatedAt string
	Mode      string
}

// bundleEnvelope intentionally validates only the hand-off envelope. Record
// semantics remain the website importer's responsibility.
type bundleEnvelope struct {
	Format            string                     `json:"format"`
	FormatVersion     json.RawMessage            `json:"formatVersion"`
	CreatedAt         string                     `json:"createdAt"`
	Mode              string                     `json:"mode"`
	Modes             map[string]json.RawMessage `json:"modes"`
	Weight            json.RawMessage            `json:"weight"`
	WeightUpdatedAt   json.RawMessage            `json:"weightUpdatedAt"`
	PKParams          json.RawMessage            `json:"pkParams"`
	PKParamsUpdatedAt json.RawMessage            `json:"pkParamsUpdatedAt"`
	Calibration       json.RawMessage            `json:"calibration"`
	Preferences       json.RawMessage            `json:"preferences"`
}

func validateBundle(raw []byte) (*bundleInfo, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: empty input", errBundleSchema)
	}
	if len(raw) > maxBundleBytes {
		return nil, fmt.Errorf("%w: bundle is %d bytes", errTooLarge, len(raw))
	}
	if err := checkJSONStructure(raw); err != nil {
		return nil, err
	}

	var envelope bundleEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("%w: %v", errBundleSchema, err)
	}
	if envelope.Format != bundleFormat {
		return nil, fmt.Errorf("%w: format %q", errBundleSchema, envelope.Format)
	}
	version, err := decodeJSONNumber(envelope.FormatVersion, "formatVersion")
	if err != nil {
		return nil, err
	}
	if version.String() != strconv.Itoa(bundleFormatVersion) {
		return nil, fmt.Errorf("%w: unsupported formatVersion %q", errBundleSchema, version.String())
	}
	if _, err := time.Parse(time.RFC3339, envelope.CreatedAt); err != nil {
		return nil, fmt.Errorf("%w: createdAt must be RFC 3339: %v", errBundleSchema, err)
	}
	if envelope.Mode != "transfem" && envelope.Mode != "transmasc" {
		return nil, fmt.Errorf("%w: mode %q", errBundleSchema, envelope.Mode)
	}
	if envelope.Modes == nil {
		return nil, fmt.Errorf("%w: modes must be an object", errBundleSchema)
	}
	for _, mode := range []string{"transfem", "transmasc"} {
		block, ok := envelope.Modes[mode]
		if !ok {
			return nil, fmt.Errorf("%w: modes.%s is required", errBundleSchema, mode)
		}
		if err := validateModeBlock(mode, block); err != nil {
			return nil, err
		}
	}
	if err := validateOptionalNumber(envelope.Weight, "weight", true, 0); err != nil {
		return nil, err
	}
	if err := validateOptionalNumber(envelope.WeightUpdatedAt, "weightUpdatedAt", true, 0); err != nil {
		return nil, err
	}
	if err := validateOptionalNumber(envelope.PKParamsUpdatedAt, "pkParamsUpdatedAt", true, 0); err != nil {
		return nil, err
	}
	if len(envelope.PKParams) > 0 && !isJSONNull(envelope.PKParams) {
		if err := validateObject(envelope.PKParams, "pkParams"); err != nil {
			return nil, err
		}
	}
	if len(envelope.Calibration) > 0 {
		if err := validateCalibration(envelope.Calibration); err != nil {
			return nil, err
		}
	}
	if len(envelope.Preferences) > 0 {
		if err := validateObject(envelope.Preferences, "preferences"); err != nil {
			return nil, err
		}
	}
	return &bundleInfo{CreatedAt: envelope.CreatedAt, Mode: envelope.Mode}, nil
}

func validateModeBlock(mode string, raw json.RawMessage) error {
	if isJSONNull(raw) {
		return fmt.Errorf("%w: modes.%s must be an object", errBundleSchema, mode)
	}
	var block map[string]json.RawMessage
	if err := json.Unmarshal(raw, &block); err != nil || block == nil {
		return fmt.Errorf("%w: modes.%s must be an object", errBundleSchema, mode)
	}
	for _, category := range []string{"events", "labResults", "doseTemplates", "quickDoses", "supplies", "plan"} {
		value, ok := block[category]
		if !ok {
			continue
		}
		if err := validateArrayOfObjects(value, "modes."+mode+"."+category); err != nil {
			return err
		}
	}
	if value, ok := block["deletions"]; ok {
		if err := validateObject(value, "modes."+mode+".deletions"); err != nil {
			return err
		}
	}
	return nil
}

func validateArrayOfObjects(raw json.RawMessage, field string) error {
	if isJSONNull(raw) {
		return fmt.Errorf("%w: %s must be an array", errBundleSchema, field)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("%w: %s must be an array: %v", errBundleSchema, field, err)
	}
	for i, item := range items {
		if isJSONNull(item) {
			return fmt.Errorf("%w: %s[%d] must be an object", errBundleSchema, field, i)
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(item, &object); err != nil || object == nil {
			return fmt.Errorf("%w: %s[%d] must be an object", errBundleSchema, field, i)
		}
	}
	return nil
}

func validateObject(raw json.RawMessage, field string) error {
	if isJSONNull(raw) {
		return fmt.Errorf("%w: %s must be an object", errBundleSchema, field)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return fmt.Errorf("%w: %s must be an object", errBundleSchema, field)
	}
	return nil
}

func validateCalibration(raw json.RawMessage) error {
	if err := validateObject(raw, "calibration"); err != nil {
		return err
	}
	var calibration struct {
		Method      string `json:"method"`
		HistoryMode string `json:"historyMode"`
	}
	if err := json.Unmarshal(raw, &calibration); err != nil {
		return fmt.Errorf("%w: calibration: %v", errBundleSchema, err)
	}
	switch calibration.Method {
	case "off", "ekf", "ou_kalman", "mipd":
	default:
		return fmt.Errorf("%w: calibration.method %q", errBundleSchema, calibration.Method)
	}
	switch calibration.HistoryMode {
	case "forward", "retrospective":
	default:
		return fmt.Errorf("%w: calibration.historyMode %q", errBundleSchema, calibration.HistoryMode)
	}
	return nil
}

func validateOptionalNumber(raw json.RawMessage, field string, allowNull bool, minimum float64) error {
	if len(raw) == 0 {
		return nil
	}
	if isJSONNull(raw) {
		if allowNull {
			return nil
		}
		return fmt.Errorf("%w: %s must be a number", errBundleSchema, field)
	}
	number, err := decodeJSONNumber(raw, field)
	if err != nil {
		return err
	}
	value, err := number.Float64()
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < minimum {
		return fmt.Errorf("%w: %s has an invalid numeric value", errBundleSchema, field)
	}
	return nil
}

func decodeJSONNumber(raw []byte, field string) (json.Number, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || (trimmed[0] != '-' && (trimmed[0] < '0' || trimmed[0] > '9')) {
		return "", fmt.Errorf("%w: %s must be a number", errBundleSchema, field)
	}
	var number json.Number
	if err := json.Unmarshal(trimmed, &number); err != nil {
		return "", fmt.Errorf("%w: %s must be a number", errBundleSchema, field)
	}
	return number, nil
}

func isJSONNull(raw []byte) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// checkJSONStructure rejects duplicate object keys, excessive nesting, very
// large strings, and trailing JSON values before typed decoding.
func checkJSONStructure(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var walk func(depth int) error
	walk = func(depth int) error {
		if depth > maxJSONDepth {
			return errDepthTooDeep
		}
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("%w: %v", errBundleSchema, err)
		}
		switch value := token.(type) {
		case string:
			if len(value) > maxJSONStringBytes {
				return fmt.Errorf("%w: string exceeds %d bytes", errBundleSchema, maxJSONStringBytes)
			}
		case json.Delim:
			switch value {
			case '{':
				seen := make(map[string]bool)
				for decoder.More() {
					keyToken, err := decoder.Token()
					if err != nil {
						return fmt.Errorf("%w: %v", errBundleSchema, err)
					}
					key, ok := keyToken.(string)
					if !ok {
						return fmt.Errorf("%w: object key is not a string", errBundleSchema)
					}
					if len(key) > maxJSONStringBytes {
						return fmt.Errorf("%w: object key exceeds %d bytes", errBundleSchema, maxJSONStringBytes)
					}
					if seen[key] {
						return fmt.Errorf("%w: %q", errDuplicateKey, key)
					}
					seen[key] = true
					if err := walk(depth + 1); err != nil {
						return err
					}
				}
				end, err := decoder.Token()
				if err != nil || end != json.Delim('}') {
					return fmt.Errorf("%w: malformed object", errBundleSchema)
				}
			case '[':
				for decoder.More() {
					if err := walk(depth + 1); err != nil {
						return err
					}
				}
				end, err := decoder.Token()
				if err != nil || end != json.Delim(']') {
					return fmt.Errorf("%w: malformed array", errBundleSchema)
				}
			default:
				return fmt.Errorf("%w: unexpected delimiter %v", errBundleSchema, value)
			}
		}
		return nil
	}
	if err := walk(0); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("%w: trailing JSON value", errBundleSchema)
		}
		return fmt.Errorf("%w: trailing JSON: %v", errBundleSchema, err)
	}
	return nil
}
