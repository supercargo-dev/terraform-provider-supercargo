package manifest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v3"
)

// ResolvedContract represents a data contract resolved from a product manifest and local filesystem.
type ResolvedContract struct {
	URN          string `json:"urn"`
	Version      string `json:"version"`
	Name         string `json:"name"`
	ContractPath string `json:"contract_path"`
	SchemaPath   string `json:"schema_path"`
	SchemaJSON   string `json:"schema_json"`
	ContentHash  string `json:"content_hash"`
	CommitSha    string `json:"commit_sha"`
	DataAsset    string `json:"data_asset"`
}

type rawContract struct {
	Meta struct {
		URN       string `yaml:"urn" json:"urn"`
		ID        string `yaml:"id" json:"id"`
		Version   string `yaml:"version" json:"version"`
		DataAsset string `yaml:"data_asset" json:"data_asset"`
		CommitSha string `yaml:"commit_sha" json:"commit_sha"`
		OwnerTeam string `yaml:"owner_team" json:"owner_team"`
	} `yaml:"meta" json:"meta"`
	Schema []rawContractField `yaml:"schema" json:"schema"`
}

type rawContractField struct {
	Name        string             `yaml:"name" json:"name"`
	Type        string             `yaml:"type" json:"type"`
	Mode        string             `yaml:"mode" json:"mode"`
	Description string             `yaml:"description" json:"description"`
	Fields      []rawContractField `yaml:"fields" json:"fields"`
}

type bqFieldJSON struct {
	Name        string        `json:"name"`
	Type        string        `json:"type"`
	Mode        string        `json:"mode,omitempty"`
	Description string        `json:"description,omitempty"`
	Fields      []bqFieldJSON `json:"fields,omitempty"`
}

// Load reads a YAML manifest file and unmarshals it into a protobuf message.
func Load(path string) (*hubv1.ProductManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file: %w", err)
	}

	// 1. Unmarshal YAML to generic map
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	// 2. Marshal to JSON
	jsonBytes, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to convert YAML to JSON: %w", err)
	}

	// 3. Unmarshal JSON to Proto
	manifest := &hubv1.ProductManifest{}
	unmarshaler := protojson.UnmarshalOptions{
		DiscardUnknown: true, // Allow for fields in YAML that aren't in Proto yet
	}
	if err := unmarshaler.Unmarshal(jsonBytes, manifest); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON to Proto: %w", err)
	}

	return manifest, nil
}

