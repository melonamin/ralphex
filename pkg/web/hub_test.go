package web

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/ralphex/pkg/processor"
)

func TestNewHub(t *testing.T) {
	h := NewHub()
	assert.NotNil(t, h)
	assert.Equal(t, 0, h.ClientCount())
}

func TestHub_Subscribe(t *testing.T) {
	h := NewHub()

	ch, err := h.Subscribe()
	require.NoError(t, err)
	assert.NotNil(t, ch)
	assert.Equal(t, 1, h.ClientCount())

	// subscribe another
	ch2, err := h.Subscribe()
	require.NoError(t, err)
	assert.NotNil(t, ch2)
	assert.Equal(t, 2, h.ClientCount())
}

func TestHub_Unsubscribe(t *testing.T) {
	h := NewHub()

	ch, err := h.Subscribe()
	require.NoError(t, err)
	assert.Equal(t, 1, h.ClientCount())

	h.Unsubscribe(ch)
	assert.Equal(t, 0, h.ClientCount())

	// channel should be closed
	_, open := <-ch
	assert.False(t, open)
}

func TestHub_Unsubscribe_SafeForMultipleCalls(t *testing.T) {
	h := NewHub()
	ch, err := h.Subscribe()
	require.NoError(t, err)

	// first unsubscribe
	h.Unsubscribe(ch)

	// second unsubscribe should not panic
	assert.NotPanics(t, func() {
		h.Unsubscribe(ch)
	})
}

func TestHub_Broadcast(t *testing.T) {
	h := NewHub()

	ch1, err := h.Subscribe()
	require.NoError(t, err)
	ch2, err := h.Subscribe()
	require.NoError(t, err)

	event := NewOutputEvent(processor.PhaseTask, "test message")
	h.Broadcast(event)

	// both clients should receive the event
	select {
	case e := <-ch1:
		assert.Equal(t, "test message", e.Text)
	case <-time.After(time.Second):
		t.Fatal("ch1 did not receive event")
	}

	select {
	case e := <-ch2:
		assert.Equal(t, "test message", e.Text)
	case <-time.After(time.Second):
		t.Fatal("ch2 did not receive event")
	}
}

func TestHub_Broadcast_DropsForFullClient(t *testing.T) {
	h := NewHub()

	ch, err := h.Subscribe()
	require.NoError(t, err)

	// fill the channel buffer (256 events)
	for range 300 {
		h.Broadcast(NewOutputEvent(processor.PhaseTask, "event"))
	}

	// should not block, some events were dropped
	// drain the channel
	count := 0
	timeout := time.After(time.Second)
drainLoop:
	for {
		select {
		case <-ch:
			count++
		case <-timeout:
			break drainLoop
		default:
			break drainLoop
		}
	}

	// should have received up to buffer size (256)
	assert.LessOrEqual(t, count, 256)
}

func TestHub_ClientCount(t *testing.T) {
	h := NewHub()

	assert.Equal(t, 0, h.ClientCount())

	ch1, err := h.Subscribe()
	require.NoError(t, err)
	assert.Equal(t, 1, h.ClientCount())

	ch2, err := h.Subscribe()
	require.NoError(t, err)
	assert.Equal(t, 2, h.ClientCount())

	h.Unsubscribe(ch1)
	assert.Equal(t, 1, h.ClientCount())

	h.Unsubscribe(ch2)
	assert.Equal(t, 0, h.ClientCount())
}

func TestHub_Close(t *testing.T) {
	h := NewHub()

	ch1, err := h.Subscribe()
	require.NoError(t, err)
	ch2, err := h.Subscribe()
	require.NoError(t, err)
	ch3, err := h.Subscribe()
	require.NoError(t, err)

	assert.Equal(t, 3, h.ClientCount())

	h.Close()

	assert.Equal(t, 0, h.ClientCount())

	// all channels should be closed
	_, open1 := <-ch1
	_, open2 := <-ch2
	_, open3 := <-ch3
	assert.False(t, open1)
	assert.False(t, open2)
	assert.False(t, open3)
}

func TestHub_Concurrency(t *testing.T) {
	h := NewHub()
	var wg sync.WaitGroup

	// concurrent subscribes
	channels := make([]chan Event, 0, 20)
	var chMu sync.Mutex

	for range 20 {
		wg.Go(func() {
			ch, err := h.Subscribe()
			if err == nil {
				chMu.Lock()
				channels = append(channels, ch)
				chMu.Unlock()
			}
		})
	}

	wg.Wait()
	require.Equal(t, 20, h.ClientCount())

	// concurrent broadcasts
	for range 10 {
		wg.Go(func() {
			for range 10 {
				h.Broadcast(NewOutputEvent(processor.PhaseTask, "event"))
			}
		})
	}

	// concurrent unsubscribes
	for i := range 10 {
		n := i
		wg.Go(func() {
			chMu.Lock()
			if n < len(channels) {
				ch := channels[n]
				chMu.Unlock()
				h.Unsubscribe(ch)
			} else {
				chMu.Unlock()
			}
		})
	}

	wg.Wait()

	// should not panic, client count should be reduced
	count := h.ClientCount()
	assert.GreaterOrEqual(t, count, 0)
}

func TestHub_BroadcastToNoClients(t *testing.T) {
	h := NewHub()

	// should not panic
	assert.NotPanics(t, func() {
		h.Broadcast(NewOutputEvent(processor.PhaseTask, "nobody listening"))
	})
}

func TestHub_Subscribe_MaxClientsExceeded(t *testing.T) {
	h := NewHub()

	// subscribe up to the limit
	channels := make([]chan Event, 0, MaxClients)
	for range MaxClients {
		ch, err := h.Subscribe()
		require.NoError(t, err)
		channels = append(channels, ch)
	}

	assert.Equal(t, MaxClients, h.ClientCount())

	// next subscribe should fail
	ch, err := h.Subscribe()
	require.ErrorIs(t, err, ErrMaxClientsExceeded)
	assert.Nil(t, ch)

	// client count should not change
	assert.Equal(t, MaxClients, h.ClientCount())

	// unsubscribe one client
	h.Unsubscribe(channels[0])
	assert.Equal(t, MaxClients-1, h.ClientCount())

	// now subscribe should work again
	ch, err = h.Subscribe()
	require.NoError(t, err)
	assert.NotNil(t, ch)
	assert.Equal(t, MaxClients, h.ClientCount())
}

func TestHub_DroppedEvents(t *testing.T) {
	h := NewHub()

	// initially no dropped events
	assert.Equal(t, int64(0), h.DroppedEvents())

	ch, err := h.Subscribe()
	require.NoError(t, err)
	defer h.Unsubscribe(ch)

	// fill the channel buffer (256 events) and send more to trigger drops
	for range 300 {
		h.Broadcast(NewOutputEvent(processor.PhaseTask, "event"))
	}

	// some events should have been dropped (300 - 256 = 44 minimum)
	dropped := h.DroppedEvents()
	assert.GreaterOrEqual(t, dropped, int64(44), "expected at least 44 dropped events")
}
