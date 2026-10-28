package enigma

import (
	"context"
	"slices"
	"sync"
)

// ModalSelection is ported from @qlik/api's switchModalSelection and abortModalSelection.

// ModalSelection keeps track of which object in an app is in selection mode (modal state), started
// with GenericObject.BeginSelections. While an object is in selection mode, the engine refuses
// selections and BeginSelections on other objects (LOCERR_HC_MODAL_OBJECT_ERROR), so Switch ends
// selections on the previous object before beginning them on the next one.
type ModalSelection struct {
	doc   *Doc
	mutex sync.Mutex
	obj   *GenericObject
	paths []string
}

// ModalSelection returns the app's modal selection state. Every call for the same app returns the same one.
func (doc *Doc) ModalSelection() *ModalSelection {
	remote := doc.RemoteObject
	remote.mutex.Lock()
	defer remote.mutex.Unlock()
	if remote.modalSelection == nil {
		remote.modalSelection = &ModalSelection{doc: doc}
	}
	return remote.modalSelection
}

// active returns the object in selection mode, or nil if there is none. The engine ends selection
// mode when the object is destroyed, keeping the selections made in it.
func (modal *ModalSelection) active() *GenericObject {
	if modal.obj == nil {
		return nil
	}
	select {
	case <-modal.obj.Closed():
		modal.obj, modal.paths = nil, nil
	default:
	}
	return modal.obj
}

// Object returns the object in selection mode, or nil if there is none.
func (modal *ModalSelection) Object() *GenericObject {
	modal.mutex.Lock()
	defer modal.mutex.Unlock()
	return modal.active()
}

// Switch puts obj in selection mode for the hypercubes or list objects at paths, such as
// "/qHyperCubeDef". If another object is in selection mode, its selections are ended first,
// and accept says whether to keep the selections made in it. A nil obj only ends selection mode.
//
// Unlike @qlik/api, switching to the object already in selection mode with other paths begins
// selections again with those paths. The engine keeps the selections made so far.
func (modal *ModalSelection) Switch(ctx context.Context, obj *GenericObject, paths []string, accept bool) error {
	modal.mutex.Lock()
	defer modal.mutex.Unlock()
	current := modal.active()
	same := current != nil && obj != nil && current.RemoteObject == obj.RemoteObject
	if current != nil && !same {
		if err := current.EndSelections(ctx, accept); err != nil {
			return err
		}
		modal.obj, modal.paths = nil, nil
	}
	if obj == nil {
		return nil
	}
	select {
	case <-obj.Closed():
		return nil
	default:
	}
	if same && slices.Equal(paths, modal.paths) {
		return nil
	}
	if err := obj.BeginSelections(ctx, paths); err != nil {
		return err
	}
	modal.obj, modal.paths = obj, slices.Clone(paths)
	return nil
}

// Abort ends selection mode in the app, whichever object is in it, also when it wasn't started
// through Switch. accept says whether to keep the selections made in it.
func (modal *ModalSelection) Abort(ctx context.Context, accept bool) error {
	modal.mutex.Lock()
	defer modal.mutex.Unlock()
	modal.obj, modal.paths = nil, nil
	return modal.doc.AbortModal(ctx, accept)
}