// ResolveContracts resolves contract YAML files and BigQuery schema JSON files for all output ports in a product manifest.
func ResolveContracts(manifestPath string, manifest *hubv1.ProductManifest) (map[string]*ResolvedContract, error) {
	if manifest == nil || len(manifest.OutputPorts) == 0 {
		return make(map[string]*ResolvedContract), nil
	}

	manifestClean := filepath.Clean(manifestPath)
	manifestDir := filepath.Dir(manifestClean)
	resolved := make(map[string]*ResolvedContract, len(manifest.OutputPorts))

	for _, port := range manifest.OutputPorts {
		if port == nil || port.Contract == nil {
			continue
		}

		portName := port.Name
		contractURN := port.Contract.Urn
		contractVersion := port.Contract.Version
		explicitPath := port.Contract.Path

		baseNames := candidateBaseNames(portName, contractURN)
		versions := candidateVersions(contractVersion)

		// 1. Discover Contract YAML
		contractPath, err := discoverContractFile(manifestDir, explicitPath, baseNames, versions)
		if err != nil {
			return nil, fmt.Errorf("failed looking for contract file for port %q: %w", portName, err)
		}

		var rawCont rawContract
		var contentHash string
		var dataAsset string
		var commitSha string

		if contractPath != "" {
			data, err := os.ReadFile(contractPath)
			if err != nil {
				return nil, fmt.Errorf("failed reading contract file %q: %w", contractPath, err)
			}
			h := sha256.Sum256(data)
			contentHash = fmt.Sprintf("%x", h)

			if err := yaml.Unmarshal(data, &rawCont); err != nil {
				return nil, fmt.Errorf("failed parsing contract YAML %q: %w", contractPath, err)
			}

			if rawCont.Meta.URN != "" {
				if contractURN == "" {
					contractURN = rawCont.Meta.URN
				}
			} else if rawCont.Meta.ID != "" {
				if contractURN == "" {
					contractURN = rawCont.Meta.ID
				}
			}
			if rawCont.Meta.Version != "" && contractVersion == "" {
				contractVersion = rawCont.Meta.Version
			}
			dataAsset = rawCont.Meta.DataAsset
			commitSha = rawCont.Meta.CommitSha
		}

		// Recompute candidate base names and versions once metadata is discovered
		baseNames = candidateBaseNames(portName, contractURN)
		versions = candidateVersions(contractVersion)

		// 2. Discover Schema JSON or fallback to dynamic generation
		schemaPath, schemaJSON, err := discoverSchemaJSONFile(manifestDir, baseNames, versions)
		if err != nil {
			return nil, fmt.Errorf("failed looking for schema JSON for port %q: %w", portName, err)
		}

		if schemaJSON == "" {
			// Dynamic fallback generation if contract YAML provided schema fields
			if contractPath != "" && len(rawCont.Schema) > 0 {
				genJSON, err := generateBigQuerySchemaJSON(rawCont.Schema)
				if err != nil {
					return nil, fmt.Errorf("failed generating schema JSON for port %q: %w", portName, err)
				}
				schemaJSON = genJSON
			} else {
				return nil, fmt.Errorf("missing contract file and schema JSON for port %q (urn: %q)", portName, contractURN)
			}
		}

		mapKey := contractURN
		if mapKey == "" {
			mapKey = portName
		}

		resolved[mapKey] = &ResolvedContract{
			URN:          contractURN,
			Version:      contractVersion,
			Name:         portName,
			ContractPath: contractPath,
			SchemaPath:   schemaPath,
			SchemaJSON:   schemaJSON,
			ContentHash:  contentHash,
			CommitSha:    commitSha,
			DataAsset:    dataAsset,
		}
	}

	return resolved, nil
}

func candidateBaseNames(portName, contractURN string) []string {
	seen := make(map[string]struct{})
	var list []string

	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			list = append(list, s)
		}
	}

	addVariants := func(s string) {
		if s == "" {
			return
		}
		add(s)
		add(toSnakeCase(s))
		add(toKebabCase(s))
	}

	addVariants(portName)
	if contractURN != "" {
		urnName := extractContractName(contractURN)
		addVariants(urnName)
	}

	return list
}

func candidateVersions(version string) []string {
	seen := make(map[string]struct{})
	var list []string

	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			list = append(list, s)
		}
	}

	if version != "" {
		add(version)
		trimmed := strings.TrimPrefix(version, "v")
		add(trimmed)
		add("v" + trimmed)
	}

	return list
}

func extractContractName(urn string) string {
	idx := strings.LastIndex(urn, ":")
	if idx >= 0 && idx < len(urn)-1 {
		return urn[idx+1:]
	}
	return urn
}

func toSnakeCase(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "-", "_"))
}

func toKebabCase(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, "_", "-"))
}

func discoverContractFile(manifestDir, explicitPath string, baseNames, versions []string) (string, error) {
	if explicitPath != "" {
		target := explicitPath
		if !filepath.IsAbs(target) {
			target = filepath.Join(manifestDir, explicitPath)
		}
		target = filepath.Clean(target)
		if fi, err := os.Stat(target); err == nil && !fi.IsDir() {
			return target, nil
		}
		return "", fmt.Errorf("explicit contract file not found: %s", target)
	}

	searchDirs := []string{
		filepath.Join(manifestDir, "contracts"),
		manifestDir,
	}

	for _, dir := range searchDirs {
		for _, baseName := range baseNames {
			// 1. Versioned patterns
			for _, v := range versions {
				patterns := []string{
					fmt.Sprintf("%s.%s.contract.yaml", baseName, v),
					fmt.Sprintf("%s.%s.contract.yml", baseName, v),
					fmt.Sprintf("%s.%s.yaml", baseName, v),
					fmt.Sprintf("%s.%s.yml", baseName, v),
				}
				for _, p := range patterns {
					candidate := filepath.Join(dir, p)
					if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
						return filepath.Clean(candidate), nil
					}
				}
			}

			// 2. Unversioned patterns
			patterns := []string{
				fmt.Sprintf("%s.contract.yaml", baseName),
				fmt.Sprintf("%s.contract.yml", baseName),
				fmt.Sprintf("%s.yaml", baseName),
				fmt.Sprintf("%s.yml", baseName),
			}
			for _, p := range patterns {
				candidate := filepath.Join(dir, p)
				if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
					return filepath.Clean(candidate), nil
				}
			}
		}
	}

	return "", nil
}

