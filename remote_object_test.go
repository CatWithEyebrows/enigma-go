package enigma

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestRemoteObject(t *testing.T) {

	// Create emitter, register listener
	object := newRemoteObject(nil, nil)
	channel := object.ChangedChannel()

	// Send an event
	object.signalChanged()

	// Receive the change
	<-channel

	// Unregister the listener
	object.RemoveChangeChannel(channel)
	assert.Empty(t, object.changedChannels)

	// Check that the changed channel is closed
	_, isOpen := <-channel
	assert.False(t, isOpen)

	// Check that we are still not closed
	select {
	case <-object.Closed():
		assert.Fail(t, "Should lock!")
	default:
	}

	// Close the object
	object.signalClosed()

	// Make sure we're closed now
	_, isOpen = <-object.Closed()
	assert.False(t, isOpen)

}

func TestRemoteObjectChangedChannelConcurrently(t *testing.T) {
	object := newRemoteObject(nil, nil)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			object.signalChanged()
		}
		close(done)
	}()
	for i := 0; i < 100; i++ {
		object.RemoveChangeChannel(object.ChangedChannel())
	}
	<-done
}

func TestRemoteObjectSignalChangedDoesNotBlock(t *testing.T) {
	object := newRemoteObject(nil, nil)
	channel := object.ChangedChannel()
	done := make(chan struct{})
	go func() {
		// More changes than the channel buffers, with nobody reading
		for i := 0; i < 100; i++ {
			object.signalChanged()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("signalChanged blocked on a full channel")
	}
	assert.Len(t, channel, cap(channel))
}
