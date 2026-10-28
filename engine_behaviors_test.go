package enigma

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUnknownObjectReturnsErrObjectNotFound(t *testing.T) {
	doc, _ := connectFakeEngine(t, func(string, []interface{}) string { return emptyObject })
	ctx := context.Background()

	obj, err := doc.GetObject(ctx, "unknown")
	assert.Nil(t, obj)
	assert.ErrorIs(t, err, ErrObjectNotFound)
	field, err := doc.GetField(ctx, "unknown", "")
	assert.Nil(t, field)
	assert.ErrorIs(t, err, ErrObjectNotFound)
	variable, err := doc.GetVariableByName(ctx, "unknown")
	assert.Nil(t, variable)
	assert.ErrorIs(t, err, ErrObjectNotFound)
}

func TestCreateWithIDTaken(t *testing.T) {
	// A new object always gets a new handle, and the session caches remote objects by handle
	created := map[string]string{"free": object(5, "free"), "taken": object(6, "random-id")}
	doc, calls := connectFakeEngine(t, func(method string, params []interface{}) string {
		if method == "CreateObject" {
			return created[params[0].(*GenericObjectProperties).Info.Id]
		}
		return `{"qSuccess":true}`
	})
	ctx := context.Background()

	obj, err := doc.CreateObjectWithID(ctx, &GenericObjectProperties{Info: &NxInfo{Id: "free", Type: "x"}})
	assert.NoError(t, err)
	assert.Equal(t, "free", obj.GenericId)
	assert.Equal(t, []string{"CreateObject"}, methods(*calls))

	*calls = nil
	obj, err = doc.CreateObjectWithID(ctx, &GenericObjectProperties{Info: &NxInfo{Id: "taken", Type: "x"}})
	assert.Nil(t, obj)
	assert.ErrorIs(t, err, ErrIDTaken)
	assert.Equal(t, []string{"CreateObject", "DestroyObject"}, methods(*calls))
	assert.Equal(t, []interface{}{"random-id"}, (*calls)[1].params)
}

func TestCreateWithIDWithoutID(t *testing.T) {
	doc, calls := connectFakeEngine(t, func(method string, params []interface{}) string { return object(5, "random-id") })
	ctx := context.Background()
	for _, props := range []*GenericObjectProperties{{Info: &NxInfo{Type: "x"}}, {}, nil} {
		obj, err := doc.CreateSessionObjectWithID(ctx, props)
		assert.NoError(t, err)
		assert.Equal(t, "random-id", obj.GenericId)
	}
	assert.Equal(t, []string{"CreateSessionObject", "CreateSessionObject", "CreateSessionObject"}, methods(*calls))
}

type customListData struct {
	Title   string   `json:"title"`
	Count   int      `json:"count"`
	Visible bool     `json:"visible"`
	Tags    []string `json:"tags"`
}

func TestUnmarshalListDataMissingPaths(t *testing.T) {
	var data customListData
	// The engine returns "" for paths that don't exist in an object
	err := UnmarshalListData(json.RawMessage(`{"title":"T","count":"","visible":"","tags":""}`), &data)
	assert.NoError(t, err)
	assert.Equal(t, customListData{Title: "T"}, data)

	err = UnmarshalListData(json.RawMessage(`{"title":"T","count":3,"visible":true,"tags":["a"]}`), &data)
	assert.NoError(t, err)
	assert.Equal(t, customListData{Title: "T", Count: 3, Visible: true, Tags: []string{"a"}}, data)
}

func TestGetSessionListCustomList(t *testing.T) {
	doc, _ := connectFakeEngine(t, func(method string, params []interface{}) string {
		if method == "GetLayout" {
			return `{"qLayout":{"qAppObjectList":{"qItems":[{"qInfo":{"qId":"o1"},"qData":{"title":"One","count":""}}]}}}`
		}
		return object(5, "MyList")
	})
	ctx := context.Background()
	list, err := GetSessionList[*ListItem[customListData]](ctx, doc, &GenericObjectProperties{
		Info:             &NxInfo{Id: "MyList", Type: "MyList"},
		AppObjectListDef: &AppObjectListDef{Type: "my-object", Data: json.RawMessage(`{"title":"/title","count":"/count"}`)},
	})
	require.NoError(t, err)
	items, err := list.Items(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, customListData{Title: "One"}, items[0].Data)

	_, err = GetSessionList[*ListItem[customListData]](ctx, doc, &GenericObjectProperties{Info: &NxInfo{Id: "NoList"}})
	assert.Error(t, err, "props without a list definition")
}

