package enigma

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeUndoEngine keeps an undo buffer of UndoInfo marker states. Like the engine, every SetProperties,
// including the one on the UndoInfo object, is an undo step.
type fakeUndoEngine struct {
	states [][]int
	pos    int
	// oldest steps are dropped beyond this many, 0 for no limit
	limit int
}

func (e *fakeUndoEngine) push(markers []int) {
	e.states = append(e.states[:e.pos+1], markers)
	e.pos++
	if e.limit > 0 && e.pos > e.limit {
		e.states = e.states[e.pos-e.limit:]
		e.pos = e.limit
	}
}

func (e *fakeUndoEngine) respond(method string, params []interface{}) string {
	switch method {
	case "GetObject":
		return object(7, "UndoInfo")
	case "GetLayout":
		markers, _ := json.Marshal(e.states[e.pos])
		return fmt.Sprintf(`{"qLayout":{"qUndoInfo":{"qUndoCount":%d,"qRedoCount":%d},"markers":%s}}`, e.pos, len(e.states)-1-e.pos, markers)
	case "SetProperties":
		var props struct {
			Markers []int `json:"markers"`
		}
		raw, _ := json.Marshal(params[0])
		_ = json.Unmarshal(raw, &props)
		if props.Markers == nil {
			props.Markers = e.states[e.pos] // a change to another object
		}
		e.push(props.Markers)
	case "Undo":
		e.pos--
	case "Redo":
		e.pos++
	case "ClearUndoBuffer":
		e.states, e.pos = [][]int{e.states[e.pos]}, 0
	}
	return `{}`
}

func (e *fakeUndoEngine) change() { e.push(e.states[e.pos]) }

func newFakeUndo(t *testing.T, limit int) (*UndoInfoObject, *fakeUndoEngine) {
	engine := &fakeUndoEngine{states: [][]int{nil}, limit: limit}
	doc, _ := connectFakeEngine(t, engine.respond)
	undo, err := doc.GetUndoInfoObject(context.Background())
	require.NoError(t, err)
	return undo, engine
}

func TestUndoGroups(t *testing.T) {
	undo, engine := newFakeUndo(t, 0)
	ctx := context.Background()
	engine.change()
	err := undo.Group(ctx, func() error {
		engine.change()
		engine.change()
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []int{2, 5}, engine.states[engine.pos])

	require.NoError(t, undo.Undo(ctx))
	assert.Equal(t, 1, engine.pos, "the whole group is undone")
	require.NoError(t, undo.Undo(ctx))
	assert.Equal(t, 0, engine.pos)
	require.NoError(t, undo.Undo(ctx))
	assert.Equal(t, 0, engine.pos)

	require.NoError(t, undo.Redo(ctx))
	assert.Equal(t, 1, engine.pos)
	require.NoError(t, undo.Redo(ctx))
	assert.Equal(t, 5, engine.pos, "the whole group is redone")
	require.NoError(t, undo.Redo(ctx))
	assert.Equal(t, 5, engine.pos)

	require.NoError(t, undo.Reset(ctx))
	assert.Equal(t, [][]int{{}}, engine.states)
}

func TestUndoNestedGroups(t *testing.T) {
	undo, engine := newFakeUndo(t, 0)
	ctx := context.Background()
	outer, err := undo.StartGroup(ctx)
	require.NoError(t, err)
	inner, err := undo.StartGroup(ctx)
	require.NoError(t, err)
	assert.Equal(t, outer, inner)
	engine.change()
	require.NoError(t, undo.EndGroup(ctx, inner))
	require.NoError(t, undo.EndGroup(ctx, outer))
	assert.Equal(t, []int{1, 3}, engine.states[engine.pos])
	assert.Error(t, undo.EndGroup(ctx, 0))

	// Another wrapper for the same object shares the open group
	undo.StartGroup(ctx)
	other, err := undo.doc.GetUndoInfoObject(ctx)
	require.NoError(t, err)
	id, err := other.StartGroup(ctx)
	require.NoError(t, err)
	assert.Equal(t, undo.state.groupID, id)
}

func TestUndoLimitReached(t *testing.T) {
	undo, engine := newFakeUndo(t, 3)
	ctx := context.Background()
	err := undo.Group(ctx, func() error {
		engine.change()
		engine.change()
		engine.change()
		return nil
	})
	require.NoError(t, err)
	// The start marker step fell out of the buffer, so undo stops inside the group
	require.NoError(t, undo.Undo(ctx))
	assert.Equal(t, 0, engine.pos)
	assert.Len(t, engine.states[engine.pos], 1)
	assert.True(t, undo.state.limitReached)

	// A new group starts over instead of joining the broken one
	_, err = undo.StartGroup(ctx)
	require.NoError(t, err)
	assert.Equal(t, []int{1}, engine.states[engine.pos])
	assert.False(t, undo.state.limitReached)
}

func TestCurrentSelectionObject(t *testing.T) {
	doc, calls := connectFakeEngine(t, func(method string, params []interface{}) string {
		switch method {
		case "GetObject":
			return object(5, "CurrentSelection")
		case "GetField":
			return `{"qReturn":{"qType":"Field","qHandle":6}}`
		case "GetLayout":
			return `{"qLayout":{"qSelectionObject":{"qBackCount":2,"qSelections":[{"qField":"A"}]}}}`
		}
		return `{"qReturn":true}`
	})
	ctx := context.Background()
	sel, err := doc.GetCurrentSelectionObject(ctx)
	require.NoError(t, err)
	selections, err := sel.Selections(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, selections.BackCount)
	assert.Equal(t, "A", selections.Selections[0].Field)

	*calls = nil
	ok, err := sel.ClearField(ctx, "A", "S1")
	assert.True(t, ok)
	assert.NoError(t, err)
	assert.Equal(t, []interface{}{"A", "S1"}, (*calls)[0].params)
	_, err = sel.LockField(ctx, "A")
	assert.NoError(t, err)
	_, err = sel.UnlockField(ctx, "A")
	assert.NoError(t, err)
	assert.NoError(t, sel.ClearAll(ctx))
	assert.NoError(t, sel.Back(ctx))
	assert.NoError(t, sel.Forward(ctx))
	assert.Equal(t, []string{"GetField", "Clear", "GetField", "Lock", "GetField", "Unlock", "ClearAll", "Back", "Forward"}, methods(*calls))
}
