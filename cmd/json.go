package cmd

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// printJSON marshals v with 2-space indent and writes to stdout.
// Nil slices are coerced to empty slices so the output is always
// a JSON array, never `null` — important for jq pipelines.
func printJSON(v any) error {
	v = coerceNilSlice(v)
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func coerceNilSlice(v any) any {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice && rv.IsNil() {
		return reflect.MakeSlice(rv.Type(), 0, 0).Interface()
	}
	return v
}
