package ui

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/spf13/pflag"
)

func containsFlagByName(flagNames []string, flag *pflag.Flag) bool {
	for _, name := range flagNames {
		if name == flag.Name {
			return true
		}
	}
	return false
}

type fieldOptions struct {
	Name         string
	Transform    string
	TransformArg string
}

// parseTag takes the tag string and parses it into FieldOptions
func parseTag(tag string) *fieldOptions {
	parts := strings.Split(tag, ";")
	f := fieldOptions{
		Name: parts[0],
	}
	if len(parts) == 1 {
		return &f
	}
	transformParts := strings.Split(parts[1], ":")
	f.Transform = transformParts[0]
	if len(transformParts) == 1 {
		return &f
	}
	f.TransformArg = transformParts[1]
	return &f
}

// FlagsToStruct applies flag to a Struct using `flag` tags
func FlagsToStruct(s interface{}, flags *pflag.FlagSet) error {
	ref := reflect.ValueOf(s).Elem()
	fields := reflect.VisibleFields(ref.Type())
	// get a list of all flags present in the command
	flagsUsed := []string{}
	flags.Visit(func(flag *pflag.Flag) {
		flagsUsed = append(flagsUsed, flag.Name)
	})
	for i, field := range fields {
		// get the flag tag for the field
		tag := parseTag(field.Tag.Get("flag"))
		flag := flags.Lookup(tag.Name)
		// skip fields without a tag, missing flags, or flags that weren't used
		if tag.Name == "" || flag == nil || !containsFlagByName(flagsUsed, flag) {
			continue
		}
		// set the field to the value of the flag
		switch field.Type.Kind() {
		case reflect.String:
			val := flag.Value.String()
			if tag.Transform == "fmtString" {
				val = fmt.Sprintf(tag.TransformArg, val)
			}
			ref.Field(i).SetString(val)
		case reflect.Bool:
			val, err := flags.GetBool(tag.Name)
			if err != nil {
				return err
			}
			if tag.Transform == "negate" {
				val = !val
			}
			ref.Field(i).SetBool(val)
		case reflect.Int:
			val, err := flags.GetInt(tag.Name)
			if err != nil {
				return err
			}
			ref.Field(i).SetInt(int64(val))
		case reflect.Slice:
			if field.Type.Elem().Kind() != reflect.String {
				return fmt.Errorf("unsupported slice type %s", field.Type.Elem().Kind())
			}
			ref.Field(i).Set(reflect.ValueOf(strings.Split(flag.Value.String(), ",")))
		default:
			return fmt.Errorf("unsupported type %s", field.Type.Kind())
		}
	}
	return nil
}
