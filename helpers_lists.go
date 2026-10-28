package enigma

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// The list helpers below are ported from @qlik/api. Each list is a session object with a fixed id,
// so getting the same list twice reuses the session object instead of creating a new one.

// GetOrCreateSessionObject returns the object with the id in props.Info.Id and creates it as a
// session object if it doesn't exist yet.
func (doc *Doc) GetOrCreateSessionObject(ctx context.Context, props *GenericObjectProperties) (*GenericObject, error) {
	if props == nil || props.Info == nil || props.Info.Id == "" {
		return nil, errors.New("enigma: GetOrCreateSessionObject needs props.Info.Id")
	}
	id := props.Info.Id
	obj, err := doc.GetObject(ctx, id)
	if !errors.Is(err, ErrObjectNotFound) {
		return obj, err
	}
	obj, err = doc.CreateSessionObjectWithID(ctx, props)
	if errors.Is(err, ErrIDTaken) {
		// Someone else created it in the meantime
		return doc.GetObject(ctx, id)
	}
	return obj, err
}

// SessionList is a session object holding a list of app objects, such as the sheet list.
// Use Items to get the list's content, and ChangedChannel to know when it changes.
type SessionList[T any] struct {
	*GenericObject
	// layout property holding the list, e.g. qAppObjectList
	layoutKey string
}

// Items returns the items currently in the list.
func (list *SessionList[T]) Items(ctx context.Context) ([]T, error) {
	raw, err := list.GetLayoutRaw(ctx)
	if err != nil {
		return nil, err
	}
	var layout map[string]json.RawMessage
	if err := json.Unmarshal(raw, &layout); err != nil {
		return nil, err
	}
	var content struct {
		Items []T `json:"qItems"`
	}
	if data, ok := layout[list.layoutKey]; ok {
		if err := json.Unmarshal(data, &content); err != nil {
			return nil, fmt.Errorf("enigma: reading %s: %w", list.layoutKey, err)
		}
	}
	return content.Items, nil
}

// GetSessionList returns a session list for the list definition in props, such as an AppObjectListDef
// for your own object type, and creates the session object if needed. T is the type of the list's items,
// for example *ListItem[MyData] for app object lists. ListItem reads qData with UnmarshalListData.
func GetSessionList[T any](ctx context.Context, doc *Doc, props *GenericObjectProperties) (*SessionList[T], error) {
	layoutKey, err := listLayoutKey(props)
	if err != nil {
		return nil, err
	}
	obj, err := doc.GetOrCreateSessionObject(ctx, props)
	if err != nil {
		return nil, err
	}
	return &SessionList[T]{GenericObject: obj, layoutKey: layoutKey}, nil
}

// listLayoutKey returns the layout property holding the list defined in props
func listLayoutKey(props *GenericObjectProperties) (string, error) {
	switch {
	case props == nil:
	case props.AppObjectListDef != nil:
		return "qAppObjectList", nil
	case props.BookmarkListDef != nil:
		return "qBookmarkList", nil
	case props.ChildListDef != nil:
		return "qChildList", nil
	case props.DimensionListDef != nil:
		return "qDimensionList", nil
	case props.ExtensionListDef != nil:
		return "qExtensionList", nil
	case props.FieldListDef != nil:
		return "qFieldList", nil
	case props.MeasureListDef != nil:
		return "qMeasureList", nil
	case props.MediaListDef != nil:
		return "qMediaList", nil
	case props.VariableListDef != nil:
		return "qVariableList", nil
	}
	return "", errors.New("enigma: GetSessionList needs a list definition such as props.AppObjectListDef")
}

func getListItems[T any](ctx context.Context, doc *Doc, props *GenericObjectProperties) ([]T, error) {
	list, err := GetSessionList[T](ctx, doc, props)
	if err != nil {
		return nil, err
	}
	return list.Items(ctx)
}

// ListItem is an entry in a list of app objects. Data holds the values the list's qData selects
// from the object's layout, so properties the engine evaluates, such as qDim/qLabelExpression,
// hold the evaluated value.
type ListItem[T any] struct {
	Info *NxInfo `json:"qInfo,omitempty"`
	// The object's qMeta, such as title, owner, published and privileges
	Meta json.RawMessage `json:"qMeta,omitempty"`
	Data T               `json:"qData"`
}

