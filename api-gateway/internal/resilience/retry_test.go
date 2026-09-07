package resilience_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FranciscoHonorat/movies/api-gateway/internal/resilience"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errBoom = errors.New("falha transitória")

func alwaysRetryable(error) bool { return true }
func neverRetryable(error) bool  { return false }

func TestRetry_SucceedsFirstTry(t *testing.T) {
	calls := 0
	result, err := resilience.Retry(context.Background(), resilience.DefaultRetryPolicy(), alwaysRetryable, func() (string, error) {
		calls++
		return "ok", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.Equal(t, 1, calls)
}

func TestRetry_SucceedsAfterRetryableFailures(t *testing.T) {
	calls := 0
	policy := resilience.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}

	result, err := resilience.Retry(context.Background(), policy, alwaysRetryable, func() (string, error) {
		calls++
		if calls < 3 {
			return "", errBoom
		}
		return "ok", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	assert.Equal(t, 3, calls, "deveria ter tentado até a 3ª chamada suceder")
}

func TestRetry_ExhaustsAttemptsAndReturnsLastError(t *testing.T) {
	calls := 0
	policy := resilience.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}

	_, err := resilience.Retry(context.Background(), policy, alwaysRetryable, func() (string, error) {
		calls++
		return "", errBoom
	})

	require.ErrorIs(t, err, errBoom)
	assert.Equal(t, 3, calls, "não deveria tentar mais que MaxAttempts vezes")
}

func TestRetry_StopsImmediatelyOnNonRetryableError(t *testing.T) {
	calls := 0
	result, err := resilience.Retry(context.Background(), resilience.DefaultRetryPolicy(), neverRetryable, func() (string, error) {
		calls++
		return "", errBoom
	})

	require.ErrorIs(t, err, errBoom)
	assert.Empty(t, result)
	assert.Equal(t, 1, calls, "erro não-retryable não deveria gerar nenhuma nova tentativa")
}

func TestRetry_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	policy := resilience.RetryPolicy{MaxAttempts: 5, BaseDelay: 50 * time.Millisecond, MaxDelay: 200 * time.Millisecond}

	calls := 0
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := resilience.Retry(ctx, policy, alwaysRetryable, func() (string, error) {
		calls++
		return "", errBoom
	})

	require.ErrorIs(t, err, context.Canceled)
	assert.Less(t, calls, 5, "cancelar o contexto deveria interromper antes de esgotar as tentativas")
}

func TestNoRetryPolicy_CallsExactlyOnce(t *testing.T) {
	calls := 0
	_, err := resilience.Retry(context.Background(), resilience.NoRetryPolicy(), alwaysRetryable, func() (string, error) {
		calls++
		return "", errBoom
	})

	require.ErrorIs(t, err, errBoom)
	assert.Equal(t, 1, calls)
}
