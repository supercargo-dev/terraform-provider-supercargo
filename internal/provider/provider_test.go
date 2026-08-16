package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvider_Version(t *testing.T) {
	expectedVersion := "1.2.3"
	providerFunc := New(expectedVersion)
	require.NotNil(t, providerFunc)

	p := providerFunc()
	require.NotNil(t, p)

	sp, ok := p.(*SupercargoProvider)
	require.True(t, ok)
	assert.Equal(t, expectedVersion, sp.version)
	assert.NotNil(t, sp.factory)

	var metaResp provider.MetadataResponse
	p.Metadata(context.Background(), provider.MetadataRequest{}, &metaResp)
	assert.Equal(t, "supercargo", metaResp.TypeName)
	assert.Equal(t, expectedVersion, metaResp.Version)
}

func TestNewProvider_ResourcesAndDataSources(t *testing.T) {
	p := New("test")()
	require.NotNil(t, p)

	ctx := context.Background()
	resources := p.Resources(ctx)
	assert.NotEmpty(t, resources)
	assert.Len(t, resources, 5)

	dataSources := p.DataSources(ctx)
	assert.NotEmpty(t, dataSources)
	assert.Len(t, dataSources, 1)
}
