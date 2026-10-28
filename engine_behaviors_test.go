package enigma

import (
	"context"
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
}
