package schema

import (
	"strings"
	"testing"
)

func TestParseSchema(t *testing.T) {
	t.Run("valid_schema", func(t *testing.T) {
		raw := `{"required":["title"],"properties":{"title":{"type":"string"},"count":{"type":"integer"}}}`
		schema, err := parseSchema(raw)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if schema == nil {
			t.Fatal("schema should not be nil")
			return
		}
		if len(schema.Required) != 1 || schema.Required[0] != "title" {
			t.Errorf("expected required=[title], got %v", schema.Required)
		}
		if schema.Raw != raw {
			t.Error("schema.Raw field not preserved")
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		_, err := parseSchema("{not valid json")
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
		if !strings.Contains(err.Error(), "invalid JSON") {
			t.Errorf("expected 'invalid JSON' in error, got: %v", err)
		}
	})

	t.Run("invalid_pattern", func(t *testing.T) {
		_, err := parseSchema(`{"properties":{"tag":{"pattern":"[invalid"}}}`)
		if err == nil {
			t.Fatal("expected error for invalid regex pattern")
		}
		if !strings.Contains(err.Error(), "invalid pattern") {
			t.Errorf("expected 'invalid pattern' in error, got: %v", err)
		}
	})

	t.Run("invalid_type", func(t *testing.T) {
		_, err := parseSchema(`{"properties":{"field":{"type":"array"}}}`)
		if err == nil {
			t.Fatal("expected error for unsupported type")
		}
		if !strings.Contains(err.Error(), "unsupported type") {
			t.Errorf("expected 'unsupported type' in error, got: %v", err)
		}
	})

	t.Run("negative_minItems", func(t *testing.T) {
		_, err := parseSchema(`{"properties":{"tags":{"minItems":-1}}}`)
		if err == nil {
			t.Fatal("expected error for negative minItems")
		}
		if !strings.Contains(err.Error(), "minItems") {
			t.Errorf("expected 'minItems' in error, got: %v", err)
		}
	})

	t.Run("negative_maxItems", func(t *testing.T) {
		_, err := parseSchema(`{"properties":{"tags":{"maxItems":-2}}}`)
		if err == nil {
			t.Fatal("expected error for negative maxItems")
		}
		if !strings.Contains(err.Error(), "maxItems") {
			t.Errorf("expected 'maxItems' in error, got: %v", err)
		}
	})

	t.Run("valid_all_types", func(t *testing.T) {
		for _, typ := range []string{"string", "number", "integer", "boolean"} {
			_, err := parseSchema(`{"properties":{"f":{"type":"` + typ + `"}}}`)
			if err != nil {
				t.Errorf("type %q should be valid, got error: %v", typ, err)
			}
		}
	})

	t.Run("valid_pattern", func(t *testing.T) {
		_, err := parseSchema(`{"properties":{"email":{"pattern":"^[a-z]+@[a-z]+\\.[a-z]+$"}}}`)
		if err != nil {
			t.Fatalf("valid pattern should not error: %v", err)
		}
	})

	t.Run("enum_property", func(t *testing.T) {
		schema, err := parseSchema(`{"properties":{"status":{"enum":["draft","published"]}}}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		prop := schema.Properties["status"]
		if len(prop.Enum) != 2 {
			t.Errorf("expected 2 enum values, got %d", len(prop.Enum))
		}
	})
}
func TestValidateMeta(t *testing.T) {
	t.Run("required_field_missing", func(t *testing.T) {
		schema := &MetaSchema{Required: []string{"title"}}
		err := validateMeta(schema, map[string][]string{})
		if err == nil {
			t.Fatal("expected error for missing required field")
		}
		if !strings.Contains(err.Error(), "missing required field") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("required_field_present", func(t *testing.T) {
		schema := &MetaSchema{Required: []string{"title"}}
		err := validateMeta(schema, map[string][]string{"title": {"Hello"}})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("required_field_empty_values", func(t *testing.T) {
		schema := &MetaSchema{Required: []string{"title"}}
		err := validateMeta(schema, map[string][]string{"title": {}})
		if err == nil {
			t.Fatal("expected error for required field with empty values")
		}
	})

	t.Run("type_validation_number_valid", func(t *testing.T) {
		schema := &MetaSchema{
			Properties: map[string]PropertySchema{
				"count": {Type: "number"},
			},
		}
		err := validateMeta(schema, map[string][]string{"count": {"3.14"}})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("type_validation_number_invalid", func(t *testing.T) {
		schema := &MetaSchema{
			Properties: map[string]PropertySchema{
				"count": {Type: "number"},
			},
		}
		err := validateMeta(schema, map[string][]string{"count": {"abc"}})
		if err == nil {
			t.Fatal("expected error for invalid number")
		}
	})

	t.Run("enum_validation_valid", func(t *testing.T) {
		schema := &MetaSchema{
			Properties: map[string]PropertySchema{
				"status": {Enum: []string{"draft", "published"}},
			},
		}
		err := validateMeta(schema, map[string][]string{"status": {"draft"}})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("enum_validation_invalid", func(t *testing.T) {
		schema := &MetaSchema{
			Properties: map[string]PropertySchema{
				"status": {Enum: []string{"draft", "published"}},
			},
		}
		err := validateMeta(schema, map[string][]string{"status": {"archived"}})
		if err == nil {
			t.Fatal("expected error for invalid enum value")
		}
		if !strings.Contains(err.Error(), "not in allowed values") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("pattern_validation_valid", func(t *testing.T) {
		schema := &MetaSchema{
			Properties: map[string]PropertySchema{
				"code": {Pattern: "^[A-Z]{3}$"},
			},
		}
		err := validateMeta(schema, map[string][]string{"code": {"ABC"}})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("pattern_validation_invalid", func(t *testing.T) {
		schema := &MetaSchema{
			Properties: map[string]PropertySchema{
				"code": {Pattern: "^[A-Z]{3}$"},
			},
		}
		err := validateMeta(schema, map[string][]string{"code": {"abc"}})
		if err == nil {
			t.Fatal("expected error for pattern mismatch")
		}
		if !strings.Contains(err.Error(), "does not match pattern") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("minItems_violation", func(t *testing.T) {
		schema := &MetaSchema{
			Properties: map[string]PropertySchema{
				"tags": {MinItems: 2},
			},
		}
		err := validateMeta(schema, map[string][]string{"tags": {"one"}})
		if err == nil {
			t.Fatal("expected error for minItems violation")
		}
		if !strings.Contains(err.Error(), "minimum") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("maxItems_violation", func(t *testing.T) {
		schema := &MetaSchema{
			Properties: map[string]PropertySchema{
				"tags": {MaxItems: 1},
			},
		}
		err := validateMeta(schema, map[string][]string{"tags": {"one", "two"}})
		if err == nil {
			t.Fatal("expected error for maxItems violation")
		}
		if !strings.Contains(err.Error(), "maximum") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing_property_skipped", func(t *testing.T) {
		schema := &MetaSchema{
			Properties: map[string]PropertySchema{
				"optional": {Type: "integer"},
			},
		}
		err := validateMeta(schema, map[string][]string{})
		if err != nil {
			t.Errorf("absent non-required field should be skipped: %v", err)
		}
	})

	t.Run("no_errors_returns_nil", func(t *testing.T) {
		schema := &MetaSchema{}
		err := validateMeta(schema, map[string][]string{"anything": {"value"}})
		if err != nil {
			t.Errorf("expected nil error, got: %v", err)
		}
	})
}
func TestValidateType(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		typ     string
		wantErr bool
	}{
		{"string_always_valid", "f", "hello", "string", false},
		{"number_valid_int", "f", "42", "number", false},
		{"number_valid_float", "f", "3.14", "number", false},
		{"number_invalid", "f", "abc", "number", true},
		{"integer_valid", "f", "42", "integer", false},
		{"integer_invalid_float", "f", "3.14", "integer", true},
		{"integer_invalid_string", "f", "abc", "integer", true},
		{"boolean_true", "f", "true", "boolean", false},
		{"boolean_false", "f", "false", "boolean", false},
		{"boolean_invalid", "f", "yes", "boolean", true},
		{"boolean_invalid_1", "f", "1", "boolean", true},
		{"unknown_type_valid", "f", "anything", "unknown", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateType(tt.key, tt.value, tt.typ)
			if tt.wantErr && result == "" {
				t.Error("expected error string, got empty")
			}
			if !tt.wantErr && result != "" {
				t.Errorf("expected no error, got: %s", result)
			}
		})
	}
}
func TestParseSchema_ValidEmpty(t *testing.T) {
	schema, err := parseSchema(`{}`)
	if err != nil {
		t.Fatalf("expected no error for empty schema, got: %v", err)
	}
	if schema == nil {
		t.Fatal("expected non-nil schema")
	}
}
func TestParseSchema_InvalidMinItems(t *testing.T) {
	// minItems with negative value - note: JSON will parse negative into int
	// The check is minItems < 0
	_, err := parseSchema(`{"properties":{"tags":{"minItems":-1}}}`)
	if err == nil {
		t.Error("expected error for negative minItems")
	}
}
func TestParseSchema_InvalidMaxItems(t *testing.T) {
	_, err := parseSchema(`{"properties":{"tags":{"maxItems":-1}}}`)
	if err == nil {
		t.Error("expected error for negative maxItems")
	}
}
func TestValidateType_StringAlwaysValid(t *testing.T) {
	msg := validateType("field", "anything", "string")
	if msg != "" {
		t.Errorf("string type should always be valid, got: %s", msg)
	}
}
func TestValidateType_NumberValid(t *testing.T) {
	msg := validateType("field", "3.14", "number")
	if msg != "" {
		t.Errorf("3.14 should be valid number, got: %s", msg)
	}
}
func TestValidateType_NumberInvalid(t *testing.T) {
	msg := validateType("field", "abc", "number")
	if msg == "" {
		t.Error("abc should not be valid number")
	}
}
func TestValidateType_IntegerValid(t *testing.T) {
	msg := validateType("field", "42", "integer")
	if msg != "" {
		t.Errorf("42 should be valid integer, got: %s", msg)
	}
}
func TestValidateType_IntegerInvalid(t *testing.T) {
	msg := validateType("field", "3.14", "integer")
	if msg == "" {
		t.Error("3.14 should not be valid integer")
	}
}
func TestValidateType_BooleanValid(t *testing.T) {
	for _, val := range []string{"true", "false"} {
		msg := validateType("field", val, "boolean")
		if msg != "" {
			t.Errorf("%s should be valid boolean, got: %s", val, msg)
		}
	}
}
func TestValidateType_BooleanInvalid(t *testing.T) {
	msg := validateType("field", "yes", "boolean")
	if msg == "" {
		t.Error("yes should not be valid boolean")
	}
}
func TestSchemaManagerSetBinlog(t *testing.T) {
	sm, cleanup := newTestSchemaManager(t)
	defer cleanup()

	sm.SetBinlog(nil)
	if sm.binlog != nil {
		t.Error("expected nil binlog")
	}
}
func TestValidateMeta_CombinedRules(t *testing.T) {
	schema := &MetaSchema{
		Required: []string{"status", "priority"},
		Properties: map[string]PropertySchema{
			"status":   {Enum: []string{"draft", "published"}},
			"priority": {Type: "integer", MinItems: 1, MaxItems: 1},
			"tags":     {MinItems: 1, MaxItems: 3},
		},
	}

	// Valid
	err := validateMeta(schema, map[string][]string{
		"status":   {"draft"},
		"priority": {"5"},
		"tags":     {"go", "test"},
	})
	if err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	// Missing required
	err = validateMeta(schema, map[string][]string{
		"status": {"draft"},
	})
	if err == nil {
		t.Error("expected error for missing priority")
	}

	// Invalid enum
	err = validateMeta(schema, map[string][]string{
		"status":   {"deleted"},
		"priority": {"5"},
	})
	if err == nil {
		t.Error("expected error for invalid enum value")
	}

	// Too many tags
	err = validateMeta(schema, map[string][]string{
		"status":   {"draft"},
		"priority": {"5"},
		"tags":     {"a", "b", "c", "d"},
	})
	if err == nil {
		t.Error("expected error for too many tags")
	}
}
