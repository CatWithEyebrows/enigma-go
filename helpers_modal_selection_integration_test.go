package enigma

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModalSelectionIntegration(t *testing.T) {
	doc := connectSessionApp(t)
	ctx := context.Background()
	// Unlinked tables, so a selection in one field never excludes values in the other
	require.NoError(t, doc.SetScript(ctx, "TA: Load RecNo() as A AutoGenerate 5; TB: Load RecNo() as B AutoGenerate 5;"))
	ok, err := doc.DoReload(ctx, 0, false, false)
	require.NoError(t, err)
	require.True(t, ok)

	listbox := func(field string) *GenericObject {
		obj, err := doc.CreateSessionObject(ctx, &GenericObjectProperties{
			Info:          &NxInfo{Type: "listbox"},
			ListObjectDef: &ListObjectDef{Def: &NxInlineDimensionDef{FieldDefs: []string{field}}},
		})
		require.NoError(t, err)
		return obj
	}
	a, b := listbox("A"), listbox("B")
	sel, err := doc.GetCurrentSelectionObject(ctx)
	require.NoError(t, err)
	selected := func() map[string]string {
		selections, err := sel.Selections(ctx)
		require.NoError(t, err)
		result := map[string]string{}
		for _, item := range selections.Selections {
			result[item.Field] = item.Selected
		}
		return result
	}
	selectValue := func(obj *GenericObject, row int) {
		_, err := obj.SelectListObjectValues(ctx, "/qListObjectDef", []int{row}, false, false)
		require.NoError(t, err)
	}
	paths := []string{"/qListObjectDef"}
	modal := doc.ModalSelection()

	require.NoError(t, modal.Switch(ctx, a, paths, true))
	selectValue(a, 0)
	// Switching ends selection mode on a first, which the engine requires before b can begin
	require.NoError(t, modal.Switch(ctx, b, paths, true))
	assert.Same(t, b, modal.Object())
	selectValue(b, 1)
	require.NoError(t, modal.Switch(ctx, nil, nil, false))
	assert.Nil(t, modal.Object())
	assert.Equal(t, map[string]string{"A": "1"}, selected(), "a's selection accepted, b's discarded")

	require.NoError(t, modal.Switch(ctx, b, paths, true))
	selectValue(b, 2)
	require.NoError(t, modal.Abort(ctx, true))
	assert.Nil(t, modal.Object())
	assert.Equal(t, map[string]string{"A": "1", "B": "3"}, selected())

	// Destroying the object ends selection mode
	require.NoError(t, modal.Switch(ctx, a, paths, true))
	_, err = doc.DestroySessionObject(ctx, a.GenericId)
	require.NoError(t, err)
	assert.Nil(t, modal.Object())
	require.NoError(t, modal.Switch(ctx, b, paths, true))
	assert.Same(t, b, modal.Object())

	// Paths the engine would reject are caught before they lock the app
	require.NoError(t, modal.Switch(ctx, nil, nil, false))
	for _, paths := range [][]string{{"/nope"}, {""}, nil} {
		assert.ErrorIs(t, modal.Switch(ctx, b, paths, true), ErrInvalidSelectionPaths)
	}
	assert.Nil(t, modal.Object())
	_, err = b.SelectListObjectValues(ctx, "/qListObjectDef", []int{0}, false, false)
	assert.NoError(t, err, "selections still work")

	// Tree data can be in selection mode too
	tree, err := doc.CreateSessionObjectRaw(ctx, map[string]any{
		"qInfo":        map[string]any{"qType": "tree"},
		"qTreeDataDef": map[string]any{"qDimensions": []any{map[string]any{"qDef": map[string]any{"qFieldDefs": []string{"A"}}}}},
	})
	require.NoError(t, err)
	require.NoError(t, modal.Switch(ctx, tree, []string{"/qTreeDataDef"}, true))
	assert.Same(t, tree, modal.Object())
	require.NoError(t, modal.Abort(ctx, false))
}
