package enigma

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeLayouts returns the layouts 1, 2, 3, ... and can be told to fail or block
type fakeLayouts struct {
	fetches chan int
	count   int
	errs    map[int]error
	// called during a fetch, before it returns
	during map[int]func()
}

func newFakeLayouts() *fakeLayouts {
	return &fakeLayouts{fetches: make(chan int, 100), errs: map[int]error{}, during: map[int]func(){}}
}

func (f *fakeLayouts) get(ctx context.Context) (int, error) {
	f.count++
	f.fetches <- f.count
	if during := f.during[f.count]; during != nil {
		during()
	}
	return f.count, f.errs[f.count]
}

func receive[T any](t *testing.T, updates <-chan LayoutUpdate[T]) LayoutUpdate[T] {
	t.Helper()
	select {
	case update, ok := <-updates:
		require.True(t, ok, "channel closed")
		return update
	case <-time.After(time.Second):
		t.Fatal("no update")
	}
	return LayoutUpdate[T]{}
}

func assertClosed[T any](t *testing.T, updates <-chan LayoutUpdate[T]) {
	t.Helper()
	select {
	case _, ok := <-updates:
		assert.False(t, ok, "channel still open")
	case <-time.After(time.Second):
		t.Fatal("channel not closed")
	}
}

// waitForFetch waits until the watcher has fetched layout n
func waitForFetch(t *testing.T, f *fakeLayouts, n int) {
	t.Helper()
	for {
		select {
		case got := <-f.fetches:
			if got == n {
				return
			}
		case <-time.After(time.Second):
			t.Fatalf("layout %d not fetched", n)
		}
	}
}

func TestWatchLayout(t *testing.T) {
	obj := newRemoteObject(nil, nil)
	layouts := newFakeLayouts()
	ctx, cancel := context.WithCancel(context.Background())
	updates := WatchLayout(ctx, obj, layouts.get)

	assert.Equal(t, LayoutUpdate[int]{Layout: 1}, receive(t, updates))
	obj.signalChanged()
	assert.Equal(t, LayoutUpdate[int]{Layout: 2}, receive(t, updates))

	cancel()
	assertClosed(t, updates)
	assert.Empty(t, obj.changedChannels, "the change channel is removed")
}

func TestWatchLayoutObjectClosed(t *testing.T) {
	obj := newRemoteObject(nil, nil)
	updates := WatchLayout(context.Background(), obj, newFakeLayouts().get)
	receive(t, updates)
	obj.signalClosed()
	assertClosed(t, updates)
}

func TestWatchLayoutErrors(t *testing.T) {
	obj := newRemoteObject(nil, nil)
	layouts := newFakeLayouts()
	failed := errors.New("failed")
	layouts.errs[1] = failed
	layouts.errs[2] = &qixError{ErrorCode: errorCodeAborted}
	updates := WatchLayout(context.Background(), obj, layouts.get)

	assert.Equal(t, LayoutUpdate[int]{Layout: 1, Err: failed}, receive(t, updates))
	obj.signalChanged()
	waitForFetch(t, layouts, 2)
	// The aborted layout 2 is not sent
	obj.signalChanged()
	assert.Equal(t, LayoutUpdate[int]{Layout: 3}, receive(t, updates))
}

func TestWatchLayoutChangeDuringFetch(t *testing.T) {
	obj := newRemoteObject(nil, nil)
	layouts := newFakeLayouts()
	layouts.during[1] = obj.signalChanged
	updates := WatchLayout(context.Background(), obj, layouts.get)

	// Layout 1 is outdated before it is read, and the change triggers fetching layout 2
	waitForFetch(t, layouts, 2)
	assert.Equal(t, LayoutUpdate[int]{Layout: 2}, receive(t, updates))
}

func TestWatchLayoutCoalescesChanges(t *testing.T) {
	obj := newRemoteObject(nil, nil)
	layouts := newFakeLayouts()
	updates := WatchLayout(context.Background(), obj, layouts.get)

	waitForFetch(t, layouts, 1)
	for i := 0; i < 5; i++ {
		obj.signalChanged()
	}
	// Nobody read layout 1 before the changes, so it is dropped. The five changes need one or two
	// more fetches, depending on how many were queued when the watcher drained the channel.
	update := receive(t, updates)
	assert.Contains(t, []int{2, 3}, update.Layout)
}
