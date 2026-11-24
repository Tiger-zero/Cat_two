package api

import (
	"strings"
	"testing"
)

func TestValidateAndGenerateUIDValidatesUserProvidedResourceIDs(t *testing.T) {
	tests := []struct {
		name      string
		provided  string
		wantError bool
	}{
		{name: "lowercase", provided: "memo-1"},
		{name: "UUID", provided: "21ec98aa-9a8f-458c-a2a3-c7dc69b6f591"},
		{name: "maximum length", provided: "a" + strings.Repeat("b", 35)},
		{name: "digit first", provided: "1-memo"},
		{name: "uppercase", provided: "Memo"},
		{name: "too long", provided: "a" + strings.Repeat("b", 36), wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			uid, err := ValidateAndGenerateUID(test.provided)
			if test.wantError {
				if err == nil {
					t.Fatalf("ValidateAndGenerateUID(%q) succeeded, want error", test.provided)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateAndGenerateUID(%q) returned error: %v", test.provided, err)
			}
			if uid != test.provided {
				t.Fatalf("ValidateAndGenerateUID(%q) = %q", test.provided, uid)
			}
		})
	}
}

func TestReadResourceNamesValidateMemoAndAttachmentUIDs(t *testing.T) {
	for _, resource := range []struct {
		prefix  string
		extract func(string) (string, error)
	}{
		{MemoNamePrefix, ExtractMemoUIDFromName},
		{AttachmentNamePrefix, ExtractAttachmentUIDFromName},
	} {
		t.Run(strings.TrimSuffix(resource.prefix, "/"), func(t *testing.T) {
			for _, uid := range []string{"123", "AbC-23", "21ec98aa-9a8f-458c-a2a3-c7dc69b6f591"} {
				name := resource.prefix + uid
				got, err := resource.extract(name)
				if err != nil || got != uid {
					t.Errorf("extract(%q) = (%q, %v), want %q", name, got, err, uid)
				}
			}
			for _, uid := range []string{"", "invalid_uid", " 123", "nested/uid", strings.Repeat("x", 37)} {
				name := resource.prefix + uid
				if _, err := resource.extract(name); err == nil {
					t.Errorf("extract(%q) succeeded, want error", name)
				}
			}
		})
	}
}
