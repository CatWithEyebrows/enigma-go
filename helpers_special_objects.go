package enigma

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
)

// The special objects below are ported from @qlik/api. Like the lists, each is a session object with a
// fixed id, so getting one twice reuses the session object.

// Current selections

// CurrentSelectionObject is a session object holding the app's current selections, with shortcuts
// for the selection methods on the app and its fields.
type CurrentSelectionObject struct {
	*GenericObject
	doc *Doc
}

func currentSelectionProps() *GenericObjectProperties {
	return &GenericObjectProperties{
		Info:               &NxInfo{Id: "CurrentSelection", Type: "CurrentSelection"},
		SelectionObjectDef: &SelectionObjectDef{},
	}
}

// GetCurrentSelectionObject returns the session object holding the app's current selections.
// Use ChangedChannel to know when the selections change.
func (doc *Doc) GetCurrentSelectionObject(ctx context.Context) (*CurrentSelectionObject, error) {
	obj, err := doc.GetOrCreateSessionObject(ctx, currentSelectionProps())
	if err != nil {
		return nil, err
	}
	return &CurrentSelectionObject{GenericObject: obj, doc: doc}, nil
}

// Selections returns the current selections, including the number of back and forward steps.
func (sel *CurrentSelectionObject) Selections(ctx context.Context) (*SelectionObject, error) {
	layout, err := sel.GetLayout(ctx)
	if err != nil {
		return nil, err
	}
	if layout.SelectionObject == nil {
		return &SelectionObject{}, nil
	}
	return layout.SelectionObject, nil
}

// ClearField clears the selections in a field. An empty stateName means the default state.
func (sel *CurrentSelectionObject) ClearField(ctx context.Context, fieldName, stateName string) (bool, error) {
	field, err := sel.doc.GetField(ctx, fieldName, stateName)
	if err != nil {
		return false, err
	}
	return field.Clear(ctx)
}

// LockField locks the selections in a field.
func (sel *CurrentSelectionObject) LockField(ctx context.Context, fieldName string) (bool, error) {
	field, err := sel.doc.GetField(ctx, fieldName, "")
	if err != nil {
		return false, err
	}
	return field.Lock(ctx)
}

// UnlockField unlocks the selections in a field.
func (sel *CurrentSelectionObject) UnlockField(ctx context.Context, fieldName string) (bool, error) {
	field, err := sel.doc.GetField(ctx, fieldName, "")
	if err != nil {
		return false, err
	}
	return field.Unlock(ctx)
}

// ClearAll clears all selections in the default state, except in locked fields.
func (sel *CurrentSelectionObject) ClearAll(ctx context.Context) error {
	return sel.doc.ClearAll(ctx, false, "")
}

// Back goes one step back in the selection history.
func (sel *CurrentSelectionObject) Back(ctx context.Context) error {
	return sel.doc.Back(ctx)
}

// Forward goes one step forward in the selection history.
func (sel *CurrentSelectionObject) Forward(ctx context.Context) error {
	return sel.doc.Forward(ctx)
}

// Undo info

// UndoInfoObject is a session object holding the app's undo and redo counts. It can group several
// changes so that Undo and Redo treat them as one step.
//
// Groups are stored as markers in the object's properties: StartGroup and EndGroup each add one.
// Changing the session object's properties is itself an undoable change, so undoing past a marker
// removes it again. Undo and Redo keep going while the number of markers is odd, that is, while
// they are inside a group.
type UndoInfoObject struct {
	*GenericObject
	doc *Doc
	// shared by every UndoInfoObject for the same app, since they wrap the same remote object
	state *undoGroupState
}

type undoGroupState struct {
	// held for each whole operation, so concurrent calls don't interleave their engine calls
	mutex sync.Mutex
	// id of the open group, 0 when no group is open
	groupID int64
	lastID  int64
	// set when Undo stopped inside a group because the undo buffer had no more steps
	limitReached bool
}

type undoInfoLayout struct {
	UndoInfo UndoInfo `json:"qUndoInfo"`
	Markers  []int    `json:"markers"`
}

func undoInfoProps() *GenericObjectProperties {
	return &GenericObjectProperties{
		Info:        &NxInfo{Id: "UndoInfo", Type: "UndoInfo"},
		UndoInfoDef: &UndoInfoDef{},
	}
}

// GetUndoInfoObject returns the session object holding the app's undo and redo counts.
func (doc *Doc) GetUndoInfoObject(ctx context.Context) (*UndoInfoObject, error) {
	obj, err := doc.GetOrCreateSessionObject(ctx, undoInfoProps())
	if err != nil {
		return nil, err
	}
	remote := obj.RemoteObject
	remote.mutex.Lock()
	if remote.undoGroups == nil {
		remote.undoGroups = &undoGroupState{}
	}
	state := remote.undoGroups
	remote.mutex.Unlock()
	return &UndoInfoObject{GenericObject: obj, doc: doc, state: state}, nil
}

func (undo *UndoInfoObject) layout(ctx context.Context) (*undoInfoLayout, error) {
	raw, err := undo.GetLayoutRaw(ctx)
	if err != nil {
		return nil, err
	}
	layout := &undoInfoLayout{}
	err = json.Unmarshal(raw, layout)
	return layout, err
}

