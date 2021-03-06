package origin

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cloudflare/cloudflared/connection"
	"github.com/cloudflare/cloudflared/logger"
	"github.com/stretchr/testify/assert"
)

type dynamicMockFetcher struct {
	percentage int32
	err        error
}

func (dmf *dynamicMockFetcher) fetch() connection.PercentageFetcher {
	return func() (int32, error) {
		if dmf.err != nil {
			return 0, dmf.err
		}
		return dmf.percentage, nil
	}
}
func TestWaitForBackoffFallback(t *testing.T) {
	maxRetries := uint(3)
	backoff := BackoffHandler{
		MaxRetries: maxRetries,
		BaseTime:   time.Millisecond * 10,
	}
	ctx := context.Background()
	logger, err := logger.New()
	assert.NoError(t, err)
	resolveTTL := time.Duration(0)
	namedTunnel := &connection.NamedTunnelConfig{
		Credentials: connection.Credentials{
			AccountTag: "test-account",
		},
	}
	mockFetcher := dynamicMockFetcher{
		percentage: 0,
	}
	protocolSelector, err := connection.NewProtocolSelector(connection.HTTP2.String(), namedTunnel, mockFetcher.fetch(), resolveTTL, logger)
	assert.NoError(t, err)
	config := &TunnelConfig{
		Logger:           logger,
		ProtocolSelector: protocolSelector,
	}
	connIndex := uint8(1)

	initProtocol := protocolSelector.Current()
	assert.Equal(t, connection.HTTP2, initProtocol)

	protocallFallback := &protocallFallback{
		backoff,
		initProtocol,
		false,
	}

	// Retry #0 and #1. At retry #2, we switch protocol, so the fallback loop has one more retry than this
	for i := 0; i < int(maxRetries-1); i++ {
		err := waitForBackoff(ctx, protocallFallback, config, connIndex, fmt.Errorf("Some error"))
		assert.NoError(t, err)
		assert.Equal(t, initProtocol, protocallFallback.protocol)
	}

	// Retry fallback protocol
	for i := 0; i < int(maxRetries); i++ {
		err := waitForBackoff(ctx, protocallFallback, config, connIndex, fmt.Errorf("Some error"))
		assert.NoError(t, err)
		fallback, ok := protocolSelector.Fallback()
		assert.True(t, ok)
		assert.Equal(t, fallback, protocallFallback.protocol)
	}

	currentGlobalProtocol := protocolSelector.Current()
	assert.Equal(t, initProtocol, currentGlobalProtocol)

	// No protocol to fallback, return error
	err = waitForBackoff(ctx, protocallFallback, config, connIndex, fmt.Errorf("Some error"))
	assert.Error(t, err)

	protocallFallback.reset()
	err = waitForBackoff(ctx, protocallFallback, config, connIndex, fmt.Errorf("New error"))
	assert.NoError(t, err)
	assert.Equal(t, initProtocol, protocallFallback.protocol)
}
