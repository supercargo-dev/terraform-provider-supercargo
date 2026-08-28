package manifest

import (
	"encoding/json"
	"fmt"
	"os"

	hubv1 "github.com/supercargo-dev/core/gen/go/hub/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v3"
)

// Load reads a YAML manifest file and unmarshals it into a protobuf message.
func Load(path string) (*hubv1.ProductManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file: %w", err)
	}

	// 1. Unmarshal YAML to generic map
	var raw interface{}
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
