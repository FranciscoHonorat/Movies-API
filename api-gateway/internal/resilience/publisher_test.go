package resilience_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/FranciscoHonorat/movies/api-gateway/internal/resilience"
	"github.com/FranciscoHonorat/movies/shared"
	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errBroker = errors.New("conexão com o rabbitmq perdida")

type fakePublisher struct {
	mu       sync.Mutex
	calls    int
	behavior func(call int) error
}

func (f *fakePublisher) Publish(context.Context, shared.MoviePublisherMessage) error {
	f.mu.Lock()
	f.calls++
	n := f.calls
	f.mu.Unlock()
	return f.behavior(n)
}

func (f *fakePublisher) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestPublisher_RetriesTransientFailureThenSucceeds(t *testing.T) {
	fake := &fakePublisher{behavior: func(call int) error {
		if call < 2 {
			return errBroker
		}
		return nil
	}}
	pub := resilience.NewPublisher(fake)

	err := pub.Publish(context.Background(), shared.MoviePublisherMessage{Title: "Tenet", Year: "2020"})

	require.NoError(t, err)
	assert.Equal(t, 2, fake.callCount())
}

func TestPublisher_BreakerOpensAndStopsCallingInner(t *testing.T) {
	fake := &fakePublisher{behavior: func(int) error { return errBroker }}
	pub := resilience.NewPublisher(fake)

	for i := 0; i < 2; i++ {
		_ = pub.Publish(context.Background(), shared.MoviePublisherMessage{Title: "x", Year: "2020"})
	}
	callsWhenTripped := fake.callCount()
	assert.LessOrEqual(t, callsWhenTripped, 5)

	for i := 0; i < 10; i++ {
		err := pub.Publish(context.Background(), shared.MoviePublisherMessage{Title: "x", Year: "2020"})
		require.ErrorIs(t, err, gobreaker.ErrOpenState)
	}
	assert.Equal(t, callsWhenTripped, fake.callCount(), "com o breaker aberto, o publisher interno não deveria ser chamado de novo")
}
