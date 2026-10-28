package enigma

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModalSelectionSwitch(t *testing.T) {
	handles := map[string]int{"a": 5, "b": 6}
	doc, calls := connectFakeEngine(t, func(method string, params []interface{}) string {
		if method == "GetObject" {
			id := params[0].(string)
			return object(handles[id], id)
		}
		return `{}`
	})
	ctx := context.Background()
	a, err := doc.GetObject(ctx, "a")
	require.NoError(t, err)
	b, err := doc.GetObject(ctx, "b")
	require.NoError(t, err)
	modal := doc.ModalSelection()
	assert.Same(t, modal, (&Doc{doc.RemoteObject}).ModalSelection(), "shared by every Doc for the app")
	assert.Nil(t, modal.Object())

	type call = fakeCall
	// recorded returns the calls since the last time and forgets them
	recorded := func() []call {
		got := *calls
		*calls = nil
		return got
	}
	*calls = nil
	paths := []string{"/qHyperCubeDef"}

	require.NoError(t, modal.Switch(ctx, a, paths, true))
	assert.Equal(t, []call{{method: "BeginSelections", params: []interface{}{paths}}}, recorded())
	assert.Same(t, a, modal.Object())

	// Same object and paths, even through another wrapper: nothing to do
	again, err := doc.GetObject(ctx, "a")
	require.NoError(t, err)
	*calls = nil
	require.NoError(t, modal.Switch(ctx, again, []string{"/qHyperCubeDef"}, true))
	assert.Empty(t, recorded())

	// Same object, other paths: begin again
	other := []string{"/qListObjectDef"}
	require.NoError(t, modal.Switch(ctx, a, other, true))
	assert.Equal(t, []call{{method: "BeginSelections", params: []interface{}{other}}}, recorded())

	// Another object: end the first one, then begin
	require.NoError(t, modal.Switch(ctx, b, paths, false))
	assert.Equal(t, []call{
		{method: "EndSelections", params: []interface{}{false}},
		{method: "BeginSelections", params: []interface{}{paths}},
	}, recorded())
	assert.Same(t, b, modal.Object())

	// nil only ends
	require.NoError(t, modal.Switch(ctx, nil, nil, true))
	assert.Equal(t, []call{{method: "EndSelections", params: []interface{}{true}}}, recorded())
	assert.Nil(t, modal.Object())

	require.NoError(t, modal.Switch(ctx, a, paths, true))
	require.NoError(t, modal.Abort(ctx, true))
	assert.Equal(t, []string{"BeginSelections", "AbortModal"}, methods(*calls))
	assert.Nil(t, modal.Object())
}

func TestModalSelectionClosedObject(t *testing.T) {
	doc, calls := connectFakeEngine(t, func(method string, params []interface{}) string {
		if method == "GetObject" {
			return object(5, "a")
		}
		return `{}`
	})
	ctx := context.Background()
	a, err := doc.GetObject(ctx, "a")
	require.NoError(t, err)
	modal := doc.ModalSelection()
	require.NoError(t, modal.Switch(ctx, a, []string{"/qHyperCubeDef"}, true))

	// The engine ends selection mode when the object is destroyed
	a.session.handleUpdates(nil, []int{a.Handle})
	assert.Nil(t, modal.Object())
	*calls = nil
	require.NoError(t, modal.Switch(ctx, nil, nil, true))
	assert.Empty(t, *calls, "no EndSelections on a closed object")
	require.NoError(t, modal.Switch(ctx, a, []string{"/qHyperCubeDef"}, true))
	assert.Empty(t, *calls, "no BeginSelections on a closed object")
}
