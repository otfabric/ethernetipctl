package config

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

type sample struct {
	Inner
	Name    string        `env:"T_NAME" flag:"name" desc:"a name"`
	Port    uint16        `env:"T_PORT" flag:"port" desc:"a port"`
	Small   uint8         `env:"T_SMALL" flag:"small" desc:"small"`
	Wide    uint32        `env:"T_WIDE" flag:"wide" desc:"wide"`
	Huge    uint64        `env:"T_HUGE" flag:"huge" desc:"huge"`
	Plain   uint          `env:"T_PLAIN" flag:"plain" desc:"plain"`
	Signed  int           `env:"T_SIGNED" flag:"signed" desc:"signed"`
	S8      int8          `env:"T_S8" flag:"s8" desc:"s8"`
	S16     int16         `env:"T_S16" flag:"s16" desc:"s16"`
	S32     int32         `env:"T_S32" flag:"s32" desc:"s32"`
	S64     int64         `env:"T_S64" flag:"s64" desc:"s64"`
	On      bool          `env:"T_ON" flag:"on" desc:"on"`
	List    []string      `env:"T_LIST" flag:"list" desc:"a list"`
	Wait    time.Duration `env:"T_WAIT" flag:"wait" desc:"a duration"`
	NoEnv   string        `flag:"no-env" desc:"without env"`
	Hidden  string        // neither flag nor env
	private string        `env:"T_PRIVATE"`
}

type Inner struct {
	Deep string `env:"T_DEEP" flag:"deep" desc:"embedded"`
}

func TestEnvironmentAndFlags(t *testing.T) {
	for k, v := range map[string]string{
		"T_NAME": "rtu", "T_PORT": "2404", "T_SMALL": "7", "T_WIDE": "70000", "T_HUGE": "5000000000", "T_PLAIN": "9",
		"T_SIGNED": "-3", "T_S8": "-8", "T_S16": "-16", "T_S32": "-32", "T_S64": "-64", "T_ON": "true",
		"T_LIST": "a, b ,c", "T_DEEP": "embedded", "T_PRIVATE": "x", "T_WAIT": "1500ms",
	} {
		t.Setenv(k, v)
	}
	var cfg sample
	if err := LoadFromEnv(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "rtu" || cfg.Port != 2404 || cfg.Small != 7 || cfg.Wide != 70000 || cfg.Huge != 5000000000 || cfg.Plain != 9 ||
		cfg.Signed != -3 || cfg.S8 != -8 || cfg.S16 != -16 || cfg.S32 != -32 || cfg.S64 != -64 || !cfg.On ||
		strings.Join(cfg.List, "|") != "a|b|c" || cfg.Deep != "embedded" || cfg.private != "" || cfg.Wait != 1500*time.Millisecond {
		t.Fatalf("loaded %+v", cfg)
	}

	// Flags start from the environment, say which variable sets them, and win over it.
	cmd := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
	RegisterFlags(cmd, &cfg)
	if f := cmd.Flags().Lookup("port"); f == nil || f.DefValue != "2404" || f.Usage != "a port [env: T_PORT]" {
		t.Errorf("flag port: %+v", f)
	}
	if f := cmd.Flags().Lookup("no-env"); f == nil || f.Usage != "without env" {
		t.Errorf("flag no-env: %+v", f)
	}
	if cmd.Flags().Lookup("deep") == nil || cmd.Flags().Lookup("hidden") != nil {
		t.Error("the flag of an embedded struct is missing, or an untagged field became a flag")
	}
	cmd.SetArgs([]string{"--port", "19998", "--list", "x,y", "--on=false", "--s8", "-1", "--small", "200", "--signed", "4", "--huge", "1",
		"--plain", "2", "--wide", "3", "--s16", "5", "--s32", "6", "--s64", "7", "--name", "n", "--deep", "d", "--wait", "2s"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 19998 || strings.Join(cfg.List, "|") != "x|y" || cfg.On || cfg.S8 != -1 || cfg.Small != 200 || cfg.Deep != "d" || cfg.Wait != 2*time.Second {
		t.Errorf("after flags: %+v", cfg)
	}
}

func TestMalformedEnvironment(t *testing.T) {
	for k, v := range map[string]string{"T_PORT": "70000", "T_SMALL": "x", "T_SIGNED": "1.5", "T_S8": "500", "T_ON": "maybe", "T_WAIT": "soon"} {
		t.Setenv(k, v)
	}
	var cfg sample
	err := LoadFromEnv(&cfg)
	if err == nil {
		t.Fatal("malformed values were accepted")
	}
	for _, name := range []string{"T_PORT", "T_SMALL", "T_SIGNED", "T_S8", "T_ON", "T_WAIT"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("the error does not name %s: %v", name, err)
		}
	}
	if LoadFromEnv(cfg) != nil || LoadFromEnv(nil) != nil {
		t.Error("a value that is no pointer to a struct is ignored, not an error")
	}
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "invalid environment") {
			t.Errorf("MustLoadFromEnv: %v", r)
		}
	}()
	MustLoadFromEnv(&cfg)
}

func TestUnsupportedFlagKindsFailAtStartup(t *testing.T) {
	for name, cfg := range map[string]any{
		"float": &struct {
			F float64 `flag:"f"`
		}{},
		"int slice": &struct {
			L []int `flag:"l"`
		}{},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("an unsupported field kind was registered as a flag")
				}
			}()
			RegisterFlags(&cobra.Command{}, cfg)
		})
	}
	// An env tag on an unsupported kind is skipped.
	t.Setenv("T_F", "1.5")
	t.Setenv("T_L", "1,2")
	var cfg struct {
		F float64 `env:"T_F"`
		L []int   `env:"T_L"`
	}
	if err := LoadFromEnv(&cfg); err != nil || cfg.F != 0 || cfg.L != nil {
		t.Errorf("unsupported kinds: %+v, %v", cfg, err)
	}
}

func TestParseHost(t *testing.T) {
	for in, want := range map[string]string{
		"192.168.1.10":      "192.168.1.10:44818",
		" 10.0.0.1:2222 ":   "10.0.0.1:2222",
		"plc-1.example.net": "plc-1.example.net:44818",
		"localhost:1":       "localhost:1",
		"my_host":           "my_host:44818",
	} {
		got, err := ParseHost(in, 44818)
		if err != nil || got.String() != want {
			t.Errorf("ParseHost(%q) = %v, %v; want %s", in, got, err, want)
		}
	}
	if got, _ := ParseHost("10.0.0.1", 0); got.Port != DefaultPort {
		t.Errorf("default port: %v", got)
	}
	for in, msg := range map[string]string{
		"":                      "no target",
		"10.0.0.1:0":            "port must be",
		"10.0.0.1:x":            "port must be",
		"::1":                   "IPv6",
		"[::1]:44818":           "IPv6",
		"192.168.1.0/24":        "does not scan",
		"192.168.1.1-20":        "does not scan",
		"10.0.0.*":              "does not scan",
		"a,b":                   "does not scan",
		"300.1.1.1":             "not a valid IPv4",
		"bad..name":             "neither",
		":44818":                "missing host",
		strings.Repeat("a", 64): "neither",
	} {
		if _, err := ParseHost(in, 44818); err == nil || !strings.Contains(err.Error(), msg) {
			t.Errorf("ParseHost(%q): %v, want %q", in, err, msg)
		}
	}
}

func TestGlobalDefaults(t *testing.T) {
	g := NewGlobal()
	if g.OutputFormat != "text" || g.Timeout != DefaultTimeout || g.Port != DefaultPort || g.Debug {
		t.Errorf("defaults: %+v", g)
	}
}
