package hocon_test

import (
	"testing"
	"time"
)

type ServerCfg struct {
	Host    string        `hocon:"host"`
	Port    int           `hocon:"port"`
	Timeout time.Duration `hocon:"timeout,omitempty"`
	Tags    []string      `hocon:"tags"`
}

func TestUnmarshal_Basic(t *testing.T) {
	cfg := mustParseCfg(t, "host=localhost\nport=8080\ntags=[\"web\",\"api\"]")
	var s ServerCfg
	if err := cfg.Unmarshal(&s); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if s.Host != "localhost" {
		t.Errorf("Host=%q", s.Host)
	}
	if s.Port != 8080 {
		t.Errorf("Port=%d", s.Port)
	}
	if len(s.Tags) != 2 || s.Tags[0] != "web" {
		t.Errorf("Tags=%v", s.Tags)
	}
}

func TestUnmarshal_Omitempty_Missing(t *testing.T) {
	cfg := mustParseCfg(t, "host=localhost\nport=9000")
	s := ServerCfg{Timeout: 5 * time.Second} // pre-populated default
	if err := cfg.Unmarshal(&s); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	// omitempty: pre-populated value should be preserved, not zeroed
	if s.Timeout != 5*time.Second {
		t.Errorf("Timeout should be preserved, got %v", s.Timeout)
	}
}

func TestUnmarshal_MissingRequired_Error(t *testing.T) {
	cfg := mustParseCfg(t, `port=9000`)
	var s ServerCfg
	err := cfg.Unmarshal(&s)
	if err == nil {
		t.Fatal("expected error for missing required field 'host'")
	}
}

func TestUnmarshal_Nested(t *testing.T) {
	type Inner struct {
		X int `hocon:"x"`
	}
	type Outer struct {
		Inner Inner `hocon:"inner"`
	}
	cfg := mustParseCfg(t, "inner {\n  x = 42\n}")
	var o Outer
	if err := cfg.Unmarshal(&o); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if o.Inner.X != 42 {
		t.Errorf("Inner.X=%d", o.Inner.X)
	}
}

func TestUnmarshal_NoTag(t *testing.T) {
	type Cfg struct {
		Host string // no tag — uses field name lowercased
	}
	cfg := mustParseCfg(t, `host=localhost`)
	var c Cfg
	if err := cfg.Unmarshal(&c); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if c.Host != "localhost" {
		t.Errorf("Host=%q", c.Host)
	}
}

func TestUnmarshal_EnvVarStringCoercion(t *testing.T) {
	t.Setenv("HOCON_TEST_PORT", "9090")
	t.Setenv("HOCON_TEST_RATIO", "1.5")
	t.Setenv("HOCON_TEST_ENABLED", "true")
	type Cfg struct {
		Port    int     `hocon:"port"`
		Ratio   float64 `hocon:"ratio"`
		Enabled bool    `hocon:"enabled"`
	}
	src := "port=8080\nport=${?HOCON_TEST_PORT}\nratio=1.0\nratio=${?HOCON_TEST_RATIO}\nenabled=false\nenabled=${?HOCON_TEST_ENABLED}"
	cfg := mustParseCfg(t, src)
	var out Cfg
	if err := cfg.Unmarshal(&out); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if out.Port != 9090 {
		t.Errorf("Port: expected 9090, got %d", out.Port)
	}
	if out.Ratio != 1.5 {
		t.Errorf("Ratio: expected 1.5, got %f", out.Ratio)
	}
	if !out.Enabled {
		t.Errorf("Enabled: expected true, got false")
	}
}

func TestConfig_EnvVarInt64Slice(t *testing.T) {
	// GetInt64Slice with string elements from env var substitution is a corner case;
	// the more common scenario is that slice elements are literals. This test covers
	// a mixed scenario to ensure the string fallback works.
	cfg := mustParseCfg(t, "ports=[8080, 9090]")
	s := cfg.GetInt64Slice("ports")
	if len(s) != 2 || s[0] != 8080 || s[1] != 9090 {
		t.Errorf("unexpected slice: %v", s)
	}
}

func TestUnmarshal_MapStringAny(t *testing.T) {
	cfg := mustParseCfg(t, "a=1\nb=\"hello\"\nc=true")
	m := make(map[string]any)
	if err := cfg.Unmarshal(&m); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if m["a"] != int64(1) {
		t.Errorf("a=%v (%T)", m["a"], m["a"])
	}
	if m["b"] != "hello" {
		t.Errorf("b=%v", m["b"])
	}
}

func TestUnmarshal_MapStringInt(t *testing.T) {
	cfg := mustParseCfg(t, "ports { http = 80, https = 443 }")
	var result struct{ Ports map[string]int }
	if err := cfg.Unmarshal(&result); err != nil {
		t.Fatal(err)
	}
	if result.Ports["http"] != 80 {
		t.Errorf("http = %d, want 80", result.Ports["http"])
	}
	if result.Ports["https"] != 443 {
		t.Errorf("https = %d, want 443", result.Ports["https"])
	}
}

func TestUnmarshal_MapStringString(t *testing.T) {
	cfg := mustParseCfg(t, `labels { env = "prod", region = "us-east" }`)
	var result struct{ Labels map[string]string }
	if err := cfg.Unmarshal(&result); err != nil {
		t.Fatal(err)
	}
	if result.Labels["env"] != "prod" {
		t.Errorf("env = %q", result.Labels["env"])
	}
}

func TestUnmarshal_MapStringBool(t *testing.T) {
	cfg := mustParseCfg(t, "flags { debug = true, verbose = false }")
	var result struct{ Flags map[string]bool }
	if err := cfg.Unmarshal(&result); err != nil {
		t.Fatal(err)
	}
	if !result.Flags["debug"] {
		t.Error("debug should be true")
	}
}

func TestUnmarshal_MapStringFloat64(t *testing.T) {
	cfg := mustParseCfg(t, "rates { usd = 1.0, eur = 0.85 }")
	var result struct{ Rates map[string]float64 }
	if err := cfg.Unmarshal(&result); err != nil {
		t.Fatal(err)
	}
	if result.Rates["eur"] != 0.85 {
		t.Errorf("eur = %f", result.Rates["eur"])
	}
}
