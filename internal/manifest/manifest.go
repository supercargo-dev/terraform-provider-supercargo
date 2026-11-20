package manifest

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v3"
)

// ErrMissingContractAndSchema indicates neither a contract file with schema fields nor a schema JSON file was found for a port.
var ErrMissingContractAndSchema = errors.New("missing contract file and schema JSON")

// IngestionEngine identifies how an asset is materialized into BigQuery.
type IngestionEngine string

const (
	// IngestionEngineUnspecified defaults to standard streaming behavior (or manifest/port inference).
	IngestionEngineUnspecified IngestionEngine = ""
	// IngestionEngineStreaming materializes repeated fields as BigQuery REPEATED arrays.
	IngestionEngineStreaming IngestionEngine = "streaming"
	// IngestionEngineDLT materializes repeated fields as JSON NULLABLE (dlt max_table_nesting=0).
	IngestionEngineDLT IngestionEngine = "dlt"
)

// SchemaOptions configures BigQuery schema translation options.
type SchemaOptions struct {
	IngestionEngine    IngestionEngine
	IncludeDLTMetadata bool
}

// ManifestExtras carries optional YAML-only port lineage fields (output_ports[].source_ref and transforms)
// that may not be present on older pinned hub/v1 protobuf structs.
type ManifestExtras struct {
	OutputSourceRefs map[string]string
	TransformNames   map[string]bool
	TransformTargets map[string]bool
}

var bearerTokenRegex = regexp.MustCompile(`(?i)\bbearer\s+[^\s"',;]+`)

// ScrubBearerTokens redacts bearer credentials in diagnostic and error strings.
func ScrubBearerTokens(msg string) string {
	return bearerTokenRegex.ReplaceAllString(msg, "Bearer [REDACTED]")
}

// ParseIngestionEngine parses and validates a BigQuery ingestion engine identifier,
// scrubbing any bearer tokens on error paths.
func ParseIngestionEngine(raw string) (IngestionEngine, error) {
	trimmed := strings.TrimSpace(raw)
	switch strings.ToLower(trimmed) {
	case "", "unspecified":
		return IngestionEngineUnspecified, nil
	case "streaming":
		return IngestionEngineStreaming, nil
	case "dlt":
		return IngestionEngineDLT, nil
	default:
		return "", fmt.Errorf("unsupported ingestion engine %q: must be \"streaming\" or \"dlt\"", ScrubBearerTokens(trimmed))
	}
}

// InferIngestionEngine infers the IngestionEngine from an input port source URI and type.
func InferIngestionEngine(source, portType string) IngestionEngine {
	src := strings.ToLower(strings.TrimSpace(source))
	pt := strings.ToLower(strings.TrimSpace(portType))
	if pt == "dlt" || src == "dlt" || strings.HasPrefix(src, "dlt://") {
		return IngestionEngineDLT
	}
	return IngestionEngineStreaming
}

func parseEngineFromLabels(labels map[string]string) (IngestionEngine, bool, error) {
	if len(labels) == 0 {
		return IngestionEngineUnspecified, false, nil
	}
	for _, k := range []string{"supercargo.ingestion_engine", "ingestion_engine"} {
		if raw, ok := labels[k]; ok && strings.TrimSpace(raw) != "" {
			parsed, err := ParseIngestionEngine(raw)
			if err != nil {
				return "", false, err
			}
			if parsed == IngestionEngineUnspecified {
				return IngestionEngineStreaming, true, nil
			}
			return parsed, true, nil
		}
	}
	return IngestionEngineUnspecified, false, nil
}

