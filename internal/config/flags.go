// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var durationType = reflect.TypeFor[time.Duration]()

// intFlag is an int flag that reads its value in 64 bits and keeps it within
// the range of int. pflag's own int flag keeps the low 32 bits of a value on a
// platform where int has 32 bits, so 4294967296 would become 0 there. Kept
// within range, a value beyond int is refused by the range check of its field,
// as on a 64-bit platform.
type intFlag int

func (f *intFlag) Set(s string) error {
	v, err := strconv.ParseInt(s, 0, 64)
	if err != nil {
		return err
	}
	*f = intFlag(max(min(v, math.MaxInt), math.MinInt))
	return nil
}

func (f *intFlag) String() string { return strconv.Itoa(int(*f)) }

func (f *intFlag) Type() string { return "int" }

// RegisterFlags binds cobra flags to tagged fields on cfg (pointer to struct).
// Numeric and bool fields use reflect.Value.Addr().Interface().(*T) with the matching
// pflag *Var function — no unsafe.Pointer casts. Supported kinds: string, bool, []string,
// uint8–uint64, int, int8–int64, uint and time.Duration.
func RegisterFlags(cmd *cobra.Command, cfg any) {
	RegisterFlagSet(cmd.Flags(), cfg)
}

// RegisterFlagSet is [RegisterFlags] on a flag set: the root's persistent flags.
func RegisterFlagSet(fs *pflag.FlagSet, cfg any) {
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
			p := fv.Addr().Interface().(*uint8)
			fs.Uint8Var(p, flagName, *p, desc)
		case reflect.Uint16:
			p := fv.Addr().Interface().(*uint16)
			fs.Uint16Var(p, flagName, *p, desc)
		case reflect.Uint32:
			p := fv.Addr().Interface().(*uint32)
			fs.Uint32Var(p, flagName, *p, desc)
		case reflect.Uint64:
			fs.Uint64Var(fv.Addr().Interface().(*uint64), flagName, fv.Uint(), desc)
		case reflect.Uint:
			fs.UintVar(fv.Addr().Interface().(*uint), flagName, uint(fv.Uint()), desc)
		case reflect.Int:
			fs.Var((*intFlag)(fv.Addr().Interface().(*int)), flagName, desc)
		case reflect.Int8:
			p := fv.Addr().Interface().(*int8)
			fs.Int8Var(p, flagName, *p, desc)
		case reflect.Int16:
			p := fv.Addr().Interface().(*int16)
			fs.Int16Var(p, flagName, *p, desc)
		case reflect.Int32:
			p := fv.Addr().Interface().(*int32)
			fs.Int32Var(p, flagName, *p, desc)
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
