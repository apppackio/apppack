package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
)

// printJSON marshals v with 2-space indent and writes to stdout.
// Nil slices are coerced to empty slices so the output is always
// a JSON array, never `null` — important for jq pipelines.
func printJSON(v any) error {
	return fprintJSON(os.Stdout, v)
}

// fprintJSON is printJSON with the destination named, so a command that has
// been converted can write through cmd.OutOrStdout() and a test can read it
// back.
func fprintJSON(w io.Writer, v any) error {
	v = coerceNilSlice(v)

	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(w, string(data))

	return err
}

func coerceNilSlice(v any) any {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice && rv.IsNil() {
		return reflect.MakeSlice(rv.Type(), 0, 0).Interface()
	}
	return v
}