func TestEngineBehaviorsIntegration(t *testing.T) {
	doc := connectSessionApp(t)
	ctx := context.Background()
	require.NoError(t, doc.SetScript(ctx, "T: Load 1 as A AutoGenerate 1;"))
	_, err := doc.DoReload(ctx, 0, false, false)
	require.NoError(t, err)

	t.Run("unknown ids", func(t *testing.T) {
		_, err := doc.GetObject(ctx, "unknown")
		assert.ErrorIs(t, err, ErrObjectNotFound)
		_, err = doc.GetDimension(ctx, "unknown")
		assert.ErrorIs(t, err, ErrObjectNotFound)
		_, err = doc.GetField(ctx, "unknown", "")
		assert.ErrorIs(t, err, ErrObjectNotFound)
		field, err := doc.GetField(ctx, "A", "")
		require.NoError(t, err)
		assert.Equal(t, "Field", field.Type)
		assert.False(t, errors.Is(err, ErrObjectNotFound))
	})

	t.Run("taken ids", func(t *testing.T) {
		dimension := &GenericDimensionProperties{Info: &NxInfo{Id: "dim-1", Type: "dimension"}, Dim: &NxLibraryDimensionDef{FieldDefs: []string{"A"}}}
		_, err := doc.CreateDimensionWithID(ctx, dimension)
		require.NoError(t, err)
		_, err = doc.CreateDimensionWithID(ctx, dimension)
		assert.ErrorIs(t, err, ErrIDTaken)
		dimensions, err := doc.GetDimensionList(ctx)
		require.NoError(t, err)
		require.Len(t, dimensions, 1, "the duplicate is removed")
		assert.Equal(t, "dim-1", dimensions[0].Info.Id)

		object := &GenericObjectProperties{Info: &NxInfo{Id: "obj-1", Type: "my-type"}}
		_, err = doc.CreateObjectWithID(ctx, object)
		require.NoError(t, err)
		_, err = doc.CreateObjectWithID(ctx, object)
		assert.ErrorIs(t, err, ErrIDTaken)
		objects, err := doc.GetObjects(ctx, &NxGetObjectOptions{Types: []string{"my-type"}})
		require.NoError(t, err)
		require.Len(t, objects, 1, "the duplicate is removed")

		// The plain create still falls back to a random id
		duplicate, err := doc.CreateObject(ctx, object)
		require.NoError(t, err)
		assert.NotEqual(t, "obj-1", duplicate.GenericId)
	})

	t.Run("missing qData paths", func(t *testing.T) {
		_, err := doc.CreateObjectRaw(ctx, map[string]any{"qInfo": map[string]any{"qId": "full", "qType": "custom"}, "title": "Full", "count": 3, "visible": true, "tags": []string{"a"}})
		require.NoError(t, err)
		_, err = doc.CreateObjectRaw(ctx, map[string]any{"qInfo": map[string]any{"qId": "empty", "qType": "custom"}})
		require.NoError(t, err)
		qData := json.RawMessage(`{"title":"/title","count":"/count","visible":"/visible","tags":"/tags"}`)

		list, err := GetSessionList[*ListItem[customListData]](ctx, doc, &GenericObjectProperties{
			Info:             &NxInfo{Id: "CustomList", Type: "CustomList"},
			AppObjectListDef: &AppObjectListDef{Type: "custom", Data: qData},
		})
		require.NoError(t, err)
		items, err := list.Items(ctx)
		require.NoError(t, err)
		byID := map[string]customListData{}
		for _, item := range items {
			byID[item.Info.Id] = item.Data
		}
		assert.Equal(t, map[string]customListData{
			"full":  {Title: "Full", Count: 3, Visible: true, Tags: []string{"a"}},
			"empty": {},
		}, byID)

		// GetObjects returns the same "" values for paths an object doesn't have
		entries, err := doc.GetObjects(ctx, &NxGetObjectOptions{Types: []string{"custom"}, Data: qData})
		require.NoError(t, err)
		require.Len(t, entries, 2)
		for _, entry := range entries {
			var data customListData
			require.NoError(t, UnmarshalListData(entry.Data, &data), entry.Info.Id)
			assert.Equal(t, byID[entry.Info.Id], data)
			if entry.Info.Id == "empty" {
				assert.Error(t, json.Unmarshal(entry.Data, &data), "plain json.Unmarshal fails on the empty values")
			}
		}
	})
}
