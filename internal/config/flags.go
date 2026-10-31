// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"reflect"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var durationType = reflect.TypeOf(time.Duration(0))

// RegisterFlags binds cobra flags to tagged fields on cfg (pointer to struct).
// Numeric and bool fields use reflect.Value.Addr().Interface().(*T) with the matching
// pflag *Var function — no unsafe.Pointer casts. Supported kinds: string, bool, []string,
// uint8–uint64, int, int8–int64, uint and time.Duration.
func RegisterFlags(cmd *cobra.Command, cfg interface{}) {
	RegisterFlagSet(cmd.Flags(), cfg)
}

// RegisterFlagSet is [RegisterFlags] on a flag set: the root's persistent flags.
func RegisterFlagSet(fs *pflag.FlagSet, cfg interface{}) {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		flagName := field.Tag.Get("flag")
		desc := field.Tag.Get("desc")
		envVar := field.Tag.Get("env")

		if field.Anonymous {
			RegisterFlagSet(fs, v.Field(i).Addr().Interface())
			continue
		}

		if flagName == "" {
			continue
		}

		if envVar != "" {
			desc = fmt.Sprintf("%s [env: %s]", desc, envVar)
		}

		fv := v.Field(i)
		if field.Type == durationType {
			fs.DurationVar(fv.Addr().Interface().(*time.Duration), flagName, time.Duration(fv.Int()), desc)
			continue
		}
		switch field.Type.Kind() {
		case reflect.String:
			fs.StringVar(fv.Addr().Interface().(*string), flagName, fv.String(), desc)
		case reflect.Uint8:
			fs.Uint8Var(fv.Addr().Interface().(*uint8), flagName, uint8(fv.Uint()), desc)
		case reflect.Uint16:
			fs.Uint16Var(fv.Addr().Interface().(*uint16), flagName, uint16(fv.Uint()), desc)
		case reflect.Uint32:
			fs.Uint32Var(fv.Addr().Interface().(*uint32), flagName, uint32(fv.Uint()), desc)
		case reflect.Uint64:
			fs.Uint64Var(fv.Addr().Interface().(*uint64), flagName, fv.Uint(), desc)
		case reflect.Uint:
			fs.UintVar(fv.Addr().Interface().(*uint), flagName, uint(fv.Uint()), desc)
		case reflect.Int:
			fs.IntVar(fv.Addr().Interface().(*int), flagName, int(fv.Int()), desc)
		case reflect.Int8:
			fs.Int8Var(fv.Addr().Interface().(*int8), flagName, int8(fv.Int()), desc)
		case reflect.Int16:
			fs.Int16Var(fv.Addr().Interface().(*int16), flagName, int16(fv.Int()), desc)
		case reflect.Int32:
			fs.Int32Var(fv.Addr().Interface().(*int32), flagName, int32(fv.Int()), desc)
		case reflect.Int64:
			fs.Int64Var(fv.Addr().Interface().(*int64), flagName, fv.Int(), desc)
		case reflect.Bool:
			fs.BoolVar(fv.Addr().Interface().(*bool), flagName, fv.Bool(), desc)
		case reflect.Slice:
			if field.Type.Elem().Kind() != reflect.String {
				panic(fmt.Sprintf("config.RegisterFlags: field %s: flag %q only supports []string slices", field.Name, flagName))
			}
			var cur []string
			if !fv.IsNil() {
				cur = fv.Interface().([]string)
			}
			fs.StringSliceVar(fv.Addr().Interface().(*[]string), flagName, cur, desc)
		default:
			panic(fmt.Sprintf("config.RegisterFlags: field %s (%s) has flag %q but unsupported reflect.Kind %s — add a case or remove the flag tag",
				field.Name, field.Type.String(), flagName, field.Type.Kind()))
		}
	}
}