// LoadManifestExtras parses optional YAML-only source_ref and transforms fields from a dataproduct.yaml file.
func LoadManifestExtras(manifestPath string) ManifestExtras {
	var extras ManifestExtras
	if strings.TrimSpace(manifestPath) == "" {
		return extras
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return extras
	}
	var raw struct {
		OutputPorts []struct {
			Name      string `yaml:"name"`
			SourceRef string `yaml:"source_ref"`
		} `yaml:"output_ports"`
		Transforms []struct {
			Name          string   `yaml:"name"`
			TargetOutputs []string `yaml:"target_outputs"`
		} `yaml:"transforms"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return extras
	}
	if len(raw.OutputPorts) > 0 {
		extras.OutputSourceRefs = make(map[string]string, len(raw.OutputPorts))
		for _, op := range raw.OutputPorts {
			if op.Name != "" && op.SourceRef != "" {
				extras.OutputSourceRefs[op.Name] = op.SourceRef
			}
		}
	}
	if len(raw.Transforms) > 0 {
		extras.TransformNames = make(map[string]bool, len(raw.Transforms))
		extras.TransformTargets = make(map[string]bool, len(raw.Transforms))
		for _, tr := range raw.Transforms {
			if tr.Name != "" {
				extras.TransformNames[tr.Name] = true
			}
			for _, target := range tr.TargetOutputs {
				if target != "" {
					extras.TransformTargets[target] = true
				}
			}
		}
	}
	return extras
}

// ResolvePortSchemaOptions resolves per-port BigQuery SchemaOptions from explicit overrides,
// contract/manifest labels, output port evolution_profile, source_ref / contract URN links,
// and DLT input ports while enforcing Transform Isolation.
func ResolvePortSchemaOptions(
	manifest *hubv1.ProductManifest,
	outputPort *hubv1.OutputPort,
	inputPort *hubv1.InputPort,
	contractLabels map[string]string,
	contractURN string,
	explicit SchemaOptions,
	extras ...ManifestExtras,
) (SchemaOptions, error) {
	parsedExplicit, err := ParseIngestionEngine(string(explicit.IngestionEngine))
	if err != nil {
		return SchemaOptions{}, err
	}
	resolved := SchemaOptions{
		IncludeDLTMetadata: explicit.IncludeDLTMetadata,
	}
	if parsedExplicit != IngestionEngineUnspecified {
		resolved.IngestionEngine = parsedExplicit
		return resolved, nil
	}

	if eng, ok, err := parseEngineFromLabels(contractLabels); err != nil {
		return SchemaOptions{}, err
	} else if ok {
		resolved.IngestionEngine = eng
		return resolved, nil
	}

	var manifestEngine IngestionEngine
	var hasManifestLabel bool
	if manifest != nil && manifest.Meta != nil {
		manifestEngine, hasManifestLabel, err = parseEngineFromLabels(manifest.Meta.Labels)
		if err != nil {
			return SchemaOptions{}, err
		}
	}

	var ext ManifestExtras
	if len(extras) > 0 {
		ext = extras[0]
	}

	resolveForOutput := func(port *hubv1.OutputPort) IngestionEngine {
		prof := strings.ToUpper(strings.TrimSpace(port.EvolutionProfile))
		if prof == "BIGQUERY_DLT" || prof == "DLT" {
			return IngestionEngineDLT
		}
		sourceRef := ext.OutputSourceRefs[port.Name]
		isTransformBacked := ext.TransformTargets[port.Name] || (sourceRef != "" && ext.TransformNames[sourceRef])
		if isTransformBacked {
			if hasManifestLabel {
				return manifestEngine
			}
			return IngestionEngineStreaming
		}
		if manifest != nil {
			if sourceRef != "" {
				for _, in := range manifest.InputPorts {
					if in != nil && in.Name == sourceRef {
						if InferIngestionEngine(in.Source, in.Type) == IngestionEngineDLT {
							return IngestionEngineDLT
						}
					}
				}
			} else if port.Contract != nil && port.Contract.Urn != "" {
				for _, in := range manifest.InputPorts {
					if in == nil {
						continue
					}
					inURN := in.ContractUrn
					if in.Contract != nil && in.Contract.Urn != "" {
						inURN = in.Contract.Urn
					}
					if inURN == port.Contract.Urn && InferIngestionEngine(in.Source, in.Type) == IngestionEngineDLT {
						return IngestionEngineDLT
					}
				}
			}
			if sourceRef == "" && len(manifest.InputPorts) == 1 && manifest.InputPorts[0] != nil {
				if InferIngestionEngine(manifest.InputPorts[0].Source, manifest.InputPorts[0].Type) == IngestionEngineDLT {
					return IngestionEngineDLT
				}
			}
		}
		if hasManifestLabel {
			return manifestEngine
		}
		return IngestionEngineStreaming
	}

	if outputPort != nil {
		resolved.IngestionEngine = resolveForOutput(outputPort)
		return resolved, nil
	}

	if inputPort != nil {
		if InferIngestionEngine(inputPort.Source, inputPort.Type) == IngestionEngineDLT {
			resolved.IngestionEngine = IngestionEngineDLT
			return resolved, nil
		}
		if hasManifestLabel {
			resolved.IngestionEngine = manifestEngine
			return resolved, nil
		}
		resolved.IngestionEngine = IngestionEngineStreaming
		return resolved, nil
	}

	if manifest != nil && contractURN != "" {
		for _, op := range manifest.OutputPorts {
			if op != nil && op.Contract != nil && op.Contract.Urn == contractURN {
				resolved.IngestionEngine = resolveForOutput(op)
				return resolved, nil
			}
		}
		for _, in := range manifest.InputPorts {
			if in == nil {
				continue
			}
			inURN := in.ContractUrn
			if in.Contract != nil && in.Contract.Urn != "" {
				inURN = in.Contract.Urn
			}
			if (inURN != "" && inURN == contractURN) || (in.Name != "" && strings.Contains(contractURN, "-internal-"+in.Name)) {
				if InferIngestionEngine(in.Source, in.Type) == IngestionEngineDLT {
					resolved.IngestionEngine = IngestionEngineDLT
					return resolved, nil
				}
			}
		}
	}

	if hasManifestLabel {
		resolved.IngestionEngine = manifestEngine
		return resolved, nil
	}
	resolved.IngestionEngine = IngestionEngineStreaming
	return resolved, nil
}

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
		URN       string            `yaml:"urn" json:"urn"`
		ID        string            `yaml:"id" json:"id"`
		Version   string            `yaml:"version" json:"version"`
		DataAsset string            `yaml:"data_asset" json:"data_asset"`
		CommitSha string            `yaml:"commit_sha" json:"commit_sha"`
		OwnerTeam string            `yaml:"owner_team" json:"owner_team"`
		Labels    map[string]string `yaml:"labels" json:"labels"`
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

// BQFieldJSON represents a field in a BigQuery JSON schema.
type BQFieldJSON struct {
	Name        string        `json:"name"`
	Type        string        `json:"type"`
	Mode        string        `json:"mode,omitempty"`
	Description string        `json:"description,omitempty"`
	Fields      []BQFieldJSON `json:"fields,omitempty"`
}

// ParseSLATier parses flexible SLA tier names into hubv1.SLATier enum.
func ParseSLATier(tierStr string) (hubv1.SLATier, bool) {
	s := strings.ToUpper(strings.TrimSpace(tierStr))
	if val, ok := hubv1.SLATier_value[s]; ok {
		return hubv1.SLATier(val), true
	}
	if val, ok := hubv1.SLATier_value["SLA_TIER_"+s]; ok {
		return hubv1.SLATier(val), true
	}

	switch s {
	case "GOLD", "TIER_1", "TIER1", "1", "MISSION_CRITICAL", "MISSION-CRITICAL":
		return hubv1.SLATier_SLA_TIER_1_MISSION_CRITICAL, true
	case "SILVER", "TIER_2", "TIER2", "2", "IMPORTANT":
		return hubv1.SLATier_SLA_TIER_2_IMPORTANT, true
	case "BRONZE", "TIER_3", "TIER3", "3", "BEST_EFFORT", "BEST-EFFORT":
		return hubv1.SLATier_SLA_TIER_3_BEST_EFFORT, true
	case "UNSPECIFIED", "0":
		return hubv1.SLATier_SLA_TIER_UNSPECIFIED, true
	default:
		return hubv1.SLATier_SLA_TIER_UNSPECIFIED, false
	}
}

// Load reads a YAML manifest file and unmarshals it into a protobuf message.
func Load(path string) (*hubv1.ProductManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file: %w", err)
	}

	// 1. Unmarshal YAML to generic map
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	if slaRaw, ok := raw["sla"].(map[string]any); ok {
		if tierVal, ok := slaRaw["tier"].(string); ok && tierVal != "" {
			parsedTier, ok := ParseSLATier(tierVal)
			if !ok {
				return nil, fmt.Errorf("invalid SLA tier: '%s' is not a valid SLA tier. Valid values include: SLA_TIER_1_MISSION_CRITICAL (GOLD, TIER_1, MISSION_CRITICAL), SLA_TIER_2_IMPORTANT (SILVER, TIER_2, IMPORTANT), SLA_TIER_3_BEST_EFFORT (BRONZE, TIER_3, BEST_EFFORT)", tierVal)
			}
			slaRaw["tier"] = parsedTier.String()
		}
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

// ResolveContracts resolves contract YAML files and BigQuery schema JSON files for all input and output ports in a product manifest.
func ResolveContracts(manifestPath string, manifest *hubv1.ProductManifest) (map[string]*ResolvedContract, error) {
	return ResolveContractsWithOptions(manifestPath, manifest, SchemaOptions{})
}

// ResolveContractsWithOptions resolves contract YAML files and BigQuery schema JSON files
// for all input and output ports in a product manifest while applying DLT/streaming SchemaOptions.
func ResolveContractsWithOptions(manifestPath string, manifest *hubv1.ProductManifest, explicitOpts SchemaOptions) (map[string]*ResolvedContract, error) {
	parsedExplicit, err := ParseIngestionEngine(string(explicitOpts.IngestionEngine))
	if err != nil {
		return nil, err
	}
	explicitOpts.IngestionEngine = parsedExplicit

	if explicitOpts.IngestionEngine == IngestionEngineUnspecified && manifest != nil && manifest.Meta != nil {
		if _, _, err := parseEngineFromLabels(manifest.Meta.Labels); err != nil {
			return nil, err
		}
	}

	if manifest == nil || (len(manifest.OutputPorts) == 0 && len(manifest.InputPorts) == 0) {
		return make(map[string]*ResolvedContract), nil
	}

	manifestClean := filepath.Clean(manifestPath)
	manifestDir := filepath.Dir(manifestClean)
	extras := LoadManifestExtras(manifestPath)
	totalPorts := len(manifest.OutputPorts) + len(manifest.InputPorts)
	resolved := make(map[string]*ResolvedContract, totalPorts)

	type portRef struct {
		name       string
		contract   *hubv1.ContractPointer
		outputPort *hubv1.OutputPort
		inputPort  *hubv1.InputPort
	}
	ports := make([]portRef, 0, totalPorts)
	for _, port := range manifest.OutputPorts {
		if port != nil && port.Contract != nil {
			ports = append(ports, portRef{name: port.Name, contract: port.Contract, outputPort: port})
		}
	}
	for _, port := range manifest.InputPorts {
		if port != nil && port.Contract != nil {
			ports = append(ports, portRef{name: port.Name, contract: port.Contract, inputPort: port})
		}
	}

	for _, port := range ports {
		portName := port.name
		contractURN := port.contract.Urn
		contractVersion := port.contract.Version
		explicitPath := port.contract.Path

		mapKey := contractURN
		if mapKey == "" {
			mapKey = portName
		}
		if _, exists := resolved[mapKey]; exists {
			continue
		}

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

		portOpts, err := ResolvePortSchemaOptions(manifest, port.outputPort, port.inputPort, rawCont.Meta.Labels, contractURN, explicitOpts, extras)
		if err != nil {
			return nil, fmt.Errorf("resolving ingestion engine for port %q: %w", portName, err)
		}

		// Recompute candidate base names and versions once metadata is discovered
		baseNames = candidateBaseNames(portName, contractURN)
		versions = candidateVersions(contractVersion)

		// 2. Generate dynamically when DLT / metadata / explicit override is active; otherwise preserve static .bigquery.json precedence.
		needsDynamicOrTransform := portOpts.IngestionEngine == IngestionEngineDLT || portOpts.IncludeDLTMetadata || explicitOpts.IngestionEngine != IngestionEngineUnspecified

		var schemaPath string
		var schemaJSON string

		if needsDynamicOrTransform {
			if contractPath != "" && len(rawCont.Schema) > 0 {
				genJSON, err := generateBigQuerySchemaJSONWithOptions(rawCont.Schema, portOpts)
				if err != nil {
					return nil, fmt.Errorf("failed generating schema JSON for port %q: %w", portName, err)
				}
				schemaJSON = genJSON
				schemaPath = ""
			} else {
				schemaPath, schemaJSON, err = discoverSchemaJSONFile(manifestDir, baseNames, versions)
				if err != nil {
					return nil, fmt.Errorf("failed looking for schema JSON for port %q: %w", portName, err)
				}
				if schemaJSON == "" {
					return nil, fmt.Errorf("%w for port %q (urn: %q)", ErrMissingContractAndSchema, portName, contractURN)
				}
				transformed, err := transformSchemaJSONWithOptions(schemaJSON, portOpts)
				if err != nil {
					return nil, fmt.Errorf("failed transforming schema JSON for port %q: %w", portName, err)
				}
				schemaJSON = transformed
			}
		} else {
			schemaPath, schemaJSON, err = discoverSchemaJSONFile(manifestDir, baseNames, versions)
			if err != nil {
				return nil, fmt.Errorf("failed looking for schema JSON for port %q: %w", portName, err)
			}
			if schemaJSON == "" {
				if contractPath != "" && len(rawCont.Schema) > 0 {
					genJSON, err := generateBigQuerySchemaJSONWithOptions(rawCont.Schema, portOpts)
					if err != nil {
						return nil, fmt.Errorf("failed generating schema JSON for port %q: %w", portName, err)
					}
					schemaJSON = genJSON
				} else {
					return nil, fmt.Errorf("%w for port %q (urn: %q)", ErrMissingContractAndSchema, portName, contractURN)
				}
			}
		}

		if contractURN != "" {
			mapKey = contractURN
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

func generateBigQuerySchemaJSONWithOptions(fields []rawContractField, opts SchemaOptions) (string, error) {
	parsedEngine, err := ParseIngestionEngine(string(opts.IngestionEngine))
	if err != nil {
		return "", err
	}
	opts.IngestionEngine = parsedEngine

	bqFields := convertToBQFieldsWithOptions(fields, 0, opts)
	if opts.IncludeDLTMetadata {
		bqFields = appendDLTMetadataBQFields(bqFields)
	}
	bytes, err := json.MarshalIndent(bqFields, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed marshaling BigQuery schema JSON: %w", err)
	}
	return string(bytes), nil
}

func convertToBQFieldsWithOptions(fields []rawContractField, depth int, opts SchemaOptions) []BQFieldJSON {
	if depth > 100 {
		return nil
	}
	bqFields := make([]BQFieldJSON, len(fields))
	for i, f := range fields {
		origMode := normalizeBQMode(f.Mode)
		isVector := strings.EqualFold(strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(f.Type)), "DATA_TYPE_"), "VECTOR")
		if opts.IngestionEngine == IngestionEngineDLT && origMode == "REPEATED" {
			bqFields[i] = BQFieldJSON{
				Name:        f.Name,
				Type:        "JSON",
				Mode:        "NULLABLE",
				Description: f.Description,
				Fields:      nil,
			}
			continue
		}
		if isVector {
			bqFields[i] = BQFieldJSON{
				Name:        f.Name,
				Type:        "FLOAT",
				Mode:        "REPEATED",
				Description: f.Description,
			}
			continue
		}
		bq := BQFieldJSON{
			Name:        f.Name,
			Type:        normalizeBQType(f.Type),
			Mode:        origMode,
			Description: f.Description,
		}
		if len(f.Fields) > 0 {
			bq.Fields = convertToBQFieldsWithOptions(f.Fields, depth+1, opts)
		}
		bqFields[i] = bq
	}
	return bqFields
}

func transformSchemaJSONWithOptions(rawJSON string, opts SchemaOptions) (string, error) {
	parsedEngine, err := ParseIngestionEngine(string(opts.IngestionEngine))
	if err != nil {
		return "", err
	}
	opts.IngestionEngine = parsedEngine

	var fields []BQFieldJSON
	if err := json.Unmarshal([]byte(rawJSON), &fields); err != nil {
		return "", fmt.Errorf("failed parsing static schema JSON: %w", err)
	}
	transformed := transformBQFieldsWithOptions(fields, 0, opts)
	if opts.IncludeDLTMetadata {
		transformed = appendDLTMetadataBQFields(transformed)
	}
	outBytes, err := json.MarshalIndent(transformed, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed marshaling transformed BigQuery schema JSON: %w", err)
	}
	return string(outBytes), nil
}

func transformBQFieldsWithOptions(fields []BQFieldJSON, depth int, opts SchemaOptions) []BQFieldJSON {
	if depth > 100 {
		return nil
	}
	out := make([]BQFieldJSON, len(fields))
	for i, f := range fields {
		mode := normalizeBQMode(f.Mode)
		isVector := strings.EqualFold(strings.TrimSpace(f.Type), "FLOAT") && mode == "REPEATED"
		if opts.IngestionEngine == IngestionEngineDLT && mode == "REPEATED" && !isVector {
			out[i] = BQFieldJSON{
				Name:        f.Name,
				Type:        "JSON",
				Mode:        "NULLABLE",
				Description: f.Description,
				Fields:      nil,
			}
			continue
		}
		copied := f
		if len(f.Fields) > 0 {
			copied.Fields = transformBQFieldsWithOptions(f.Fields, depth+1, opts)
		}
		out[i] = copied
	}
	return out
}

func appendDLTMetadataBQFields(fields []BQFieldJSON) []BQFieldJSON {
	hasLoadID := false
	hasID := false
	for _, f := range fields {
		if strings.EqualFold(f.Name, "_dlt_load_id") {
			hasLoadID = true
		} else if strings.EqualFold(f.Name, "_dlt_id") {
			hasID = true
		}
	}
	if !hasLoadID {
		fields = append(fields, BQFieldJSON{
			Name:        "_dlt_load_id",
			Type:        "STRING",
			Mode:        "REQUIRED",
			Description: "DLT load identifier",
		})
	}
	if !hasID {
		fields = append(fields, BQFieldJSON{
			Name:        "_dlt_id",
			Type:        "STRING",
			Mode:        "REQUIRED",
			Description: "DLT row identifier",
		})
	}
	return fields
}

// ProtoFieldsToBigQueryFields converts Hub protobuf contract fields to BigQuery schema columns using opts.
func ProtoFieldsToBigQueryFields(fields []*hubv1.Field, opts SchemaOptions) ([]BQFieldJSON, error) {
	parsedEngine, err := ParseIngestionEngine(string(opts.IngestionEngine))
	if err != nil {
		return nil, err
	}
	opts.IngestionEngine = parsedEngine

	out, err := convertProtoFieldsToBQ(fields, nil, opts)
	if err != nil {
		return nil, err
	}
	if opts.IncludeDLTMetadata {
		out = appendDLTMetadataBQFields(out)
	}
	return out, nil
}

// ProtoFieldsToBigQueryJSON converts Hub protobuf contract fields to indented BigQuery schema JSON using opts.
func ProtoFieldsToBigQueryJSON(fields []*hubv1.Field, opts SchemaOptions) (string, error) {
	bqFields, err := ProtoFieldsToBigQueryFields(fields, opts)
	if err != nil {
		return "", err
	}
	bytes, err := json.MarshalIndent(bqFields, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed marshaling BigQuery schema JSON: %w", err)
	}
	return string(bytes), nil
}

func convertProtoFieldsToBQ(fields []*hubv1.Field, path []string, opts SchemaOptions) ([]BQFieldJSON, error) {
	if len(path) > 100 {
		return nil, fmt.Errorf("max recursion depth (%d) reached: %s", 100, strings.Join(path, " -> "))
	}
	out := make([]BQFieldJSON, 0, len(fields))
	for _, f := range fields {
		if f == nil {
			continue
		}
		origMode := mapProtoFieldMode(f.Mode)
		if opts.IngestionEngine == IngestionEngineDLT && origMode == "REPEATED" {
			out = append(out, BQFieldJSON{
				Name:        f.Name,
				Type:        "JSON",
				Mode:        "NULLABLE",
				Description: f.Description,
				Fields:      nil,
			})
			continue
		}
		if f.Type == hubv1.DataType_DATA_TYPE_VECTOR {
			out = append(out, BQFieldJSON{
				Name:        f.Name,
				Type:        "FLOAT",
				Mode:        "REPEATED",
				Description: f.Description,
			})
			continue
		}
		bqType, err := mapProtoFieldType(f.Type)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", f.Name, err)
		}
		bq := BQFieldJSON{
			Name:        f.Name,
			Type:        bqType,
			Mode:        origMode,
			Description: f.Description,
		}
		if len(f.Fields) > 0 {
			subFields, err := convertProtoFieldsToBQ(f.Fields, append(path, f.Name), opts)
			if err != nil {
				return nil, err
			}
			bq.Fields = subFields
		}
		out = append(out, bq)
	}
	return out, nil
}

func mapProtoFieldType(t hubv1.DataType) (string, error) {
	switch t {
	case hubv1.DataType_DATA_TYPE_STRING:
		return "STRING", nil
	case hubv1.DataType_DATA_TYPE_BYTES:
		return "BYTES", nil
	case hubv1.DataType_DATA_TYPE_INT64:
		return "INT64", nil
	case hubv1.DataType_DATA_TYPE_FLOAT64:
		return "FLOAT64", nil
	case hubv1.DataType_DATA_TYPE_BOOL:
		return "BOOL", nil
	case hubv1.DataType_DATA_TYPE_TIMESTAMP:
		return "TIMESTAMP", nil
	case hubv1.DataType_DATA_TYPE_DATE:
		return "DATE", nil
	case hubv1.DataType_DATA_TYPE_TIME:
		return "TIME", nil
	case hubv1.DataType_DATA_TYPE_DATETIME:
		return "DATETIME", nil
	case hubv1.DataType_DATA_TYPE_GEOGRAPHY:
		return "GEOGRAPHY", nil
	case hubv1.DataType_DATA_TYPE_NUMERIC:
		return "NUMERIC", nil
	case hubv1.DataType_DATA_TYPE_BIGNUMERIC:
		return "BIGNUMERIC", nil
	case hubv1.DataType_DATA_TYPE_STRUCT:
		return "RECORD", nil
	case hubv1.DataType_DATA_TYPE_JSON:
		return "JSON", nil
	case hubv1.DataType_DATA_TYPE_VECTOR:
		return "FLOAT", nil
	case hubv1.DataType_DATA_TYPE_UNSPECIFIED:
		return "", fmt.Errorf("data type is unspecified")
	default:
		return "", fmt.Errorf("unsupported or unknown hubv1.DataType: %d", t)
	}
}

func mapProtoFieldMode(m hubv1.FieldMode) string {
	switch m {
	case hubv1.FieldMode_FIELD_MODE_REQUIRED:
		return "REQUIRED"
	case hubv1.FieldMode_FIELD_MODE_REPEATED:
		return "REPEATED"
	case hubv1.FieldMode_FIELD_MODE_NULLABLE:
		return "NULLABLE"
	default:
		return "NULLABLE"
	}
}

func normalizeBQType(t string) string {
	t = strings.ToUpper(strings.TrimSpace(t))
	t = strings.TrimPrefix(t, "DATA_TYPE_")
	switch t {
	case "INT", "INTEGER", "INT32", "INT64":
		return "INT64"
	case "FLOAT", "DOUBLE", "FLOAT32", "FLOAT64":
		return "FLOAT64"
	case "VECTOR":
		return "FLOAT"
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

// CompiledOutputPort represents a compiled public output view and its associated contract metadata.
type CompiledOutputPort struct {
	Name            string `json:"name"`
	URN             string `json:"urn"`
	Lens            string `json:"lens"`
	Materialization string `json:"materialization"`
	ViewQuery       string `json:"view_query"`
	ViewSQL         string `json:"view_sql"`
}

// CompiledInputPort represents a compiled boundary input landing table and its physical schema.
type CompiledInputPort struct {
	Name       string `json:"name"`
	URN        string `json:"urn"`
	RecordName string `json:"record_name"`
	TableName  string `json:"table_name"`
	Source     string `json:"source"`
	SchemaJSON string `json:"schema_json"`
}

var createViewRegex = regexp.MustCompile(`(?is)^\s*CREATE\s+(?:OR\s+REPLACE\s+)?VIEW\s+.*?\s+AS\s+(.*)$`)

// ExtractViewQuery extracts the pure query body from a CREATE [OR REPLACE] VIEW DDL statement.
func ExtractViewQuery(viewSQL string) string {
	trimmed := strings.TrimSpace(viewSQL)
	if strings.HasPrefix(strings.ToUpper(trimmed), "SELECT") || strings.HasPrefix(strings.ToUpper(trimmed), "WITH") {
		return strings.TrimRight(trimmed, "; \t\r\n")
	}

	matches := createViewRegex.FindStringSubmatch(trimmed)
	if len(matches) > 1 {
		return strings.TrimRight(strings.TrimSpace(matches[1]), "; \t\r\n")
	}

	return strings.TrimRight(trimmed, "; \t\r\n")
}

// DiscoverLockDir locates the compiled .supercargo/lock directory.
// If explicitLockDir is non-empty, its existence is strictly verified.
// If omitted, candidate locations relative to manifestPath or workspace root are checked.
func DiscoverLockDir(manifestPath, productSlug, explicitLockDir string) (string, bool, error) {
	if explicitLockDir != "" {
		target := explicitLockDir
		if !filepath.IsAbs(target) && manifestPath != "" {
			target = filepath.Join(filepath.Dir(manifestPath), explicitLockDir)
		}
		target = filepath.Clean(target)
		if fi, err := os.Stat(target); err == nil && fi.IsDir() {
			return target, true, nil
		}
		if fi, err := os.Stat(filepath.Clean(explicitLockDir)); err == nil && fi.IsDir() {
			return filepath.Clean(explicitLockDir), true, nil
		}
		return "", false, fmt.Errorf("explicitly configured lock_dir %q does not exist", explicitLockDir)
	}

	manifestDir := "."
	if manifestPath != "" {
		manifestDir = filepath.Dir(manifestPath)
	}
	manifestDir = filepath.Clean(manifestDir)

	var searchDirs []string
	if productSlug != "" {
		searchDirs = append(searchDirs,
			filepath.Join(manifestDir, ".supercargo", "lock", productSlug),
		)
	}
	searchDirs = append(searchDirs,
		filepath.Join(manifestDir, ".supercargo", "lock"),
	)

	curr := manifestDir
	for i := 0; i < 4; i++ {
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
		if productSlug != "" {
			searchDirs = append(searchDirs, filepath.Join(curr, ".supercargo", "lock", productSlug))
		}
		searchDirs = append(searchDirs, filepath.Join(curr, ".supercargo", "lock"))
	}

	for _, cand := range searchDirs {
		candClean := filepath.Clean(cand)
		if fi, err := os.Stat(candClean); err == nil && fi.IsDir() {
			return candClean, true, nil
		}
	}

	return "", false, nil
}

// LoadCompiledOutputs reads compiled view SQL files and ODCS contracts from the lock directory.
func LoadCompiledOutputs(lockDir, productSlug string) (map[string]*CompiledOutputPort, error) {
	viewsDir := filepath.Join(lockDir, "views")
	if fi, err := os.Stat(viewsDir); err != nil || !fi.IsDir() {
		return make(map[string]*CompiledOutputPort), nil
	}

	type odpsPort struct {
		Name            string `yaml:"name"`
		Lens            string `yaml:"lens"`
		Materialization string `yaml:"materialization"`
	}
	type odpsDoc struct {
		ID          string     `yaml:"id"`
		OutputPorts []odpsPort `yaml:"output_ports"`
	}

	odpsPorts := make(map[string]odpsPort)
	odpsPath := filepath.Join(lockDir, "dataproduct.odps.yaml")
	if data, err := os.ReadFile(odpsPath); err == nil {
		var doc odpsDoc
		if err := yaml.Unmarshal(data, &doc); err == nil {
			for _, p := range doc.OutputPorts {
				odpsPorts[p.Name] = p
			}
		}
	}

	contractsDir := filepath.Join(lockDir, "contracts")
	contractURNs := make(map[string]string)
	if entries, err := os.ReadDir(contractsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasSuffix(name, ".odcs.yaml") || strings.HasSuffix(name, ".yaml") {
				portName := strings.TrimSuffix(strings.TrimSuffix(name, ".odcs.yaml"), ".yaml")
				cData, err := os.ReadFile(filepath.Join(contractsDir, name))
				if err == nil {
					var raw struct {
						ID   string `yaml:"id"`
						Meta struct {
							URN string `yaml:"urn"`
							ID  string `yaml:"id"`
						} `yaml:"meta"`
					}
					if err := yaml.Unmarshal(cData, &raw); err == nil {
						urn := raw.ID
						if urn == "" {
							urn = raw.Meta.URN
						}
						if urn == "" {
							urn = raw.Meta.ID
						}
						if urn != "" {
							contractURNs[portName] = urn
						}
					}
				}
			}
		}
	}

	entries, err := os.ReadDir(viewsDir)
	if err != nil {
		return nil, fmt.Errorf("reading views directory %q: %w", viewsDir, err)
	}

	outputs := make(map[string]*CompiledOutputPort, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		portName := strings.TrimSuffix(e.Name(), ".sql")
		filePath := filepath.Join(viewsDir, e.Name())
		sqlBytes, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("reading view file %q: %w", filePath, err)
		}

		viewSQL := string(sqlBytes)
		viewQuery := ExtractViewQuery(viewSQL)

		lens := ""
		mat := "view"
		if op, ok := odpsPorts[portName]; ok {
			lens = op.Lens
			if op.Materialization != "" {
				mat = op.Materialization
			}
		}
		if lens == "" {
			lens = inferLensFromName(portName)
		}

		urn := contractURNs[portName]
		if urn == "" {
			urn = fmt.Sprintf("urn:supercargo:hub:contract:%s-%s:v1", productSlug, portName)
		}

		mapKey := urn
		if _, exists := outputs[mapKey]; exists {
			return nil, fmt.Errorf("duplicate collision-resistant collection key %q", mapKey)
		}

		outputs[mapKey] = &CompiledOutputPort{
			Name:            portName,
			URN:             urn,
			Lens:            lens,
			Materialization: mat,
			ViewQuery:       viewQuery,
			ViewSQL:         viewSQL,
		}
	}

	return outputs, nil
}

func inferLensFromName(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, "_replica"):
		return "replica"
	case strings.HasSuffix(lower, "_event_log"):
		return "event_log"
	case strings.HasSuffix(lower, "_scd2"):
		return "scd2"
	case strings.HasSuffix(lower, "_cdc"):
		return "cdc"
	case strings.HasSuffix(lower, "_reidentified"):
		return "reidentified"
	case strings.HasSuffix(lower, "_v1") || strings.HasSuffix(lower, "@v1"):
		return "bridge"
	default:
		return "replica"
	}
}

// LoadCompiledInputs reads ingress contracts and BigQuery schemas from the lock directory.
func LoadCompiledInputs(lockDir, productSlug string) (map[string]*CompiledInputPort, error) {
	schemasDir := filepath.Join(lockDir, "schemas")
	ingressDir := filepath.Join(lockDir, "ingress")

	inputs := make(map[string]*CompiledInputPort)

	type odpsInputPort struct {
		Name   string `yaml:"name"`
		Source string `yaml:"source"`
	}
	type odpsDoc struct {
		InputPorts []odpsInputPort `yaml:"input_ports"`
	}

	odpsInputs := make(map[string]odpsInputPort)
	odpsPath := filepath.Join(lockDir, "dataproduct.odps.yaml")
	if data, err := os.ReadFile(odpsPath); err == nil {
		var doc odpsDoc
		if err := yaml.Unmarshal(data, &doc); err == nil {
			for _, in := range doc.InputPorts {
				odpsInputs[in.Name] = in
			}
		}
	}

	type ingressContractInfo struct {
		URN        string
		RecordName string
	}
	ingressInfo := make(map[string]ingressContractInfo)
	if entries, err := os.ReadDir(ingressDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || (!strings.HasSuffix(e.Name(), ".contract.yaml") && !strings.HasSuffix(e.Name(), ".yaml")) {
				continue
			}
			portName := strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".contract.yaml"), ".yaml")
			cData, err := os.ReadFile(filepath.Join(ingressDir, e.Name()))
			if err == nil {
				var raw struct {
					Meta struct {
						URN       string `yaml:"urn"`
						ID        string `yaml:"id"`
						DataAsset string `yaml:"data_asset"`
					} `yaml:"meta"`
					ID         string `yaml:"id"`
					RecordName string `yaml:"record_name"`
					TableName  string `yaml:"table_name"`
				}
				_ = yaml.Unmarshal(cData, &raw)
				urn := raw.ID
				if urn == "" {
					urn = raw.Meta.URN
				}
				if urn == "" {
					urn = raw.Meta.ID
				}
				recName := raw.RecordName
				if recName == "" {
					recName = raw.TableName
				}
				if recName == "" {
					recName = raw.Meta.DataAsset
				}
				ingressInfo[portName] = ingressContractInfo{
					URN:        urn,
					RecordName: recName,
				}
			}
		}
	}

	schemaFiles := make(map[string]string)
	if entries, err := os.ReadDir(schemasDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".schema.json") {
				continue
			}
			base := strings.TrimSuffix(e.Name(), ".schema.json")
			recordName := base
			prefix := fmt.Sprintf("%s_internal.", productSlug)
			if strings.HasPrefix(base, prefix) {
				recordName = strings.TrimPrefix(base, prefix)
			}
			sBytes, err := os.ReadFile(filepath.Join(schemasDir, e.Name()))
			if err == nil {
				schemaFiles[recordName] = string(sBytes)
				schemaFiles[base] = string(sBytes)
			}
		}
	}

	schemaKeys := make([]string, 0, len(schemaFiles))
	for k := range schemaFiles {
		schemaKeys = append(schemaKeys, k)
	}
	sort.Strings(schemaKeys)

	if len(ingressInfo) > 0 {
		for portName, info := range ingressInfo {
			recordName := info.RecordName
			if recordName == "" {
				if _, ok := schemaFiles[portName]; ok {
					recordName = portName
				} else {
					for _, rec := range schemaKeys {
						if !strings.Contains(rec, "_internal.") {
							recordName = rec
							break
						}
					}
				}
			}
			if recordName == "" {
				recordName = portName
			}
			schemaJSON := schemaFiles[recordName]
			if schemaJSON == "" {
				schemaJSON = schemaFiles[fmt.Sprintf("%s_internal.%s", productSlug, recordName)]
			}
			if schemaJSON == "" {
				for _, k := range schemaKeys {
					schemaJSON = schemaFiles[k]
					break
				}
			}

			urn := info.URN
			if urn == "" {
				urn = fmt.Sprintf("urn:supercargo:hub:contract:%s-internal-%s:v1", productSlug, portName)
			}

			source := "managed"
			if op, ok := odpsInputs[portName]; ok && op.Source != "" {
				source = op.Source
			}

			mapKey := urn
			if _, exists := inputs[mapKey]; exists {
				return nil, fmt.Errorf("duplicate collision-resistant collection key %q", mapKey)
			}

			inputs[mapKey] = &CompiledInputPort{
				Name:       portName,
				URN:        urn,
				RecordName: recordName,
				TableName:  recordName,
				Source:     source,
				SchemaJSON: schemaJSON,
			}
		}
	} else if len(schemaFiles) > 0 {
		seen := make(map[string]struct{})
		for recName, schemaJSON := range schemaFiles {
			if strings.Contains(recName, "_internal.") {
				continue
			}
			if _, ok := seen[recName]; ok {
				continue
			}
			seen[recName] = struct{}{}

			urn := fmt.Sprintf("urn:supercargo:hub:contract:%s-internal-%s:v1", productSlug, recName)
			source := "managed"
			if op, ok := odpsInputs[recName]; ok && op.Source != "" {
				source = op.Source
			}

			mapKey := urn
			if _, exists := inputs[mapKey]; exists {
				return nil, fmt.Errorf("duplicate collision-resistant collection key %q", mapKey)
			}

			inputs[mapKey] = &CompiledInputPort{
				Name:       recName,
				URN:        urn,
				RecordName: recName,
				TableName:  recName,
				Source:     source,
				SchemaJSON: schemaJSON,
			}
		}
	}

	return inputs, nil
}
