package manifest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	if manifest == nil || (len(manifest.OutputPorts) == 0 && len(manifest.InputPorts) == 0) {
		return make(map[string]*ResolvedContract), nil
	}

	manifestClean := filepath.Clean(manifestPath)
	manifestDir := filepath.Dir(manifestClean)
	totalPorts := len(manifest.OutputPorts) + len(manifest.InputPorts)
	resolved := make(map[string]*ResolvedContract, totalPorts)

	type portRef struct {
		name     string
		contract *hubv1.ContractPointer
	}
	ports := make([]portRef, 0, totalPorts)
	for _, port := range manifest.OutputPorts {
		if port != nil && port.Contract != nil {
			ports = append(ports, portRef{name: port.Name, contract: port.Contract})
		}
	}
	for _, port := range manifest.InputPorts {
		if port != nil && port.Contract != nil {
			ports = append(ports, portRef{name: port.Name, contract: port.Contract})
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

	if len(ingressInfo) > 0 {
		for portName, info := range ingressInfo {
			recordName := info.RecordName
			if recordName == "" {
				if _, ok := schemaFiles[portName]; ok {
					recordName = portName
				} else {
					for rec := range schemaFiles {
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
				for _, sj := range schemaFiles {
					schemaJSON = sj
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

