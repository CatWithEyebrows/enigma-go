package enigma

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateHelpersProperties(t *testing.T) {
	doc, calls := connectFakeEngine(t, func(method string, params []interface{}) string {
		return object(5, "new")
	})
	ctx := context.Background()
	sent := func() string {
		require.Len(t, *calls, 1)
		data, err := json.Marshal((*calls)[0].params[0])
		require.NoError(t, err)
		*calls = nil
		return string(data)
	}

	_, err := doc.CreateSheet(ctx, SheetOptions{})
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"qInfo": {"qType": "sheet"},
		"qMetaDef": {"title": "", "description": ""},
		"rank": -1,
		"thumbnail": {"qStaticContentUrlDef": {}},
		"columns": 24,
		"rows": 12,
		"cells": [],
		"qChildListDef": {"qData": {"title": "/title"}}
	}`, sent())

	rank := 3.0
	_, err = doc.CreateStory(ctx, StoryOptions{Title: "T", Description: "D", Thumbnail: "u", Rank: &rank, Properties: map[string]any{"x": 1}})
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"qInfo": {"qType": "story"},
		"qMetaDef": {"title": "T", "description": "D"},
		"rank": 3,
		"thumbnail": {"qStaticContentUrlDef": {"qUrl": "u"}},
		"qChildListDef": {"qData": {"title": "/title", "rank": "/rank"}},
		"x": 1
	}`, sent())

	_, err = doc.CreateMasterObject(ctx, MasterObjectOptions{ID: "new"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"qInfo": {"qType": "masterobject", "qId": "new"}, "qMetaDef": {"title": "", "description": "", "tags": []}}`, sent())
}
