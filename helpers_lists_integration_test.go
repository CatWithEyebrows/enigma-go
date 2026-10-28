package enigma

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// connectSessionApp opens a session app on the Qlik Cloud tenant in QCS_HOST, authenticating
// with the token in QCS_API_KEY. The test is skipped when they are not set.
func connectSessionApp(t *testing.T) *Doc {
	host, token := os.Getenv("QCS_HOST"), os.Getenv("QCS_API_KEY")
	if host == "" || token == "" {
		t.Skip("set QCS_HOST and QCS_API_KEY to run against a Qlik Cloud tenant")
	}
	ctx := context.Background()
	global, err := Dialer{}.Dial(ctx, fmt.Sprintf("wss://%s/app/SessionApp_%v", host, rand.Int()), http.Header{
		"Authorization": []string{"Bearer " + token},
	})
	require.NoError(t, err)
	t.Cleanup(func() { global.DisconnectFromServer() })
	doc, err := global.GetActiveDoc(ctx)
	require.NoError(t, err)
	return doc
}

const listsTestScript = `
Orders: Load RecNo() as ID, Date(MakeDate(2024, 1, 1) + RecNo()) as OrderDate AutoGenerate 10;
Calendar: DECLARE FIELD DEFINITION TAGGED '$date'
Fields
  Year($1) as Year Tagged ('$numeric'),
  Month($1) as Month Tagged ('$numeric');
DERIVE FIELDS FROM FIELDS OrderDate USING Calendar;
`

