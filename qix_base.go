package enigma

import (
	"encoding/json"
	"reflect"
)

// ensureEncodable prepares a parameter for sending. Byte arrays are sent as raw JSON. The engine rejects
// null parameters with a JSON parse error, so nil slices, maps and struct pointers are sent as empty arrays
// and objects instead.
func ensureEncodable(val interface{}) interface{} {
	if v, ok := val.([]byte); ok {
		val = json.RawMessage(v)
	}
	if raw, ok := val.(json.RawMessage); ok {
		if len(raw) == 0 {
			return json.RawMessage("{}")
		}
		return raw
	}
	value := reflect.ValueOf(val)
	switch value.Kind() {
	case reflect.Slice:
		if value.IsNil() {
			return reflect.MakeSlice(value.Type(), 0, 0).Interface()
		}
	case reflect.Map:
		if value.IsNil() {
			return reflect.MakeMap(value.Type()).Interface()
		}
	case reflect.Pointer:
		if value.IsNil() && value.Type().Elem().Kind() == reflect.Struct {
			return reflect.New(value.Type().Elem()).Interface()
		}
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
