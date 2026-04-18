package module

import (
	"context"
	"testing"
)

func TestNvidiaModule_Name(t *testing.T) {
	if n := NewNvidiaModule().Name(); n != "nvidia" {
		t.Errorf("Name() = %q, want nvidia", n)
	}
}

func TestNvidiaModule_Check(t *testing.T) {
	rc := newDryRunRC(t)
	result, err := NewNvidiaModule().Check(context.Background(), rc)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result == nil {
		t.Fatal("Check returned nil result")
	}
}

func TestNvidiaModule_ApplyDryRun(t *testing.T) {
	rc := newDryRunRC(t)
	result, err := NewNvidiaModule().Apply(context.Background(), rc)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result == nil {
		t.Fatal("Apply returned nil result")
	}
}

func TestContainsBytes(t *testing.T) {
	cases := []struct {
		data string
		s    string
		want bool
	}{
		{`{"runtimes":{"nvidia":{}}}`, "nvidia", true},
		{`{"storage-driver":"overlay2"}`, "nvidia", false},
		{"", "nvidia", false},
		{"x", "xx", false},
	}
	for _, c := range cases {
		if got := containsBytes([]byte(c.data), c.s); got != c.want {
			t.Errorf("containsBytes(%q, %q) = %v, want %v", c.data, c.s, got, c.want)
		}
	}
}