func TestListsIntegration(t *testing.T) {
	doc := connectSessionApp(t)
	ctx := context.Background()
	err := doc.SetScript(ctx, listsTestScript)
	require.NoError(t, err)
	ok, err := doc.DoReload(ctx, 0, false, false)
	require.NoError(t, err)
	require.True(t, ok)

	_, err = doc.CreateDimensionRaw(ctx, map[string]any{
		"qInfo":    map[string]any{"qType": "dimension"},
		"qMetaDef": map[string]any{"title": "Order", "tags": []string{"t1"}},
		"qDim":     map[string]any{"qGrouping": "N", "qFieldDefs": []string{"ID"}, "qLabelExpression": "='Order'"},
	})
	require.NoError(t, err)
	_, err = doc.CreateMeasureRaw(ctx, map[string]any{
		"qInfo":    map[string]any{"qType": "measure"},
		"qMetaDef": map[string]any{"title": "Orders", "tags": []string{"t2"}},
		"qMeasure": map[string]any{"qDef": "Count(ID)", "qLabelExpression": "='Orders'"},
	})
	require.NoError(t, err)
	_, err = doc.CreateObjectRaw(ctx, map[string]any{
		"qInfo":           map[string]any{"qId": "sheet-1", "qType": "sheet"},
		"qMetaDef":        map[string]any{"title": "Overview", "description": "First sheet"},
		"rank":            1.5,
		"columns":         24,
		"rows":            12,
		"thumbnail":       map[string]any{"qStaticContentUrlDef": map[string]any{}},
		"cells":           []map[string]any{{"name": "chart-1", "type": "barchart", "col": 0, "row": 0, "colspan": 12, "rowspan": 6, "bounds": map[string]any{"x": 0, "y": 0, "width": 50, "height": 50}}},
		"qChildListDef":   map[string]any{"qData": map[string]any{"title": "/title"}},
		"labelExpression": "='Overview'",
	})
	require.NoError(t, err)
	_, err = doc.CreateObjectRaw(ctx, map[string]any{
		"qInfo":    map[string]any{"qId": "story-1", "qType": "story"},
		"qMetaDef": map[string]any{"title": "Story", "description": "A story"},
		"rank":     1,
	})
	require.NoError(t, err)
	_, err = doc.CreateObjectRaw(ctx, map[string]any{
		"qInfo":         map[string]any{"qId": "master-1", "qType": "masterobject"},
		"qMetaDef":      map[string]any{"title": "Master chart", "tags": []string{"t3"}},
		"visualization": "barchart",
	})
	require.NoError(t, err)
	_, err = doc.CreateBookmarkRaw(ctx, map[string]any{
		"qInfo":        map[string]any{"qId": "bookmark-1", "qType": "bookmark"},
		"qMetaDef":     map[string]any{"title": "Bookmark", "description": "A bookmark"},
		"sheetId":      "sheet-1",
		"creationDate": "2026-10-28T00:00:00.000Z",
	})
	require.NoError(t, err)
	_, err = doc.CreateVariableExRaw(ctx, map[string]any{
		"qInfo":       map[string]any{"qType": "variable"},
		"qName":       "vTest",
		"qDefinition": "=1",
		"tags":        []string{"t4"},
	})
	require.NoError(t, err)

	t.Run("fields", func(t *testing.T) {
		fields, err := doc.GetFieldList(ctx)
		require.NoError(t, err)
		names := map[string]*FieldListItem{}
		for _, field := range fields {
			names[field.Name] = field
		}
		require.Contains(t, names, "OrderDate")
		assert.Equal(t, []string{"Orders"}, names["OrderDate"].SrcTables)

		list, err := doc.GetFieldListObject(ctx)
		require.NoError(t, err)
		expanded, err := list.Expand(ctx)
		require.NoError(t, err)
		var derived []string
		for _, field := range expanded {
			if field.IsDerived {
				assert.Equal(t, "OrderDate", field.SourceField)
				assert.Equal(t, "Calendar", field.DerivedDefinitionName)
				derived = append(derived, field.Name)
			}
		}
		assert.Equal(t, []string{"OrderDate.Calendar.Year", "OrderDate.Calendar.Month"}, derived)

		for _, get := range []func(context.Context) (*FieldListObject, error){doc.GetOnTheFlyListObject, doc.GetOnTheFlyWithHiddenListObject} {
			list, err := get(ctx)
			require.NoError(t, err)
			_, err = list.Items(ctx)
			assert.NoError(t, err)
		}
	})

	t.Run("dimensions", func(t *testing.T) {
		items, err := doc.GetDimensionList(ctx)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, DimensionListData{Title: "Order", Tags: []string{"t1"}, Grouping: "N", LabelExpression: "Order", Info: items[0].Data.Info}, items[0].Data)
		assert.NotEmpty(t, items[0].Data.Info)
	})

	t.Run("measures", func(t *testing.T) {
		items, err := doc.GetMeasureList(ctx)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, MeasureListData{Title: "Orders", Tags: []string{"t2"}, LabelExpression: "Orders"}, items[0].Data)
	})

	t.Run("sheets", func(t *testing.T) {
		items, err := doc.GetSheetList(ctx)
		require.NoError(t, err)
		require.Len(t, items, 1)
		sheet := items[0].Data
		assert.Equal(t, "sheet-1", items[0].Info.Id)
		assert.Equal(t, "Overview", sheet.Title)
		assert.Equal(t, "First sheet", sheet.Description)
		assert.Equal(t, "='Overview'", sheet.LabelExpression, "plain properties are not evaluated")
		assert.Equal(t, Float64(1.5), sheet.Rank)
		assert.Equal(t, Float64(24), sheet.Columns)
		assert.Equal(t, Float64(12), sheet.Rows)
		require.Len(t, sheet.Cells, 1)
		assert.Equal(t, "barchart", sheet.Cells[0].Type)
		assert.Equal(t, Float64(50), sheet.Cells[0].Bounds.Width)
		assert.NotEmpty(t, sheet.Thumbnail)
	})

	t.Run("stories", func(t *testing.T) {
		items, err := doc.GetStoryList(ctx)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "Story", items[0].Data.Title)
		assert.Equal(t, "A story", items[0].Data.Description)
		assert.JSONEq(t, "1", string(items[0].Data.Rank))
	})

	t.Run("master objects", func(t *testing.T) {
		items, err := doc.GetMasterObjectList(ctx)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.JSONEq(t, `"Master chart"`, string(items[0].Data.Name))
		assert.JSONEq(t, `"barchart"`, string(items[0].Data.Visualization))
		assert.Equal(t, []string{"t3"}, items[0].Data.Tags)
	})

	t.Run("bookmarks", func(t *testing.T) {
		items, err := doc.GetBookmarkList(ctx)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "Bookmark", items[0].Data.Title)
		assert.Equal(t, "A bookmark", items[0].Data.Description)
		assert.Equal(t, "sheet-1", items[0].Data.SheetID)
		assert.JSONEq(t, `"2026-10-28T00:00:00.000Z"`, string(items[0].Data.CreationDate))
	})

	t.Run("variables", func(t *testing.T) {
		items, err := doc.GetVariableList(ctx)
		require.NoError(t, err)
		var found *VariableListItem
		for _, item := range items {
			if item.Name == "vTest" {
				found = item
			}
		}
		require.NotNil(t, found)
		assert.Equal(t, "=1", found.Definition)
		assert.Equal(t, []string{"t4"}, found.Data.Tags)
	})

	t.Run("empty lists", func(t *testing.T) {
		snapshots, err := doc.GetSnapshotList(ctx)
		assert.NoError(t, err)
		assert.Empty(t, snapshots)
		links, err := doc.GetODAGAppLinkList(ctx)
		assert.NoError(t, err)
		assert.Empty(t, links)
		views, err := doc.GetDynamicAppViewList(ctx)
		assert.NoError(t, err)
		assert.Empty(t, views)
	})

	t.Run("one session object per list", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := doc.GetMeasureListObject(ctx)
				assert.NoError(t, err)
			}()
		}
		wg.Wait()
		lists, err := doc.GetObjects(ctx, &NxGetObjectOptions{Types: []string{"MeasureList"}, IncludeSessionObjects: true})
		require.NoError(t, err)
		require.Len(t, lists, 1)
		assert.Equal(t, "MeasureList", lists[0].Info.Id)
	})

	t.Run("list changes", func(t *testing.T) {
		list, err := doc.GetSheetListObject(ctx)
		require.NoError(t, err)
		_, err = list.Items(ctx)
		require.NoError(t, err)
		changed := list.ChangedChannel()
		defer list.RemoveChangeChannel(changed)

		_, err = doc.CreateObjectRaw(ctx, map[string]any{"qInfo": map[string]any{"qId": "sheet-2", "qType": "sheet"}, "qMetaDef": map[string]any{"title": "Second"}})
		require.NoError(t, err)
		select {
		case <-changed:
		case <-time.After(10 * time.Second):
			t.Fatal("sheet list did not change")
		}
		items, err := list.Items(ctx)
		require.NoError(t, err)
		assert.Len(t, items, 2)
	})
}
