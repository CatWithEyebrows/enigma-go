package enigma

import (
	"context"
	"sync"

	"github.com/goccy/go-json"
)

type (
	// RemoteObject represents an object inside Qlik Associative Engine.
	RemoteObject struct {
		*ObjectInterface
		*session
		mutex           sync.Mutex
		changedChannels map[chan struct{}]bool
		closedCh        chan struct{}
		// undo group state when this is the UndoInfo session object, see UndoInfoObject
		undoGroups *undoGroupState
		// modal selection state when this is a Doc, see Doc.ModalSelection
		modalSelection *ModalSelection
	}
)

// ChangedChannel returns a channel that will receive changes when the underlying object is invalidated.
// When the channel's buffer is full, further changes are not sent until it is read, since the buffered
// ones already say that the object changed.
func (r *RemoteObject) ChangedChannel() chan struct{} {
	channel := make(chan struct{}, 16)
	r.mutex.Lock()
	r.changedChannels[channel] = true
	r.mutex.Unlock()
	return channel
}

// RemoveChangeChannel unregisters a channel from further events.
func (r *RemoteObject) RemoveChangeChannel(channel chan struct{}) {
	r.mutex.Lock()
	if r.changedChannels[channel] {
		delete(r.changedChannels, channel)
		close(channel)
	}
	r.mutex.Unlock()
}

// Closed returns a channel that is closed when the remote object in Qlik Associative Engine is closed
func (r *RemoteObject) Closed() chan struct{} {
	return r.closedCh
}

func (r *RemoteObject) signalChanged() {
	r.mutex.Lock()
	for channel := range r.changedChannels {
		// Never block, this runs on the goroutine that reads from the socket
		select {
		case channel <- struct{}{}:
		default:
		}
	}
	r.mutex.Unlock()
}

func (r *RemoteObject) signalClosed() {
	r.mutex.Lock()
	close(r.closedCh)
	for channel := range r.changedChannels {
		if r.changedChannels[channel] {
			delete(r.changedChannels, channel)
			close(channel)
		}
	}
	// Clear it
	r.changedChannels = make(map[chan struct{}]bool)
	r.mutex.Unlock()
}

// RPC invokes a method on the remote object. Not intended to be used directly but rather from generated schema code.
func (r *RemoteObject) RPC(ctx context.Context, method string, apiResponse interface{}, params ...interface{}) error {
	invocationResponse := r.interceptorChain(ctx, &Invocation{RemoteObject: r, Method: method, Params: ensureAllEncodable(params)})
	if invocationResponse.Error != nil {
		return invocationResponse.Error
	}
	if apiResponse != nil {
		err := json.Unmarshal(invocationResponse.Result, apiResponse)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetRemoteObject creates a new RemoteObject (proxy object for serverside object). Not intended to be used directly but rather from generated schema code.
func (r *RemoteObject) GetRemoteObject(objectInterface *ObjectInterface) *RemoteObject {
	return r.session.getOrCreateRemoteObject(r.session, objectInterface)
}

// newRemoteObject creates a new RemoteObject instance
func newRemoteObject(session *session, objectInterface *ObjectInterface) *RemoteObject {
	remoteObject := &RemoteObject{
		session:         session,
		ObjectInterface: objectInterface,
		changedChannels: make(map[chan struct{}]bool),
		mutex:           sync.Mutex{},
		closedCh:        make(chan struct{}),
	}
	// Signal that the object is by definition changed from the beginning
	return remoteObject
}