func (undo *UndoInfoObject) setMarkers(ctx context.Context, markers []int) error {
	if markers == nil {
		markers = []int{}
	}
	return undo.SetPropertiesRaw(ctx, struct {
		*GenericObjectProperties
		Markers []int `json:"markers"`
	}{undoInfoProps(), markers})
}

// Counts returns the number of steps that can be undone and redone. A group counts as several steps.
func (undo *UndoInfoObject) Counts(ctx context.Context) (*UndoInfo, error) {
	layout, err := undo.layout(ctx)
	if err != nil {
		return nil, err
	}
	return &layout.UndoInfo, nil
}

// Undo undoes the last change, or the whole last group of changes.
func (undo *UndoInfoObject) Undo(ctx context.Context) error {
	undo.state.mutex.Lock()
	defer undo.state.mutex.Unlock()
	for {
		layout, err := undo.layout(ctx)
		if err != nil {
			return err
		}
		if layout.UndoInfo.UndoCount == 0 {
			// The start of the group is no longer in the undo buffer
			if len(layout.Markers)%2 != 0 {
				undo.state.limitReached = true
			}
			return nil
		}
		if _, err := undo.doc.Undo(ctx); err != nil {
			return err
		}
		if layout, err = undo.layout(ctx); err != nil {
			return err
		}
		if len(layout.Markers)%2 == 0 {
			return nil
		}
	}
}

// Redo redoes the last undone change, or the whole last undone group of changes.
func (undo *UndoInfoObject) Redo(ctx context.Context) error {
	undo.state.mutex.Lock()
	defer undo.state.mutex.Unlock()
	for {
		layout, err := undo.layout(ctx)
		if err != nil {
			return err
		}
		if layout.UndoInfo.RedoCount == 0 {
			return nil
		}
		undo.state.limitReached = false
		if _, err := undo.doc.Redo(ctx); err != nil {
			return err
		}
		// Unlike @qlik/api, which checks the markers from before the redo and so stops
		// after redoing only the start marker of a group
		if layout, err = undo.layout(ctx); err != nil {
			return err
		}
		if len(layout.Markers)%2 == 0 {
			return nil
		}
	}
}

// Reset clears the undo buffer and any group markers.
func (undo *UndoInfoObject) Reset(ctx context.Context) error {
	undo.state.mutex.Lock()
	defer undo.state.mutex.Unlock()
	layout, err := undo.layout(ctx)
	if err != nil {
		return err
	}
	undo.state.groupID, undo.state.limitReached = 0, false
	if layout.UndoInfo.UndoCount == 0 && layout.UndoInfo.RedoCount == 0 {
		return nil
	}
	// The undo buffer doesn't include the markers, so they are cleared first
	if len(layout.Markers) > 0 {
		if err := undo.setMarkers(ctx, nil); err != nil {
			return err
		}
	}
	return undo.doc.ClearUndoBuffer(ctx)
}

// StartGroup starts a group of changes that Undo and Redo treat as one step, and returns the
// group's id to pass to EndGroup. If a group is already open, it returns that group's id, so
// nested groups become part of the outer one.
func (undo *UndoInfoObject) StartGroup(ctx context.Context) (int64, error) {
	undo.state.mutex.Lock()
	defer undo.state.mutex.Unlock()
	layout, err := undo.layout(ctx)
	if err != nil {
		return 0, err
	}
	markers := layout.Markers
	if len(markers)%2 != 0 && !undo.state.limitReached {
		if undo.state.groupID == 0 {
			// Opened before this program attached to the session
			undo.state.lastID++
			undo.state.groupID = undo.state.lastID
		}
		return undo.state.groupID, nil
	}
	if undo.state.limitReached {
		markers = nil
	}
	if err := undo.setMarkers(ctx, append(markers, layout.UndoInfo.UndoCount+1)); err != nil {
		return 0, err
	}
	undo.state.limitReached = false
	undo.state.lastID++
	undo.state.groupID = undo.state.lastID
	return undo.state.groupID, nil
}

// EndGroup ends the group with the id StartGroup returned. It does nothing when that group is
// not the open one.
func (undo *UndoInfoObject) EndGroup(ctx context.Context, groupID int64) error {
	if groupID == 0 {
		return errors.New("enigma: EndGroup needs the group id StartGroup returned")
	}
	undo.state.mutex.Lock()
	defer undo.state.mutex.Unlock()
	layout, err := undo.layout(ctx)
	if err != nil {
		return err
	}
	if len(layout.Markers)%2 == 0 || groupID != undo.state.groupID {
		return nil
	}
	if err := undo.setMarkers(ctx, append(layout.Markers, layout.UndoInfo.UndoCount+1)); err != nil {
		return err
	}
	undo.state.groupID = 0
	return nil
}

// Group runs fn inside a group of changes, so that Undo and Redo treat everything fn changes as one step.
// The group is ended even when fn returns an error.
func (undo *UndoInfoObject) Group(ctx context.Context, fn func() error) error {
	groupID, err := undo.StartGroup(ctx)
	if err != nil {
		return err
	}
	return errors.Join(fn(), undo.EndGroup(ctx, groupID))
}