func discoverSchemaJSONFile(manifestDir string, baseNames, versions []string) (string, string, error) {
	searchDirs := []string{
		filepath.Join(manifestDir, "schemas"),
		manifestDir,
	}

	for _, dir := range searchDirs {
		for _, baseName := range baseNames {
			for _, v := range versions {
				patterns := []string{
					fmt.Sprintf("%s.%s.bigquery.json", baseName, v),
					fmt.Sprintf("%s.%s.json", baseName, v),
				}
				for _, p := range patterns {
					candidate := filepath.Join(dir, p)
					if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
						data, err := os.ReadFile(candidate)
						if err != nil {
							return "", "", fmt.Errorf("failed reading schema file %q: %w", candidate, err)
						}
						return filepath.Clean(candidate), string(data), nil
					}
				}
			}

			patterns := []string{
				fmt.Sprintf("%s.bigquery.json", baseName),
				fmt.Sprintf("%s.json", baseName),
			}
			for _, p := range patterns {
				candidate := filepath.Join(dir, p)
				if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
					data, err := os.ReadFile(candidate)
					if err != nil {
						return "", "", fmt.Errorf("failed reading schema file %q: %w", candidate, err)
					}
					return filepath.Clean(candidate), string(data), nil
				}
			}
		}
	}

	return "", "", nil
}

func generateBigQuerySchemaJSON(fields []rawContractField) (string, error) {
	bqFields := convertToBQFields(fields, 0)
	bytes, err := json.MarshalIndent(bqFields, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed marshaling BigQuery schema JSON: %w", err)
	}
	return string(bytes), nil
}

func convertToBQFields(fields []rawContractField, depth int) []bqFieldJSON {
	if depth > 100 {
		return nil
	}
	bqFields := make([]bqFieldJSON, len(fields))
	for i, f := range fields {
		bq := bqFieldJSON{
			Name:        f.Name,
			Type:        normalizeBQType(f.Type),
			Mode:        normalizeBQMode(f.Mode),
			Description: f.Description,
		}
		if len(f.Fields) > 0 {
			bq.Fields = convertToBQFields(f.Fields, depth+1)
		}
		bqFields[i] = bq
	}
	return bqFields
}

func normalizeBQType(t string) string {
	t = strings.ToUpper(strings.TrimSpace(t))
	t = strings.TrimPrefix(t, "DATA_TYPE_")
	switch t {
	case "INT", "INTEGER", "INT32", "INT64":
		return "INT64"
	case "FLOAT", "DOUBLE", "FLOAT32", "FLOAT64":
		return "FLOAT64"
	case "BOOL", "BOOLEAN":
		return "BOOL"
	case "STRING":
		return "STRING"
	case "BYTES":
		return "BYTES"
	case "TIMESTAMP":
		return "TIMESTAMP"
	case "DATE":
		return "DATE"
	case "TIME":
		return "TIME"
	case "DATETIME":
		return "DATETIME"
	case "GEOGRAPHY":
		return "GEOGRAPHY"
	case "NUMERIC":
		return "NUMERIC"
	case "BIGNUMERIC":
		return "BIGNUMERIC"
	case "JSON":
		return "JSON"
	case "STRUCT", "RECORD", "OBJECT", "MESSAGE", "MAP":
		return "STRUCT"
	default:
		return t
	}
}

func normalizeBQMode(m string) string {
	m = strings.ToUpper(strings.TrimSpace(m))
	m = strings.TrimPrefix(m, "FIELD_MODE_")
	switch m {
	case "REQUIRED":
		return "REQUIRED"
	case "REPEATED", "ARRAY", "LIST":
		return "REPEATED"
	case "NULLABLE", "OPTIONAL":
		return "NULLABLE"
	case "":
		return "NULLABLE"
	default:
		return m
	}
}

