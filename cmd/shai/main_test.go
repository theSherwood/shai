package main

import (
	"reflect"
	"testing"
)

func TestParseTemplateVars(t *testing.T) {
	vars, err := parseTemplateVars([]string{"FOO=bar", "HELLO=world"})
	if err != nil {
		t.Fatalf("parseTemplateVars returned error: %v", err)
	}
	expected := map[string]string{"FOO": "bar", "HELLO": "world"}
	if !reflect.DeepEqual(vars, expected) {
		t.Fatalf("expected %v, got %v", expected, vars)
	}
}

func TestParseTemplateVarsErrors(t *testing.T) {
	if _, err := parseTemplateVars([]string{"broken"}); err == nil {
		t.Fatal("expected error for missing equals sign")
	}
	if _, err := parseTemplateVars([]string{" =value"}); err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestNormalizeLegacyArgs(t *testing.T) {
	input := []string{"shai", "-rw", "./src", "-rs=myset", "--verbose"}
	got := normalizeLegacyArgs(input)
	expected := []string{"shai", "--read-write", "./src", "--resource-set=myset", "--verbose"}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("expected %v, got %v", expected, got)
	}
}
