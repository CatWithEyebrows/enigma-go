package enigma

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentSelectionIntegration(t *testing.T) {
	doc := connectSessionApp(t)
	ctx := context.Background()
	require.NoError(t, doc.SetScript(ctx, "T: Load RecNo() as A, RecNo() as B AutoGenerate 5;"))
	ok, err := doc.DoReload(ctx, 0, false, false)
	require.NoError(t, err)
	require.True(t, ok)

	sel, err := doc.GetCurrentSelectionObject(ctx)
	require.NoError(t, err)
	again, err := doc.GetCurrentSelectionObject(ctx)
	require.NoError(t, err)
	assert.Equal(t, sel.Handle, again.Handle)

	selectedFields := func() []string {
		selections, err := sel.Selections(ctx)
		require.NoError(t, err)
		var fields []string
		for _, item := range selections.Selections {
			fields = append(fields, item.Field)
		}
		return fields
	}
	for _, name := range []string{"A", "B"} {
		field, err := doc.GetField(ctx, name, "")
		require.NoError(t, err)
		_, err = field.Select(ctx, "2", false, 0)
		require.NoError(t, err)
	}
	assert.ElementsMatch(t, []string{"A", "B"}, selectedFields())

	_, err = sel.ClearField(ctx, "A", "")
	require.NoError(t, err)
	assert.Equal(t, []string{"B"}, selectedFields())
	require.NoError(t, sel.Back(ctx))
	assert.ElementsMatch(t, []string{"A", "B"}, selectedFields())
	require.NoError(t, sel.Forward(ctx))
	assert.Equal(t, []string{"B"}, selectedFields())

	_, err = sel.LockField(ctx, "B")
	require.NoError(t, err)
	require.NoError(t, sel.ClearAll(ctx))
	assert.Equal(t, []string{"B"}, selectedFields(), "locked fields are kept")
	_, err = sel.UnlockField(ctx, "B")
	require.NoError(t, err)
	require.NoError(t, sel.ClearAll(ctx))
	assert.Empty(t, selectedFields())

	_, err = sel.ClearField(ctx, "Unknown", "")
	assert.ErrorIs(t, err, ErrObjectNotFound)
}

func TestUndoInfoIntegration(t *testing.T) {
	doc := connectSessionApp(t)
	ctx := context.Background()
	undo, err := doc.GetUndoInfoObject(ctx)
	require.NoError(t, err)
	obj, err := doc.CreateObjectRaw(ctx, map[string]any{"qInfo": map[string]any{"qId": "o1", "qType": "x"}, "v": 0})
	require.NoError(t, err)
	value := func() float64 {
		props, err := obj.GetPropertiesRaw(ctx)
		require.NoError(t, err)
		var v struct{ V float64 }
		require.NoError(t, json.Unmarshal(props, &v))
		return v.V
	}
	set := func(v int) {
		require.NoError(t, obj.SetPropertiesRaw(ctx, map[string]any{"qInfo": map[string]any{"qId": "o1", "qType": "x"}, "v": v}))
	}

	set(1)
	err = undo.Group(ctx, func() error {
		set(2)
		set(3)
		return nil
	})
	require.NoError(t, err)
	set(4)

	require.NoError(t, undo.Undo(ctx))
	assert.Equal(t, 3.0, value())
	require.NoError(t, undo.Undo(ctx))
	assert.Equal(t, 1.0, value(), "the group is undone as one step")
	require.NoError(t, undo.Redo(ctx))
	assert.Equal(t, 3.0, value(), "the group is redone as one step")
	require.NoError(t, undo.Redo(ctx))
	assert.Equal(t, 4.0, value())

	require.NoError(t, undo.Reset(ctx))
	counts, err := undo.Counts(ctx)
	require.NoError(t, err)
	assert.Equal(t, UndoInfo{}, *counts)
}
