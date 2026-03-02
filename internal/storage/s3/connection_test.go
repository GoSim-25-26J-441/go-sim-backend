package s3

import (
	"context"
	"testing"

	"github.com/GoSim-25-26J-441/go-sim-backend/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConnectionIfConfigured_ReturnsNilWhenDisabled(t *testing.T) {
	ctx := context.Background()

	// Nil config
	client, err := NewConnectionIfConfigured(ctx, nil)
	require.NoError(t, err)
	assert.Nil(t, client)

	// Empty bucket
	client, err = NewConnectionIfConfigured(ctx, &config.S3Config{Bucket: ""})
	require.NoError(t, err)
	assert.Nil(t, client)
}

func TestNewConnection_ReturnsErrorWhenBucketEmpty(t *testing.T) {
	ctx := context.Background()

	_, err := NewConnection(ctx, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "S3 bucket is required")

	_, err = NewConnection(ctx, &config.S3Config{Bucket: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "S3 bucket is required")
}
