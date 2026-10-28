package enigma

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
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

// ErrInvalidSelectionPaths is returned by ModalSelection.Switch when a path doesn't point to a hypercube,
// list object or tree data definition in the object's properties. Switch checks this before calling
// BeginSelections, because when the engine rejects the paths it leaves the whole app in selection
// mode: every selection on every object fails with LOCERR_HC_MODAL_OBJECT_ERROR, and only a
// successful BeginSelections on the same object followed by EndSelections or AbortModal ends it.
var ErrInvalidSelectionPaths = errors.New("enigma: selection paths must point to a qHyperCubeDef, qListObjectDef or qTreeDataDef")

// The definitions the engine accepts as BeginSelections paths. It goes by the property name,
// so a hypercube definition stored under another name is rejected.
var selectionDefinitions = []string{"qHyperCubeDef", "qListObjectDef", "qTreeDataDef"}

// checkSelectionPaths returns ErrInvalidSelectionPaths unless every path is a JSON pointer to an object in
// props named one of selectionDefinitions. The engine also accepts paths when at least one of them is valid,
// but a mistyped path is more likely a bug than intended.
func checkSelectionPaths(props json.RawMessage, paths []string) error {
	if len(paths) == 0 {
		return fmt.Errorf("%w, got no paths", ErrInvalidSelectionPaths)
	}
	var tree any
	if err := json.Unmarshal(props, &tree); err != nil {
		return err
	}
	var invalid []string
	for _, path := range paths {
		if !isSelectionPath(tree, path) {
			invalid = append(invalid, path)
		}
	}
	if len(invalid) > 0 {
		return fmt.Errorf("%w, got %q", ErrInvalidSelectionPaths, invalid)
	}
	return nil
}

func isSelectionPath(tree any, path string) bool {
	if !strings.HasPrefix(path, "/") {
		return false
	}
	tokens := strings.Split(path[1:], "/")
	last := ""
	for _, token := range tokens {
		token = strings.NewReplacer("~1", "/", "~0", "~").Replace(token)
		switch node := tree.(type) {
		case map[string]any:
			child, ok := node[token]
			if !ok {
				return false
			}
			tree = child
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(node) {
				return false
			}
			tree = node[index]
		default:
			return false
		}
		last = token
	}
	_, isObject := tree.(map[string]any)
	return isObject && slices.Contains(selectionDefinitions, last)
}

// Switch puts obj in selection mode for the hypercubes, list objects or tree data at paths, such as
// "/qHyperCubeDef". If another object is in selection mode, its selections are ended first, and accept
// says whether to keep the selections made in it. A nil obj only ends selection mode.
//
// Switch first checks that every path points to a qHyperCubeDef, qListObjectDef or qTreeDataDef in
// obj's properties, and returns ErrInvalidSelectionPaths without changing anything if not, see there why.
//
// Unlike @qlik/api, switching to the object already in selection mode with other paths begins
// selections again with those paths. The engine keeps the selections made so far.
func (modal *ModalSelection) Switch(ctx context.Context, obj *GenericObject, paths []string, accept bool) error {
	modal.mutex.Lock()
	defer modal.mutex.Unlock()
	if obj != nil {
		select {
		case <-obj.Closed():
			obj = nil
		default:
		}
	}
	current := modal.active()
	same := current != nil && obj != nil && current.RemoteObject == obj.RemoteObject
	if same && slices.Equal(paths, modal.paths) {
		return nil
	}
	if obj != nil {
		props, err := obj.GetPropertiesRaw(ctx)
		if err != nil {
			return err
		}
		if err := checkSelectionPaths(props, paths); err != nil {
			return err
		}
	}
	if current != nil && !same {
		if err := current.EndSelections(ctx, accept); err != nil {
			return err
		}
		modal.obj, modal.paths = nil, nil
	}
	if obj == nil {
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
