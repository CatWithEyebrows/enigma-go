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
}
