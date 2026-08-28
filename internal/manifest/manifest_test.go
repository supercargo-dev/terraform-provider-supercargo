package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	t.Run("success loading valid YAML manifest", func(t *testing.T) {
		content := `meta:
  urn: urn:supercargo:hub:product:test-prod
  version: v1.0.0
  owner:
    team_name: core-team
output_ports:
  - name: main
    urn: urn:supercargo:hub:port:main
`
		tmpPath := filepath.Join(t.TempDir(), "product.yaml")
		err := os.WriteFile(tmpPath, []byte(content), 0644)
		require.NoError(t, err)

		m, err := Load(tmpPath)
		require.NoError(t, err)
		require.NotNil(t, m)
		require.NotNil(t, m.Meta)
		assert.Equal(t, "urn:supercargo:hub:product:test-prod", m.Meta.Urn)
		assert.Equal(t, "v1.0.0", m.Meta.Version)
		require.NotNil(t, m.Meta.Owner)
		assert.Equal(t, "core-team", m.Meta.Owner.TeamName)
		require.Len(t, m.OutputPorts, 1)
		assert.Equal(t, "main", m.OutputPorts[0].Name)
	})

	t.Run("missing file returns error", func(t *testing.T) {
		m, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
		require.Error(t, err)
		assert.Nil(t, m)
		assert.Contains(t, err.Error(), "failed to read manifest file")
	})

	t.Run("invalid YAML returns error", func(t *testing.T) {
		tmpPath := filepath.Join(t.TempDir(), "invalid.yaml")
		err := os.WriteFile(tmpPath, []byte("invalid: yaml: : :"), 0644)
		require.NoError(t, err)

		m, err := Load(tmpPath)
		require.Error(t, err)
		assert.Nil(t, m)
		assert.Contains(t, err.Error(), "failed to unmarshal YAML")
	})
}
