package httpserver

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func writeAdminJSON(w http.ResponseWriter, status int, v any) {
	writeJSON(w, status, normalizeAdminJSON(v))
}

func normalizeAdminJSON(v any) any {
	return normalizeAdminJSONValue(reflect.ValueOf(v))
}

func normalizeAdminJSONValue(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}

	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		return normalizeAdminJSONValue(v.Elem())
	}

	if v.CanInterface() {
		if raw, ok := v.Interface().(json.RawMessage); ok {
			return raw
		}
		if ts, ok := v.Interface().(pgtype.Timestamptz); ok {
			return timestampMillis(ts)
		}
		if ts, ok := v.Interface().(time.Time); ok {
			if ts.IsZero() {
				return nil
			}
			return ts.UnixMilli()
		}
		if marshaler, ok := v.Interface().(json.Marshaler); ok && !isAdminResponseStruct(v.Type()) {
			return marshaler
		}
	}

	switch v.Kind() {
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return jsonBytes(v.Bytes())
		}
		items := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			items[i] = normalizeAdminJSONValue(v.Index(i))
		}
		return items
	case reflect.Array:
		items := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			items[i] = normalizeAdminJSONValue(v.Index(i))
		}
		return items
	case reflect.Map:
		if v.IsNil() {
			return nil
		}
		out := make(map[string]any, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			key := iter.Key()
			if key.Kind() == reflect.String {
				out[key.String()] = normalizeAdminJSONValue(iter.Value())
			}
		}
		return out
	case reflect.Struct:
		if !isAdminResponseStruct(v.Type()) {
			return v.Interface()
		}
		return normalizeAdminStruct(v)
	default:
		return v.Interface()
	}
}

func timestampMillis(value pgtype.Timestamptz) any {
	if !value.Valid || value.Time.IsZero() {
		return nil
	}
	return value.Time.UnixMilli()
}

func normalizeAdminStruct(v reflect.Value) map[string]any {
	out := make(map[string]any, v.NumField())
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}

		name := jsonFieldName(field)
		if name == "" {
			continue
		}
		out[name] = normalizeAdminJSONValue(v.Field(i))
	}
	return out
}

func jsonFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return ""
	}
	if tag != "" {
		name, _, _ := strings.Cut(tag, ",")
		if name != "" {
			return name
		}
	}
	return field.Name
}

func jsonBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	if json.Valid(b) {
		return json.RawMessage(b)
	}
	return string(b)
}

func isAdminResponseStruct(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Kind() == reflect.Struct && strings.HasPrefix(t.PkgPath(), "uni-ai-api/backend/")
}
