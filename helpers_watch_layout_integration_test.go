package enigma

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatchLayoutIntegration(t *testing.T) {
	doc := connectSessionApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, doc.SetScript(ctx, "T: Load RecNo() as A AutoGenerate 5;"))
	ok, err := doc.DoReload(ctx, 0, false, false)
	require.NoError(t, err)
	require.True(t, ok)

	obj, err := doc.CreateSessionObject(ctx, &GenericObjectProperties{
		Info:         &NxInfo{Type: "total"},
		HyperCubeDef: &HyperCubeDef{Measures: []*NxMeasure{{Def: &NxInlineMeasureDef{Def: "Sum(A)"}}}},
	})
	require.NoError(t, err)
	total := func(update LayoutUpdate[*GenericObjectLayout]) string {
		require.NoError(t, update.Err)
		return update.Layout.HyperCube.GrandTotalRow[0].Text
	}
	updates := obj.WatchLayout(ctx)
	assert.Equal(t, "15", total(receive(t, updates)))

	field, err := doc.GetField(ctx, "A", "")
	require.NoError(t, err)
	_, err = field.Select(ctx, "2", false, 0)
	require.NoError(t, err)
	assert.Equal(t, "2", total(receive(t, updates)))

	// Several changes before reading only send the latest layout
	for _, value := range []string{"3", "4", "5"} {
		_, err = field.Select(ctx, value, false, 0)
		require.NoError(t, err)
	}
	waitForTotal := func(want string) {
		for update := range updates {
			if total(update) == want {
				return
			}
			assert.Contains(t, []string{"3", "4"}, total(update), "only layouts that were current when sent")
		}
	}
	waitForTotal("5")

	// Any getter works, such as a list's items
	sheets, err := doc.GetSheetListObject(ctx)
	require.NoError(t, err)
	items := WatchLayout(ctx, sheets.RemoteObject, sheets.Items)
	assert.Empty(t, receive(t, items).Layout)
	_, err = doc.CreateObjectRaw(ctx, map[string]any{"qInfo": map[string]any{"qType": "sheet"}, "qMetaDef": map[string]any{"title": "New"}})
	require.NoError(t, err)
	update := receive(t, items)
	require.NoError(t, update.Err)
	require.Len(t, update.Layout, 1)
	assert.Equal(t, "New", update.Layout[0].Data.Title)

	_, err = doc.DestroySessionObject(ctx, obj.GenericId)
	require.NoError(t, err)
	assertClosed(t, updates)
	cancel()
	assertClosed(t, items)
}
