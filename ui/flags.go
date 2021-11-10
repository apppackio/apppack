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
		tag := field.Tag.Get("flag")
		flag := flags.Lookup(tag)
		// skip fields without a tag, missing flags, or flags that weren't used
		if tag == "" || flag == nil || !containsFlagByName(flagsUsed, flag) {
			continue
		}
		// set the field to the value of the flag
		switch field.Type.Kind() {
		case reflect.String:
			ref.Field(i).SetString(flag.Value.String())
		case reflect.Bool:
			val, err := flags.GetBool(tag)
			if err != nil {
				return err
			}
			ref.Field(i).SetBool(val)
		case reflect.Int:
			val, err := flags.GetInt(tag)
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
