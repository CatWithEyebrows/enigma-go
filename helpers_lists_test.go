package enigma

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakeCall struct {
	method string
	params []interface{}
}

// connectFakeEngine returns a Doc whose calls are answered by respond instead of an engine
func connectFakeEngine(t *testing.T, respond func(method string, params []interface{}) string) (*Doc, *[]fakeCall) {
	calls := &[]fakeCall{}
	socket, _ := NewMockSocket("")
	dialer := Dialer{
		CreateSocket: func(context.Context, string, http.Header) (Socket, error) { return socket, nil },
		Interceptors: []Interceptor{func(ctx context.Context, invocation *Invocation, next InterceptorContinuation) *InvocationResponse {
			*calls = append(*calls, fakeCall{invocation.Method, invocation.Params})
			return &InvocationResponse{Result: json.RawMessage(respond(invocation.Method, invocation.Params))}
		}},
	}
	remoteObject, err := dialer.DialRaw(context.Background(), "ws://fake", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { remoteObject.DisconnectFromServer() })
	return &Doc{remoteObject}, calls
}

func methods(calls []fakeCall) []string {
	result := make([]string, len(calls))
	for i, call := range calls {
		result[i] = call.method
	}
	return result
}

const emptyObject = `{"qReturn":{}}`

func object(handle int, id string) string {
	ret, _ := json.Marshal(map[string]interface{}{"qReturn": ObjectInterface{Type: "GenericObject", Handle: handle, GenericType: "SheetList", GenericId: id}})
	return string(ret)
}

func TestGetOrCreateSessionObjectExisting(t *testing.T) {
	doc, calls := connectFakeEngine(t, func(method string, params []interface{}) string {
		return object(5, "SheetList")
	})
	obj, err := doc.GetOrCreateSessionObject(context.Background(), sheetListProps())
	assert.NoError(t, err)
	assert.Equal(t, 5, obj.Handle)
	assert.Equal(t, []string{"GetObject"}, methods(*calls))
}

func TestGetOrCreateSessionObjectMissing(t *testing.T) {
	doc, calls := connectFakeEngine(t, func(method string, params []interface{}) string {
		if method == "GetObject" {
			return emptyObject
		}
		return object(6, "SheetList")
	})
	obj, err := doc.GetOrCreateSessionObject(context.Background(), sheetListProps())
	assert.NoError(t, err)
	assert.Equal(t, 6, obj.Handle)
	assert.Equal(t, []string{"GetObject", "CreateSessionObject"}, methods(*calls))
}

func TestGetOrCreateSessionObjectCreatedConcurrently(t *testing.T) {
	getObjectCalls := 0
	doc, calls := connectFakeEngine(t, func(method string, params []interface{}) string {
		switch method {
		case "GetObject":
			getObjectCalls++
			if getObjectCalls == 1 {
				return emptyObject
			}
			return object(7, "SheetList")
		case "CreateSessionObject":
			// Someone else created SheetList first, so the engine picks a random id
			return object(8, "random-id")
		default:
			return `{"qSuccess":true}`
		}
	})
	obj, err := doc.GetOrCreateSessionObject(context.Background(), sheetListProps())
	assert.NoError(t, err)
	assert.Equal(t, 7, obj.Handle)
	assert.Equal(t, []string{"GetObject", "CreateSessionObject", "DestroySessionObject", "GetObject"}, methods(*calls))
	assert.Equal(t, []interface{}{"random-id"}, (*calls)[2].params)
}

func TestGetOrCreateSessionObjectNeedsID(t *testing.T) {
	doc, calls := connectFakeEngine(t, func(string, []interface{}) string { return emptyObject })
	_, err := doc.GetOrCreateSessionObject(context.Background(), &GenericObjectProperties{Info: &NxInfo{Type: "SheetList"}})
	assert.Error(t, err)
	assert.Empty(t, *calls)
}

func TestSessionListItems(t *testing.T) {
	layout := `{"qLayout":{"qInfo":{"qId":"SheetList"},"qAppObjectList":{"qItems":[
		{"qInfo":{"qId":"s1","qType":"sheet"},"qMeta":{"title":"One","published":true},"qData":{"title":"One","rank":1.5,"columns":24,"cells":[{"name":"c1","type":"barchart","col":0,"row":0,"colspan":12,"rowspan":6,"bounds":{"x":0,"y":0,"width":50,"height":50}}]}},
		{"qInfo":{"qId":"s2","qType":"sheet"},"qData":{"title":"Two","visible":"","rank":"","cells":"","thumbnail":""}}
	]}}}`
	doc, _ := connectFakeEngine(t, func(method string, params []interface{}) string {
		if method == "GetLayout" {
			return layout
		}
		return object(5, "SheetList")
	})
	sheets, err := doc.GetSheetList(context.Background())
	assert.NoError(t, err)
	assert.Len(t, sheets, 2)
	assert.Equal(t, "s1", sheets[0].Info.Id)
	assert.Equal(t, "One", sheets[0].Data.Title)
	assert.Equal(t, Float64(1.5), sheets[0].Data.Rank)
	assert.Equal(t, "barchart", sheets[0].Data.Cells[0].Type)
	assert.Equal(t, Float64(50), sheets[0].Data.Cells[0].Bounds.Width)
	assert.JSONEq(t, `{"title":"One","published":true}`, string(sheets[0].Meta))
	// The engine returns "" for properties an object doesn't have
	assert.Equal(t, SheetListData{Title: "Two"}, sheets[1].Data)

	// A list the layout doesn't contain is empty
	dimensions, err := doc.GetDimensionList(context.Background())
	assert.NoError(t, err)
	assert.Empty(t, dimensions)
}

func TestFieldListExpand(t *testing.T) {
	layout := `{"qLayout":{"qFieldList":{"qItems":[
		{"qName":"OrderDate","qSrcTables":["Orders"],"qTags":["$date"],"qDerivedFieldData":{"qDerivedFieldLists":[
			{"qDerivedDefinitionName":"Calendar","qFieldDefs":[{"qName":"OrderDate.Calendar.Year","qTags":["$year"]},{"qName":"OrderDate.Calendar.Month"}]}
		]}},
		{"qName":"ID","qSrcTables":["Orders"]}
	]}}}`
	doc, _ := connectFakeEngine(t, func(method string, params []interface{}) string {
		if method == "GetLayout" {
			return layout
		}
		return object(5, "FieldList")
	})
	list, err := doc.GetFieldListObject(context.Background())
	assert.NoError(t, err)
	fields, err := list.Expand(context.Background())
	assert.NoError(t, err)

	names := make([]string, len(fields))
	for i, field := range fields {
		names[i] = field.Name
	}
	assert.Equal(t, []string{"OrderDate", "OrderDate.Calendar.Year", "OrderDate.Calendar.Month", "ID"}, names)
	assert.False(t, fields[0].IsDerived)
	assert.Equal(t, "OrderDate", fields[0].Field.Name)
	assert.Equal(t, &ExpandedFieldListItem{
		Name:                  "OrderDate.Calendar.Year",
		SrcTables:             []string{"Orders"},
		Tags:                  []string{"$year"},
		IsDerived:             true,
		SourceField:           "OrderDate",
		DerivedDefinitionName: "Calendar",
	}, fields[1])
}
