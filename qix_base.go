package enigma

import (
	"encoding/json"
	"reflect"
)

// ensureEncodable prepares a parameter for sending. Byte arrays are sent as raw JSON. Go encodes nil slices
// as null, which the engine rejects for array parameters, so they are sent as empty arrays instead. Other
// nil values are still sent as null, which optional object parameters such as qPropForThis rely on.
func ensureEncodable(val interface{}) interface{} {
	switch v := val.(type) {
	case []byte:
		return json.RawMessage(v)
	case json.RawMessage:
		return v
	}
	value := reflect.ValueOf(val)
	if value.Kind() == reflect.Slice && value.IsNil() {
		return reflect.MakeSlice(value.Type(), 0, 0).Interface()
	}
	return val
}

// Prepares all parameters with ensureEncodable
func ensureAllEncodable(params []interface{}) []interface{} {
	result := make([]interface{}, len(params))
	for i := range params {
		result[i] = ensureEncodable(params[i])
	}
	return result
}
