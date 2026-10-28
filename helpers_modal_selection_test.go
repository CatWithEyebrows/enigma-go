package enigma

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Properties with a hypercube and a list object
const selectableProps = `{"qProp":{"qInfo":{"qId":"a"},"qHyperCubeDef":{},"qListObjectDef":{}}}`

var getProperties = fakeCall{method: "GetProperties", params: []interface{}{}}

func TestModalSelectionSwitch(t *testing.T) {
	handles := map[string]int{"a": 5, "b": 6}
	doc, calls := connectFakeEngine(t, func(method string, params []interface{}) string {
		if method == "GetObject" {
			id := params[0].(string)
			return object(handles[id], id)
		}
		if method == "GetProperties" {
			return selectableProps
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
	assert.Equal(t, []call{getProperties, {method: "BeginSelections", params: []interface{}{paths}}}, recorded())
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
	assert.Equal(t, []call{getProperties, {method: "BeginSelections", params: []interface{}{other}}}, recorded())

	// Invalid paths: nothing changes
	err = modal.Switch(ctx, b, []string{"/qHyperCubeDefTypo"}, true)
	assert.ErrorIs(t, err, ErrInvalidSelectionPaths)
	assert.Equal(t, []call{getProperties}, recorded())
	assert.Same(t, a, modal.Object())

	// Another object: end the first one, then begin
	require.NoError(t, modal.Switch(ctx, b, paths, false))
	assert.Equal(t, []call{
		getProperties,
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
	assert.Equal(t, []string{"GetProperties", "BeginSelections", "AbortModal"}, methods(*calls))
	assert.Nil(t, modal.Object())
}

func TestModalSelectionClosedObject(t *testing.T) {
	doc, calls := connectFakeEngine(t, func(method string, params []interface{}) string {
		if method == "GetObject" {
			return object(5, "a")
		}
		if method == "GetProperties" {
			return selectableProps
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

func TestCheckSelectionPaths(t *testing.T) {
	props := json.RawMessage(`{
		"qInfo": {"qId": "a"},
		"qHyperCubeDef": {},
		"x": {"qTreeDataDef": {"qDimensions": []}},
		"arr": [{"qListObjectDef": {}}],
		"a/b": {"qListObjectDef": {}},
		"myCube": {"qDimensions": []},
		"q": {"qHyperCubeDef": "not an object"}
	}`)
	for _, paths := range [][]string{
		{"/qHyperCubeDef"},
		{"/x/qTreeDataDef"},
		{"/arr/0/qListObjectDef"},
		{"/a~1b/qListObjectDef"},
		{"/qHyperCubeDef", "/x/qTreeDataDef"},
	} {
		assert.NoError(t, checkSelectionPaths(props, paths), paths)
	}
	for _, paths := range [][]string{
		nil,
		{""},
		{"qHyperCubeDef"},
		{"/qHyperCubeDef/"},
		{"/qInfo"},
		{"/qListObjectDef"},
		{"/myCube"},
		{"/q/qHyperCubeDef"},
		{"/arr/1/qListObjectDef"},
		{"/arr/x/qListObjectDef"},
		{"/qHyperCubeDef", "/qInfo"},
	} {
		assert.ErrorIs(t, checkSelectionPaths(props, paths), ErrInvalidSelectionPaths, paths)
	}
}
