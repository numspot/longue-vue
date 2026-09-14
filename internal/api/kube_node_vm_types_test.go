package api

import (
	"encoding/json"
	"testing"
)

func TestParseInstanceType(t *testing.T) {
	cases := []struct {
		in        string
		vcpu, mem int
		ok        bool
	}{
		{"tinav7.c8r32p1", 8, 32, true},
		{"tinav7.c4r16p2", 4, 16, true},
		{"tinav5.c2r4p3", 2, 4, true},
		{"inference7-h100.medium", 0, 0, false},
		{"", 0, 0, false},
		{"tinav7.c8r32", 0, 0, false},
	}
	for _, c := range cases {
		v, m, ok := ParseInstanceType(c.in)
		if v != c.vcpu || m != c.mem || ok != c.ok {
			t.Errorf("ParseInstanceType(%q) = (%d,%d,%v); want (%d,%d,%v)", c.in, v, m, ok, c.vcpu, c.mem, c.ok)
		}
	}
}

// The three-field legacy payload must still decode into NodeImage with
// the new fields empty (backward compatibility, ADR-0045 §2).
func TestNodeImage_LegacyPayloadDecodes(t *testing.T) {
	var img NodeImage
	if err := json.Unmarshal([]byte(`{"provider_vm_id":"i-1","image_id":"ami-1","image_name":"img"}`), &img); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if img.ProviderVMID != "i-1" || img.ClusterHint != "" || img.ProviderCreationDate != nil {
		t.Fatalf("unexpected decode: %+v", img)
	}
}
