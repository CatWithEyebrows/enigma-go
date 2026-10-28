package enigma

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateHelpersIntegration(t *testing.T) {
	doc := connectSessionApp(t)
	ctx := context.Background()
	props := func(obj *GenericObject) map[string]any {
		raw, err := obj.GetPropertiesRaw(ctx)
		require.NoError(t, err)
		var result map[string]any
		require.NoError(t, json.Unmarshal(raw, &result))
		return result
	}

	// Sheets
	sheet, err := doc.CreateSheet(ctx, SheetOptions{Title: "Overview", Description: "First", CustomRowBase: 8,
		Thumbnail: "https://example.com/a.png", Properties: map[string]any{"labelExpression": "='Hi'"}})
	require.NoError(t, err)
	sheetProps := props(sheet)
	assert.Equal(t, map[string]any{"title": "Overview", "description": "First"}, sheetProps["qMetaDef"])
	assert.Equal(t, -1.0, sheetProps["rank"])
	assert.Equal(t, 24.0, sheetProps["columns"])
	assert.Equal(t, 12.0, sheetProps["rows"])
	assert.Equal(t, 8.0, sheetProps["customRowBase"])
	assert.Equal(t, "='Hi'", sheetProps["labelExpression"])
	assert.NotContains(t, sheetProps, "gridResolution")
	sheets, err := doc.GetSheetList(ctx)
	require.NoError(t, err)
	require.Len(t, sheets, 1)
	assert.Equal(t, "Overview", sheets[0].Data.Title)
	assert.JSONEq(t, `{"qStaticContentUrl":{"qUrl":"https%3A//example.com/a.png"}}`, string(sheets[0].Data.Thumbnail))

	rank := 2.0
	withID, err := doc.CreateSheet(ctx, SheetOptions{ID: "sheet-2", Rank: &rank})
	require.NoError(t, err)
	assert.Equal(t, "sheet-2", withID.GenericId)
	assert.Equal(t, 2.0, props(withID)["rank"])
	_, err = doc.CreateSheet(ctx, SheetOptions{ID: "sheet-2"})
	assert.ErrorIs(t, err, ErrIDTaken)

	// Master objects
	_, err = doc.CreateMasterObject(ctx, MasterObjectOptions{Title: "Chart", Tags: []string{"a"},
		Properties: map[string]any{"visualization": "barchart"}})
	require.NoError(t, err)
	_, err = doc.CreateMasterObject(ctx, MasterObjectOptions{Title: "No tags"})
	require.NoError(t, err)
	masters, err := doc.GetMasterObjectList(ctx)
	require.NoError(t, err)
	require.Len(t, masters, 2)
	tags := map[string][]string{}
	for _, master := range masters {
		var name string
		require.NoError(t, json.Unmarshal(master.Data.Name, &name))
		tags[name] = master.Data.Tags
	}
	assert.Equal(t, map[string][]string{"Chart": {"a"}, "No tags": {}}, tags)

	// Stories and slides
	story, err := doc.CreateStory(ctx, StoryOptions{Title: "Story", Description: "About"})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"title": "Story", "description": "About"}, props(story.GenericObject)["qMetaDef"])
	_, err = doc.GetStory(ctx, sheet.GenericId)
	assert.Error(t, err)
	again, err := doc.GetStory(ctx, story.GenericId)
	require.NoError(t, err)
	assert.Equal(t, story.Handle, again.Handle)

	first, err := story.CreateSlide(ctx, map[string]any{"rank": 1, "title": "First"})
	require.NoError(t, err)
	_, err = story.CreateSlide(ctx, map[string]any{"rank": 2, "title": "Second"})
	require.NoError(t, err)
	_, err = first.CreateChildRaw(ctx, map[string]any{"qInfo": map[string]any{"qType": "slideitem"}, "title": "Item"}, nil)
	require.NoError(t, err)

	ranks := func(story *Story) map[string]float64 {
		slides, err := story.slideRanks(ctx)
		require.NoError(t, err)
		result := map[string]float64{}
		for _, slide := range slides {
			obj, err := story.GetSlide(ctx, slide.id)
			require.NoError(t, err)
			result[props(obj.GenericObject)["title"].(string)+"/"+slide.id] = slide.rank
		}
		return result
	}

	copyOfFirst, err := first.Duplicate(ctx)
	require.NoError(t, err)
	copyProps := props(copyOfFirst.GenericObject)
	assert.Equal(t, "First", copyProps["title"])
	assert.Equal(t, 1.5, copyProps["rank"], "between the slide and the next one")
	assert.Equal(t, copyOfFirst.GenericId, copyProps["qInfo"].(map[string]any)["qId"], "keeps its own id")
	children, err := copyOfFirst.GetChildInfos(ctx)
	require.NoError(t, err)
	assert.Len(t, children, 1, "the slide's items are copied")

	last, err := story.GetSlide(ctx, func() string {
		slides, err := story.slideRanks(ctx)
		require.NoError(t, err)
		return slides[len(slides)-1].id
	}())
	require.NoError(t, err)
	copyOfLast, err := last.Duplicate(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3.0, props(copyOfLast.GenericObject)["rank"], "one more than the last slide")
	assert.Len(t, ranks(story), 4)

	clone, err := story.Clone(ctx)
	require.NoError(t, err)
	assert.NotEqual(t, story.GenericId, clone.GenericId)
	assert.Len(t, ranks(clone), 4, "the clone has the same slides")
}