// UnmarshalJSON implements json.Unmarshaler, see UnmarshalListData for how Data is read.
func (item *ListItem[T]) UnmarshalJSON(data []byte) error {
	var raw struct {
		Info *NxInfo         `json:"qInfo"`
		Meta json.RawMessage `json:"qMeta"`
		Data json.RawMessage `json:"qData"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	item.Info, item.Meta = raw.Info, raw.Meta
	return UnmarshalListData(raw.Data, &item.Data)
}

// UnmarshalListData reads the qData of a list item into v, for example from the items of a custom
// list or from Doc.GetObjects. The engine returns "" for qData paths that don't exist in an object,
// whatever type the value would have, so UnmarshalListData leaves those values as zero values
// instead of failing to decode them.
func UnmarshalListData(data json.RawMessage, v any) error {
	if len(data) == 0 {
		return nil
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	for key, value := range values {
		if string(value) == `""` {
			delete(values, key)
		}
	}
	data, err := json.Marshal(values)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// Fields

// FieldListItem is a field in the field list.
type FieldListItem struct {
	*NxFieldDescription
	// Tables the field is in
	SrcTables []string `json:"qSrcTables,omitempty"`
}

// ExpandedFieldListItem is a field, or a field derived from it, in an expanded field list.
type ExpandedFieldListItem struct {
	Name      string   `json:"qName"`
	SrcTables []string `json:"qSrcTables,omitempty"`
	Tags      []string `json:"qTags,omitempty"`
	// Field is the field's description and nil for derived fields
	Field                 *FieldListItem `json:"-"`
	IsDerived             bool           `json:"isDerived,omitempty"`
	SourceField           string         `json:"sourceField,omitempty"`
	DerivedDefinitionName string         `json:"derivedDefinitionName,omitempty"`
}

// FieldListObject is a session object holding the app's field list.
type FieldListObject struct {
	*SessionList[*FieldListItem]
}

// Expand returns the fields in the list followed by the fields derived from each of them.
func (list *FieldListObject) Expand(ctx context.Context) ([]*ExpandedFieldListItem, error) {
	fields, err := list.Items(ctx)
	if err != nil {
		return nil, err
	}
	var result []*ExpandedFieldListItem
	for _, field := range fields {
		result = append(result, &ExpandedFieldListItem{Name: field.Name, SrcTables: field.SrcTables, Tags: field.Tags, Field: field})
		if field.DerivedFieldData == nil {
			continue
		}
		for _, derived := range field.DerivedFieldData.DerivedFieldLists {
			for _, def := range derived.FieldDefs {
				result = append(result, &ExpandedFieldListItem{
					Name:                  def.Name,
					SrcTables:             field.SrcTables,
					Tags:                  def.Tags,
					IsDerived:             true,
					SourceField:           field.Name,
					DerivedDefinitionName: derived.DerivedDefinitionName,
				})
			}
		}
	}
	return result, nil
}

func fieldListProps(id string, showHidden, definitionOnly bool) *GenericObjectProperties {
	return &GenericObjectProperties{
		Info: &NxInfo{Id: id, Type: id},
		FieldListDef: &FieldListDef{
			ShowHidden:         showHidden,
			ShowSrcTables:      true,
			ShowSemantic:       true,
			ShowDerivedFields:  true,
			ShowDefinitionOnly: definitionOnly,
		},
	}
}

func (doc *Doc) getFieldListObject(ctx context.Context, props *GenericObjectProperties) (*FieldListObject, error) {
	list, err := GetSessionList[*FieldListItem](ctx, doc, props)
	if err != nil {
		return nil, err
	}
	return &FieldListObject{list}, nil
}

// GetFieldList returns the app's fields, including the tables they are in and their derived fields.
func (doc *Doc) GetFieldList(ctx context.Context) ([]*FieldListItem, error) {
	return getListItems[*FieldListItem](ctx, doc, fieldListProps("FieldList", false, false))
}

// GetFieldListObject returns the session object holding the app's field list.
func (doc *Doc) GetFieldListObject(ctx context.Context) (*FieldListObject, error) {
	return doc.getFieldListObject(ctx, fieldListProps("FieldList", false, false))
}

// GetOnTheFlyListObject returns a field list that also includes fields that only exist as definitions (fields on the fly).
func (doc *Doc) GetOnTheFlyListObject(ctx context.Context) (*FieldListObject, error) {
	return doc.getFieldListObject(ctx, fieldListProps("FieldOnTheFlyList", false, true))
}

// GetOnTheFlyWithHiddenListObject returns a field list that also includes fields on the fly and hidden fields.
func (doc *Doc) GetOnTheFlyWithHiddenListObject(ctx context.Context) (*FieldListObject, error) {
	return doc.getFieldListObject(ctx, fieldListProps("FieldOnTheFlyWithHiddenList", true, true))
}

// Dimensions

// DimensionListData holds the values of a master dimension in the dimension list.
type DimensionListData struct {
	Title           string                  `json:"title"`
	Tags            []string                `json:"tags"`
	Grouping        string                  `json:"grouping"`
	Info            []*GenericDimensionInfo `json:"info"`
	LabelExpression string                  `json:"labelExpression"`
}

// DimensionListItem is a master dimension in the dimension list.
type DimensionListItem = ListItem[DimensionListData]

func dimensionListProps() *GenericObjectProperties {
	return &GenericObjectProperties{
		Info: &NxInfo{Id: "DimensionList", Type: "DimensionList"},
		DimensionListDef: &DimensionListDef{
			Type: "dimension",
			Data: json.RawMessage(`{"title":"/qMetaDef/title","tags":"/qMetaDef/tags","grouping":"/qDim/qGrouping","info":"/qDimInfos","labelExpression":"/qDim/qLabelExpression"}`),
		},
	}
}

// GetDimensionList returns the app's master dimensions.
func (doc *Doc) GetDimensionList(ctx context.Context) ([]*DimensionListItem, error) {
	return getListItems[*DimensionListItem](ctx, doc, dimensionListProps())
}

// GetDimensionListObject returns the session object holding the app's master dimensions.
func (doc *Doc) GetDimensionListObject(ctx context.Context) (*SessionList[*DimensionListItem], error) {
	return GetSessionList[*DimensionListItem](ctx, doc, dimensionListProps())
}

// Measures

// MeasureListData holds the values of a master measure in the measure list.
type MeasureListData struct {
	Title           string   `json:"title"`
	Tags            []string `json:"tags"`
	LabelExpression string   `json:"labelExpression"`
}

// MeasureListItem is a master measure in the measure list.
type MeasureListItem = ListItem[MeasureListData]

func measureListProps() *GenericObjectProperties {
	return &GenericObjectProperties{
		Info: &NxInfo{Id: "MeasureList", Type: "MeasureList"},
		MeasureListDef: &MeasureListDef{
			Type: "measure",
			Data: json.RawMessage(`{"title":"/qMetaDef/title","tags":"/qMetaDef/tags","labelExpression":"/qMeasure/qLabelExpression"}`),
		},
	}
}

// GetMeasureList returns the app's master measures.
func (doc *Doc) GetMeasureList(ctx context.Context) ([]*MeasureListItem, error) {
	return getListItems[*MeasureListItem](ctx, doc, measureListProps())
}

// GetMeasureListObject returns the session object holding the app's master measures.
func (doc *Doc) GetMeasureListObject(ctx context.Context) (*SessionList[*MeasureListItem], error) {
	return GetSessionList[*MeasureListItem](ctx, doc, measureListProps())
}

// Master objects

// MasterObjectListData holds the values of a master visualization in the master object list.
type MasterObjectListData struct {
	Name            json.RawMessage `json:"name"`
	LabelExpression string          `json:"labelExpression"`
	Visualization   json.RawMessage `json:"visualization"`
	Tags            []string        `json:"tags"`
}

// MasterObjectListItem is a master visualization in the master object list.
type MasterObjectListItem = ListItem[MasterObjectListData]

func masterObjectListProps() *GenericObjectProperties {
	return &GenericObjectProperties{
		Info: &NxInfo{Id: "MasterObjectList", Type: "MasterObjectList"},
		AppObjectListDef: &AppObjectListDef{
			Type: "masterobject",
			Data: json.RawMessage(`{"name":"/qMetaDef/title","labelExpression":"/labelExpression","visualization":"/visualization","tags":"/qMetaDef/tags"}`),
		},
	}
}

// GetMasterObjectList returns the app's master visualizations.
func (doc *Doc) GetMasterObjectList(ctx context.Context) ([]*MasterObjectListItem, error) {
	return getListItems[*MasterObjectListItem](ctx, doc, masterObjectListProps())
}

// GetMasterObjectListObject returns the session object holding the app's master visualizations.
func (doc *Doc) GetMasterObjectListObject(ctx context.Context) (*SessionList[*MasterObjectListItem], error) {
	return GetSessionList[*MasterObjectListItem](ctx, doc, masterObjectListProps())
}

// Variables

// VariableListData holds the values of a variable in the variable list.
type VariableListData struct {
	Tags []string `json:"tags"`
}

// VariableListItem is a variable in the variable list.
type VariableListItem struct {
	Name            string           `json:"qName,omitempty"`
	Description     string           `json:"qDescription,omitempty"`
	Definition      string           `json:"qDefinition,omitempty"`
	IsConfig        bool             `json:"qIsConfig,omitempty"`
	IsReserved      bool             `json:"qIsReserved,omitempty"`
	IsScriptCreated bool             `json:"qIsScriptCreated,omitempty"`
	Info            *NxInfo          `json:"qInfo,omitempty"`
	Meta            json.RawMessage  `json:"qMeta,omitempty"`
	Data            VariableListData `json:"qData"`
}

// UnmarshalJSON implements json.Unmarshaler, see UnmarshalListData for how Data is read.
func (item *VariableListItem) UnmarshalJSON(data []byte) error {
	type plain VariableListItem
	raw := struct {
		*plain
		Data json.RawMessage `json:"qData"`
	}{plain: (*plain)(item)}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	return UnmarshalListData(raw.Data, &item.Data)
}

func variableListProps() *GenericObjectProperties {
	return &GenericObjectProperties{
		Info: &NxInfo{Id: "VariableList", Type: "VariableList"},
		VariableListDef: &VariableListDef{
			Type:         "variable",
			ShowReserved: true,
			ShowConfig:   true,
			Data:         json.RawMessage(`{"tags":"/tags"}`),
		},
	}
}

// GetVariableList returns the app's variables, including reserved and configuration variables.
func (doc *Doc) GetVariableList(ctx context.Context) ([]*VariableListItem, error) {
	return getListItems[*VariableListItem](ctx, doc, variableListProps())
}

// GetVariableListObject returns the session object holding the app's variables.
func (doc *Doc) GetVariableListObject(ctx context.Context) (*SessionList[*VariableListItem], error) {
	return GetSessionList[*VariableListItem](ctx, doc, variableListProps())
}

// Bookmarks

// BookmarkListData holds the values of a bookmark in the bookmark list.
type BookmarkListData struct {
	Title           string          `json:"title"`
	Description     string          `json:"description"`
	SheetID         string          `json:"sheetId"`
	SelectionFields json.RawMessage `json:"selectionFields"`
	CreationDate    json.RawMessage `json:"creationDate"`
	GroupID         string          `json:"groupId"`
}

// BookmarkListItem is a bookmark in the bookmark list.
type BookmarkListItem = ListItem[BookmarkListData]

func bookmarkListProps() *GenericObjectProperties {
	return &GenericObjectProperties{
		Info: &NxInfo{Id: "BookmarkList", Type: "BookmarkList"},
		BookmarkListDef: &BookmarkListDef{
			Type: "bookmark",
			Data: json.RawMessage(`{"title":"/qMetaDef/title","description":"/qMetaDef/description","sheetId":"/sheetId","selectionFields":"/selectionFields","creationDate":"/creationDate","groupId":"/groupId"}`),
		},
	}
}

// GetBookmarkList returns the app's bookmarks.
func (doc *Doc) GetBookmarkList(ctx context.Context) ([]*BookmarkListItem, error) {
	return getListItems[*BookmarkListItem](ctx, doc, bookmarkListProps())
}

// GetBookmarkListObject returns the session object holding the app's bookmarks.
func (doc *Doc) GetBookmarkListObject(ctx context.Context) (*SessionList[*BookmarkListItem], error) {
	return GetSessionList[*BookmarkListItem](ctx, doc, bookmarkListProps())
}

// Snapshots

// SnapshotListData holds the values of a snapshot in the snapshot list.
type SnapshotListData struct {
	Title             string          `json:"title"`
	LibraryTitle      string          `json:"libraryTitle"`
	Description       string          `json:"description"`
	SourceObjectID    string          `json:"sourceObjectId"`
	VisualizationType string          `json:"visualizationType"`
	Timestamp         json.RawMessage `json:"timestamp"`
	SnapshotData      json.RawMessage `json:"snapshotData"`
	IsClone           bool            `json:"isClone"`
}

// SnapshotListItem is a snapshot in the snapshot list.
type SnapshotListItem = ListItem[SnapshotListData]

func snapshotListProps() *GenericObjectProperties {
	return &GenericObjectProperties{
		Info: &NxInfo{Id: "SnapshotList", Type: "SnapshotList"},
		BookmarkListDef: &BookmarkListDef{
			Type: "snapshot",
			Data: json.RawMessage(`{"title":"/title","libraryTitle":"/qMetaDef/title","description":"/qMetaDef/description","sourceObjectId":"/sourceObjectId","visualizationType":"/visualizationType","timestamp":"/timestamp","snapshotData":"/snapshotData","isClone":"/isClone"}`),
		},
	}
}

// GetSnapshotList returns the app's snapshots.
func (doc *Doc) GetSnapshotList(ctx context.Context) ([]*SnapshotListItem, error) {
	return getListItems[*SnapshotListItem](ctx, doc, snapshotListProps())
}

// GetSnapshotListObject returns the session object holding the app's snapshots.
func (doc *Doc) GetSnapshotListObject(ctx context.Context) (*SessionList[*SnapshotListItem], error) {
	return GetSessionList[*SnapshotListItem](ctx, doc, snapshotListProps())
}

// Sheets

// SheetListDataCellBounds is the position and size of a cell on a sheet, in percent of the sheet.
type SheetListDataCellBounds struct {
	X      Float64 `json:"x"`
	Y      Float64 `json:"y"`
	Width  Float64 `json:"width"`
	Height Float64 `json:"height"`
}

// SheetListDataCell is a visualization placed on a sheet.
type SheetListDataCell struct {
	Bounds  *SheetListDataCellBounds `json:"bounds,omitempty"`
	Col     Float64                  `json:"col"`
	Colspan Float64                  `json:"colspan"`
	Name    string                   `json:"name"`
	Row     Float64                  `json:"row"`
	Rowspan Float64                  `json:"rowspan"`
	Type    string                   `json:"type"`
}

// SheetListData holds the values of a sheet in the sheet list.
type SheetListData struct {
	Title                 string               `json:"title"`
	LabelExpression       string               `json:"labelExpression"`
	ShowCondition         json.RawMessage      `json:"showCondition"`
	Description           string               `json:"description"`
	DescriptionExpression string               `json:"descriptionExpression"`
	Thumbnail             json.RawMessage      `json:"thumbnail"`
	Cells                 []*SheetListDataCell `json:"cells"`
	Rank                  Float64              `json:"rank"`
	Columns               Float64              `json:"columns"`
	Rows                  Float64              `json:"rows"`
	GroupID               string               `json:"groupId"`
	Visible               bool                 `json:"visible"`
}

// SheetListItem is a sheet in the sheet list.
type SheetListItem = ListItem[SheetListData]

func sheetListProps() *GenericObjectProperties {
	return &GenericObjectProperties{
		Info: &NxInfo{Id: "SheetList", Type: "SheetList"},
		AppObjectListDef: &AppObjectListDef{
			Type: "sheet",
			Data: json.RawMessage(`{"title":"/qMetaDef/title","labelExpression":"/labelExpression","showCondition":"/showCondition","description":"/qMetaDef/description","descriptionExpression":"/descriptionExpression","thumbnail":"/thumbnail","cells":"/cells","rank":"/rank","columns":"/columns","rows":"/rows","groupId":"/groupId","visible":"/visible"}`),
		},
	}
}

// GetSheetList returns the app's sheets.
func (doc *Doc) GetSheetList(ctx context.Context) ([]*SheetListItem, error) {
	return getListItems[*SheetListItem](ctx, doc, sheetListProps())
}

// GetSheetListObject returns the session object holding the app's sheets.
func (doc *Doc) GetSheetListObject(ctx context.Context) (*SessionList[*SheetListItem], error) {
	return GetSessionList[*SheetListItem](ctx, doc, sheetListProps())
}

// Stories

// StoryListData holds the values of a story in the story list.
type StoryListData struct {
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Thumbnail   json.RawMessage `json:"thumbnail"`
	Rank        json.RawMessage `json:"rank"`
}

// StoryListItem is a story in the story list.
type StoryListItem = ListItem[StoryListData]

func storyListProps() *GenericObjectProperties {
	return &GenericObjectProperties{
		Info: &NxInfo{Id: "StoryList", Type: "StoryList"},
		AppObjectListDef: &AppObjectListDef{
			Type: "story",
			Data: json.RawMessage(`{"title":"/qMetaDef/title","description":"/qMetaDef/description","thumbnail":"/thumbnail","rank":"/rank"}`),
		},
	}
}

// GetStoryList returns the app's stories.
func (doc *Doc) GetStoryList(ctx context.Context) ([]*StoryListItem, error) {
	return getListItems[*StoryListItem](ctx, doc, storyListProps())
}

// GetStoryListObject returns the session object holding the app's stories.
func (doc *Doc) GetStoryListObject(ctx context.Context) (*SessionList[*StoryListItem], error) {
	return GetSessionList[*StoryListItem](ctx, doc, storyListProps())
}

// On-demand apps

// OdagListData holds the values of an on-demand app link or a dynamic view.
type OdagListData struct {
	OdagLinkRef json.RawMessage `json:"odagLinkRef"`
	Timestamp   json.RawMessage `json:"timestamp"`
}

// ODAGAppLinkListItem is an on-demand app navigation link in the app.
type ODAGAppLinkListItem = ListItem[OdagListData]

// DynamicAppViewListItem is a dynamic view in the app.
type DynamicAppViewListItem = ListItem[OdagListData]

func odagListProps(id, objectType string) *GenericObjectProperties {
	return &GenericObjectProperties{
		Info: &NxInfo{Id: id, Type: id},
		AppObjectListDef: &AppObjectListDef{
			Type: objectType,
			Data: json.RawMessage(`{"odagLinkRef":"/qMetaDef/odagLinkRef","timestamp":"/qMetaDef/timestamp"}`),
		},
	}
}

// GetODAGAppLinkList returns the app's on-demand app navigation links.
func (doc *Doc) GetODAGAppLinkList(ctx context.Context) ([]*ODAGAppLinkListItem, error) {
	return getListItems[*ODAGAppLinkListItem](ctx, doc, odagListProps("ODAGAppLinkList", "odagapplink"))
}

// GetODAGAppLinkListObject returns the session object holding the app's on-demand app navigation links.
func (doc *Doc) GetODAGAppLinkListObject(ctx context.Context) (*SessionList[*ODAGAppLinkListItem], error) {
	return GetSessionList[*ODAGAppLinkListItem](ctx, doc, odagListProps("ODAGAppLinkList", "odagapplink"))
}

// GetDynamicAppViewList returns the app's dynamic views.
func (doc *Doc) GetDynamicAppViewList(ctx context.Context) ([]*DynamicAppViewListItem, error) {
	return getListItems[*DynamicAppViewListItem](ctx, doc, odagListProps("DynamicAppViewList", "dynamicappview"))
}

// GetDynamicAppViewListObject returns the session object holding the app's dynamic views.
func (doc *Doc) GetDynamicAppViewListObject(ctx context.Context) (*SessionList[*DynamicAppViewListItem], error) {
	return GetSessionList[*DynamicAppViewListItem](ctx, doc, odagListProps("DynamicAppViewList", "dynamicappview"))
}
