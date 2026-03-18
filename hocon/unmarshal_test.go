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
