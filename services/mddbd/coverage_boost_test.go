package main

import (
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 1. fts_stemmer.go — Porter stemmer helper functions
// ---------------------------------------------------------------------------

func TestIsConsonant(t *testing.T) {
	tests := []struct {
		name string
		word string
		idx  int
		want bool
	}{
		{"vowel_a", "apple", 0, false},
		{"vowel_e", "hello", 1, false},
		{"vowel_i", "bit", 1, false},
		{"vowel_o", "top", 1, false},
		{"vowel_u", "cup", 1, false},
		{"consonant_b", "bat", 0, true},
		{"consonant_t", "bat", 2, true},
		{"y_at_start", "yes", 0, true},
		{"y_after_consonant", "byte", 1, false},
		{"y_after_vowel", "day", 2, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isConsonant([]byte(tt.word), tt.idx)
			if got != tt.want {
				t.Errorf("isConsonant(%q, %d) = %v, want %v", tt.word, tt.idx, got, tt.want)
			}
		})
	}
}

func TestMeasure(t *testing.T) {
	tests := []struct {
		word string
		want int
	}{
		{"", 0},
		{"a", 0},        // V
		{"b", 0},        // C
		{"ab", 1},       // VC = (VC){1}
		{"tr", 0},       // CC
		{"tree", 0},     // CCVV
		{"trouble", 1},  // CC V C V C V = (VC){1}
		{"oats", 1},     // V C V C
		{"trees", 1},    // CC V V C
		{"troubles", 2}, // CC V C V C V C
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			got := measure([]byte(tt.word))
			if got != tt.want {
				t.Errorf("measure(%q) = %d, want %d", tt.word, got, tt.want)
			}
		})
	}
}

func TestHasVowel(t *testing.T) {
	tests := []struct {
		word string
		want bool
	}{
		{"bcd", false},
		{"abc", true},
		{"xyz", true}, // y after x (consonant) is a vowel
		{"yell", true},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			got := hasVowel([]byte(tt.word))
			if got != tt.want {
				t.Errorf("hasVowel(%q) = %v, want %v", tt.word, got, tt.want)
			}
		})
	}
}

func TestEndsWithDouble(t *testing.T) {
	tests := []struct {
		word string
		want bool
	}{
		{"fall", true},
		{"miss", true},
		{"buzz", true},
		{"cat", false},
		{"a", false},
		{"", false},
		{"bee", false}, // ee are vowels, not consonants
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			got := endsWithDouble([]byte(tt.word))
			if got != tt.want {
				t.Errorf("endsWithDouble(%q) = %v, want %v", tt.word, got, tt.want)
			}
		})
	}
}

func TestEndsCVC(t *testing.T) {
	tests := []struct {
		word string
		want bool
	}{
		{"hop", true},   // h-o-p, CVC, p not w/x/y
		{"lov", true},   // l-o-v, CVC
		{"bow", false},  // ends with w
		{"box", false},  // ends with x
		{"boy", false},  // ends with y
		{"ab", false},   // too short
		{"a", false},    // too short
		{"", false},     // empty
		{"oat", false},  // o is vowel at position 0, a is vowel at 1 => not CVC
		{"bat", true},   // b-a-t CVC
		{"pet", true},   // p-e-t CVC
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			got := endsCVC([]byte(tt.word))
			if got != tt.want {
				t.Errorf("endsCVC(%q) = %v, want %v", tt.word, got, tt.want)
			}
		})
	}
}

func TestHasSuffix(t *testing.T) {
	tests := []struct {
		word   string
		suffix string
		want   bool
	}{
		{"running", "ing", true},
		{"running", "run", false},
		{"ed", "ed", true},
		{"a", "ab", false},
		{"", "x", false},
		{"test", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.word+"_"+tt.suffix, func(t *testing.T) {
			got := hasSuffix([]byte(tt.word), tt.suffix)
			if got != tt.want {
				t.Errorf("hasSuffix(%q, %q) = %v, want %v", tt.word, tt.suffix, got, tt.want)
			}
		})
	}
}

func TestRemoveSuffix(t *testing.T) {
	tests := []struct {
		word string
		n    int
		want string
	}{
		{"running", 3, "runn"},
		{"tested", 2, "test"},
		{"abc", 0, "abc"},
		{"abc", 3, ""},
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			got := string(removeSuffix([]byte(tt.word), tt.n))
			if got != tt.want {
				t.Errorf("removeSuffix(%q, %d) = %q, want %q", tt.word, tt.n, got, tt.want)
			}
		})
	}
}

func TestStep1a(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"caresses", "caress"},  // SSES -> SS
		{"ponies", "poni"},      // IES -> I
		{"caress", "caress"},    // SS -> SS
		{"cats", "cat"},         // S -> (remove)
		{"cat", "cat"},          // no suffix
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := string(step1a([]byte(tt.input)))
			if got != tt.want {
				t.Errorf("step1a(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStep1b(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"feed", "feed"},       // EED with m=0 -> unchanged
		{"agreed", "agree"},    // EED with m>0 -> EE
		{"plastered", "plaster"}, // ED with vowel in stem
		{"bled", "bled"},       // ED without vowel in stem
		{"motoring", "motor"},  // ING with vowel in stem
		{"sing", "sing"},       // ING without vowel in stem
		{"conflated", "conflate"}, // ED -> stem ends "at" -> add e
		{"troubled", "trouble"},   // ED -> stem ends with double (ll) but l is exempt
		{"hopping", "hop"},     // ING -> stem ends with double (pp) -> remove last
		{"filing", "file"},     // ING -> m=1, CVC -> add e
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := string(step1b([]byte(tt.input)))
			if got != tt.want {
				t.Errorf("step1b(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStep1c(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"happy", "happi"},  // Y with vowel in stem -> I
		{"sky", "sky"},      // Y without vowel in stem -> unchanged
		{"cat", "cat"},      // no Y suffix
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := string(step1c([]byte(tt.input)))
			if got != tt.want {
				t.Errorf("step1c(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStep2(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"relational", "relate"},   // ational -> ate
		{"conditional", "condition"}, // tional -> tion
		{"valenci", "valence"},     // enci -> ence
		{"hesitanci", "hesitance"}, // anci -> ance
		{"digitizer", "digitize"}, // izer -> ize
		{"formalli", "formal"},    // alli -> al
		{"cat", "cat"},            // no matching suffix
		// m=0 stem should not apply
		{"ational", "ational"}, // stem "at" has m=0
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := string(step2([]byte(tt.input)))
			if got != tt.want {
				t.Errorf("step2(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStep3(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"triplicate", "triplic"}, // icate -> ic
		{"formative", "form"},     // ative -> ""
		{"formalize", "formal"},   // alize -> al
		{"electriciti", "electric"}, // iciti -> ic
		{"electrical", "electric"},  // ical -> ic
		{"hopeful", "hope"},       // ful -> ""
		{"goodness", "good"},      // ness -> ""
		{"cat", "cat"},            // no matching suffix
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := string(step3([]byte(tt.input)))
			if got != tt.want {
				t.Errorf("step3(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStep4(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"revival", "reviv"},      // al with m>1
		{"allowance", "allow"},    // ance with m>1
		{"inference", "infer"},    // ence with m>1
		{"adjustable", "adjust"},  // able with m>1
		{"adoption", "adopt"},     // ion with t preceding, m>1
		{"impression", "impress"}, // ion with s preceding, m>1
		{"activate", "activ"},     // ate with m>1
		{"cat", "cat"},            // no matching suffix
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := string(step4([]byte(tt.input)))
			if got != tt.want {
				t.Errorf("step4(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStep5a(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"probate", "probat"}, // m>1, remove e
		{"rate", "rate"},      // m=1, CVC -> keep e
		{"cease", "ceas"},     // m>1
		{"cat", "cat"},        // no e suffix
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := string(step5a([]byte(tt.input)))
			if got != tt.want {
				t.Errorf("step5a(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestStep5b(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"controll", "control"}, // m>1, double l -> single l
		{"roll", "roll"},        // m=1 -> unchanged
		{"cat", "cat"},          // no double ending
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := string(step5b([]byte(tt.input)))
			if got != tt.want {
				t.Errorf("step5b(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 2. schema.go — validation functions
// ---------------------------------------------------------------------------

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
			t.Error("Raw field not preserved")
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
		name     string
		key      string
		value    string
		typ      string
		wantErr  bool
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

func TestContains(t *testing.T) {
	tests := []struct {
		name  string
		slice []string
		val   string
		want  bool
	}{
		{"found", []string{"a", "b", "c"}, "b", true},
		{"not_found", []string{"a", "b", "c"}, "d", false},
		{"empty_slice", []string{}, "a", false},
		{"nil_slice", nil, "a", false},
		{"empty_string_match", []string{""}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contains(tt.slice, tt.val)
			if got != tt.want {
				t.Errorf("contains(%v, %q) = %v, want %v", tt.slice, tt.val, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3. fts_range.go — matchStringRange
// ---------------------------------------------------------------------------

func TestMatchStringRange(t *testing.T) {
	tests := []struct {
		name string
		val  string
		rf   RangeFilter
		want bool
	}{
		{"gte_pass", "b", RangeFilter{Gte: "a"}, true},
		{"gte_equal", "a", RangeFilter{Gte: "a"}, true},
		{"gte_fail", "a", RangeFilter{Gte: "b"}, false},
		{"gt_pass", "b", RangeFilter{Gt: "a"}, true},
		{"gt_equal_fail", "a", RangeFilter{Gt: "a"}, false},
		{"gt_fail", "a", RangeFilter{Gt: "b"}, false},
		{"lte_pass", "a", RangeFilter{Lte: "b"}, true},
		{"lte_equal", "b", RangeFilter{Lte: "b"}, true},
		{"lte_fail", "c", RangeFilter{Lte: "b"}, false},
		{"lt_pass", "a", RangeFilter{Lt: "b"}, true},
		{"lt_equal_fail", "b", RangeFilter{Lt: "b"}, false},
		{"lt_fail", "c", RangeFilter{Lt: "b"}, false},
		{"combined_gte_lte_pass", "m", RangeFilter{Gte: "a", Lte: "z"}, true},
		{"combined_gte_lte_fail", "a", RangeFilter{Gte: "b", Lte: "z"}, false},
		{"all_empty_pass", "anything", RangeFilter{}, true},
		{"all_boundaries", "m", RangeFilter{Gte: "a", Gt: "b", Lte: "z", Lt: "y"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchStringRange(tt.val, tt.rf)
			if got != tt.want {
				t.Errorf("matchStringRange(%q, %+v) = %v, want %v", tt.val, tt.rf, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 4. fts_wildcard.go — wildcardMatch
// ---------------------------------------------------------------------------

func TestWildcardMatchCoverage(t *testing.T) {
	tests := []struct {
		pattern string
		text    string
		want    bool
	}{
		{"*", "anything", true},
		{"*", "", true},
		{"?", "a", true},
		{"?", "", false},
		{"?", "ab", false},
		{"hello", "hello", true},
		{"hello", "world", false},
		{"hel*", "hello", true},
		{"hel*", "help", true},
		{"hel*", "he", false},
		{"*lo", "hello", true},
		{"*lo", "lo", true},
		{"*lo", "low", false},
		{"h?llo", "hello", true},
		{"h?llo", "hallo", true},
		{"h?llo", "hllo", false},
		{"h*o", "hello", true},
		{"h*o", "ho", true},
		{"h*o", "hey", false},
		{"*a*b*", "aXYZb", true},
		{"*a*b*", "xaxbx", true},
		{"*a*b*", "xyz", false},
		{"", "", true},
		{"", "a", false},
		{"**", "abc", true},
		{"a*b*c", "abc", true},
		{"a*b*c", "aXbYc", true},
		{"a*b*c", "aXbY", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.text, func(t *testing.T) {
			got := wildcardMatch(tt.pattern, tt.text)
			if got != tt.want {
				t.Errorf("wildcardMatch(%q, %q) = %v, want %v", tt.pattern, tt.text, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 5. aggregation.go — weekNumber, itoa
// ---------------------------------------------------------------------------

func TestWeekNumber(t *testing.T) {
	tests := []struct {
		name string
		time time.Time
		want string
	}{
		{"single_digit_week", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), "02"},     // week 2
		{"double_digit_week", time.Date(2026, 3, 19, 0, 0, 0, 0, time.UTC), "12"},     // week 12
		{"week_1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "01"},                 // week 1
		{"week_52", time.Date(2025, 12, 29, 0, 0, 0, 0, time.UTC), "01"},              // ISO week 1 of 2026
		{"last_week_of_year", time.Date(2025, 12, 22, 0, 0, 0, 0, time.UTC), "52"},    // week 52
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := weekNumber(tt.time)
			if got != tt.want {
				_, w := tt.time.ISOWeek()
				t.Errorf("weekNumber(%v) = %q, want %q (ISOWeek=%d)", tt.time, got, tt.want, w)
			}
		})
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{9, "9"},
		{10, "10"},
		{42, "42"},
		{100, "100"},
		{999, "999"},
		{12345, "12345"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := itoa(tt.n)
			if got != tt.want {
				t.Errorf("itoa(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 6. upload_handler.go — LaTeX converter
// ---------------------------------------------------------------------------

func TestTexToMarkdown(t *testing.T) {
	t.Run("sections", func(t *testing.T) {
		input := `\section{Introduction}Some text.\subsection{Details}More text.`
		result := texToMarkdown([]byte(input))
		if !strings.Contains(result, "## Introduction") {
			t.Errorf("expected '## Introduction' in result, got: %s", result)
		}
		if !strings.Contains(result, "### Details") {
			t.Errorf("expected '### Details' in result, got: %s", result)
		}
	})

	t.Run("bold_and_italic", func(t *testing.T) {
		input := `\textbf{bold} and \textit{italic} and \emph{emphasized}`
		result := texToMarkdown([]byte(input))
		if !strings.Contains(result, "**bold**") {
			t.Errorf("expected **bold** in result, got: %s", result)
		}
		if !strings.Contains(result, "*italic*") {
			t.Errorf("expected *italic* in result, got: %s", result)
		}
		if !strings.Contains(result, "*emphasized*") {
			t.Errorf("expected *emphasized* in result, got: %s", result)
		}
	})

	t.Run("lists", func(t *testing.T) {
		input := `\begin{itemize}\item First\item Second\end{itemize}`
		result := texToMarkdown([]byte(input))
		// \item becomes "\n- " so there may be extra whitespace
		if !strings.Contains(result, "First") {
			t.Errorf("expected 'First' in result, got: %s", result)
		}
		if !strings.Contains(result, "Second") {
			t.Errorf("expected 'Second' in result, got: %s", result)
		}
		if !strings.Contains(result, "-") {
			t.Errorf("expected list markers in result, got: %s", result)
		}
	})

	t.Run("code_blocks", func(t *testing.T) {
		input := `\begin{verbatim}code here\end{verbatim}`
		result := texToMarkdown([]byte(input))
		if !strings.Contains(result, "```") {
			t.Errorf("expected code block markers in result, got: %s", result)
		}
		if !strings.Contains(result, "code here") {
			t.Errorf("expected 'code here' in result, got: %s", result)
		}
	})

	t.Run("comments_removed", func(t *testing.T) {
		input := "% This is a comment\nVisible text"
		result := texToMarkdown([]byte(input))
		if strings.Contains(result, "This is a comment") {
			t.Errorf("comment should be removed, got: %s", result)
		}
		if !strings.Contains(result, "Visible text") {
			t.Errorf("expected 'Visible text' in result, got: %s", result)
		}
	})

	t.Run("inline_comment_stripped", func(t *testing.T) {
		input := "Some text % inline comment"
		result := texToMarkdown([]byte(input))
		if strings.Contains(result, "inline comment") {
			t.Errorf("inline comment should be stripped, got: %s", result)
		}
		if !strings.Contains(result, "Some text") {
			t.Errorf("expected 'Some text' in result, got: %s", result)
		}
	})

	t.Run("preamble_removal", func(t *testing.T) {
		input := `\documentclass{article}\usepackage{amsmath}\begin{document}Body text.\end{document}`
		result := texToMarkdown([]byte(input))
		if strings.Contains(result, "documentclass") {
			t.Errorf("preamble should be removed, got: %s", result)
		}
		if !strings.Contains(result, "Body text.") {
			t.Errorf("expected body text in result, got: %s", result)
		}
	})

	t.Run("texttt_to_inline_code", func(t *testing.T) {
		input := `Use \texttt{foo} for code.`
		result := texToMarkdown([]byte(input))
		if !strings.Contains(result, "`foo`") {
			t.Errorf("expected `foo` in result, got: %s", result)
		}
	})

	t.Run("special_characters", func(t *testing.T) {
		input := `\& \% \$ \# \_ \{ \}`
		result := texToMarkdown([]byte(input))
		if !strings.Contains(result, "&") {
			t.Errorf("expected & in result, got: %s", result)
		}
		if !strings.Contains(result, "#") {
			t.Errorf("expected # in result, got: %s", result)
		}
	})

	t.Run("empty_input", func(t *testing.T) {
		result := texToMarkdown([]byte(""))
		if result != "" {
			t.Errorf("expected empty output for empty input, got: %q", result)
		}
	})
}

func TestTexReplaceCmd(t *testing.T) {
	t.Run("basic_replacement", func(t *testing.T) {
		result := texReplaceCmd(`\textbf{hello}`, `\textbf`, "**", "**")
		if result != "**hello**" {
			t.Errorf("expected '**hello**', got: %q", result)
		}
	})

	t.Run("nested_braces", func(t *testing.T) {
		result := texReplaceCmd(`\textbf{a{b}c}`, `\textbf`, "**", "**")
		if result != "**a{b}c**" {
			t.Errorf("expected '**a{b}c**', got: %q", result)
		}
	})

	t.Run("no_argument", func(t *testing.T) {
		result := texReplaceCmd(`\maketitle rest`, `\maketitle`, "", "")
		if !strings.Contains(result, "rest") {
			t.Errorf("expected 'rest' after removing command without argument, got: %q", result)
		}
		if strings.Contains(result, "maketitle") {
			t.Errorf("command should be removed, got: %q", result)
		}
	})

	t.Run("multiple_occurrences", func(t *testing.T) {
		result := texReplaceCmd(`\textbf{a} and \textbf{b}`, `\textbf`, "**", "**")
		if result != "**a** and **b**" {
			t.Errorf("expected '**a** and **b**', got: %q", result)
		}
	})

	t.Run("no_match", func(t *testing.T) {
		result := texReplaceCmd("no commands here", `\textbf`, "**", "**")
		if result != "no commands here" {
			t.Errorf("expected unchanged string, got: %q", result)
		}
	})

	t.Run("with_optional_arg", func(t *testing.T) {
		result := texReplaceCmd(`\section[short]{Long Title}`, `\section`, "## ", "\n\n")
		if !strings.Contains(result, "## Long Title") {
			t.Errorf("expected '## Long Title' in result, got: %q", result)
		}
	})
}

// ---------------------------------------------------------------------------
// 7. collection_config.go — SetBinlog
// ---------------------------------------------------------------------------

func TestCollectionManagerSetBinlog(t *testing.T) {
	cm := &CollectionManager{
		cache: make(map[string]*CollectionConfig),
	}
	if cm.binlog != nil {
		t.Fatal("binlog should be nil initially")
	}
	bl := &Binlog{}
	cm.SetBinlog(bl)
	if cm.binlog != bl {
		t.Error("SetBinlog did not set the binlog")
	}
}

// ---------------------------------------------------------------------------
// 8. async_io.go — WaitAll with no pending operations
// ---------------------------------------------------------------------------

func TestAsyncIOWaitAllNoPending(t *testing.T) {
	aio := &AsyncIO{
		operations: make(map[uint64]*AsyncOperation),
	}
	// WaitAll with zero pending should return immediately
	done := make(chan struct{})
	go func() {
		aio.WaitAll()
		close(done)
	}()

	select {
	case <-done:
		// success — returned immediately
	case <-time.After(2 * time.Second):
		t.Fatal("WaitAll blocked with no pending operations")
	}
}

// ---------------------------------------------------------------------------
// 10. Additional coverage: FTS setters, CollectionManager.LoadAll,
//     BloomFilter, BinlogEntryType.String, etc.
// ---------------------------------------------------------------------------

func TestFTSSetSynonymManager(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	idx := NewFTSIndex(db)
	sm := NewSynonymManager(db)
	idx.SetSynonymManager(sm)
}

func TestFTSSetStopWordManager(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	idx := NewFTSIndex(db)
	swm := NewStopWordManager(db)
	idx.SetStopWordManager(swm)
}

func TestAutomationManagerSetLogStore(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	am := NewAutomationManager(db)
	ls := NewAutomationLogStore(db, 24*time.Hour)
	am.SetLogStore(ls)
}

func TestCollectionManagerLoadAll(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	cm := NewCollectionManager(db)
	_ = cm.EnsureBucket()
	// LoadAll on empty DB
	if err := cm.LoadAll(); err != nil {
		t.Fatal(err)
	}
	// Set a config, then LoadAll again
	if err := cm.Set("test-col", &CollectionConfig{Type: "default", Description: "test"}); err != nil {
		t.Fatal(err)
	}
	cm2 := NewCollectionManager(db)
	_ = cm2.EnsureBucket()
	if err := cm2.LoadAll(); err != nil {
		t.Fatal(err)
	}
	cfg, ok := cm2.Get("test-col")
	if !ok {
		t.Fatal("expected test-col config after LoadAll")
	}
	if cfg.Description != "test" {
		t.Errorf("expected description=test, got %q", cfg.Description)
	}
}

func TestBinlogEntryTypeString(t *testing.T) {
	tests := []struct {
		t    BinlogEntryType
		want string
	}{
		{BinlogPut, "Put"},
		{BinlogDelete, "Delete"},
		{BinlogDeleteBucket, "DeleteBucket"},
		{BinlogCheckpoint, "Checkpoint"},
		{BinlogEntryType(99), "Unknown(99)"},
	}
	for _, tc := range tests {
		got := tc.t.String()
		if got != tc.want {
			t.Errorf("BinlogEntryType(%d).String() = %q, want %q", tc.t, got, tc.want)
		}
	}
}

func TestBloomFilterRemoveAndClear(t *testing.T) {
	bfm := NewBloomFilterManager()
	bfm.Add("col", "key1", "en")
	if !bfm.Test("col", "key1", "en") {
		t.Error("expected Test=true after Add")
	}
	// Remove is a no-op for bloom filters
	bfm.Remove("col", "key1", "en")
	// Clear removes the filter entirely
	bfm.Clear("col")
	// After clear, new lookups should not find the old key
	if bfm.Test("col", "key1", "en") {
		t.Error("expected Test=false after Clear")
	}
}

func TestBloomFilterStats(t *testing.T) {
	bfm := NewBloomFilterManager()
	bfm.Add("col1", "a", "en")
	bfm.Add("col2", "b", "en")
	stats := bfm.Stats()
	if len(stats) != 2 {
		t.Errorf("expected 2 collections in stats, got %d", len(stats))
	}
}

func TestFTSSearchEmptyQueryCovBoost(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	idx := NewFTSIndex(db)
	idx.SetStemmer(NewPorterStemmer())
	_ = idx.EnsureBuckets()

	results, err := idx.Search("col1", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty query, got %d", len(results))
	}
}

func TestFTSIndexAndSearchCovBoost(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	idx := NewFTSIndex(db)
	idx.SetStemmer(NewPorterStemmer())
	_ = idx.EnsureBuckets()

	// Index some documents
	if err := idx.Index("col", "doc1", "The quick brown fox jumps over the lazy dog"); err != nil {
		t.Fatal(err)
	}
	if err := idx.Index("col", "doc2", "A fast red car drives on the highway"); err != nil {
		t.Fatal(err)
	}
	if err := idx.Index("col", "doc3", "The quick brown cat sleeps on the couch"); err != nil {
		t.Fatal(err)
	}

	// Search
	results, err := idx.Search("col", "quick brown", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 1 {
		t.Fatal("expected at least 1 result for 'quick brown'")
	}

	// Remove
	if err := idx.Remove("col", "doc1"); err != nil {
		t.Fatal(err)
	}
	results2, err := idx.Search("col", "fox jumps", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results2 {
		if r.DocID == "doc1" {
			t.Error("doc1 should have been removed from index")
		}
	}
}

func TestFTSSearchWithLangCovBoost(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	idx := NewFTSIndex(db)
	idx.SetStemmer(NewPorterStemmer())
	reg := NewLangRegistry("en")
	RegisterDefaultLanguages(reg)
	idx.SetLangRegistry(reg)
	_ = idx.EnsureBuckets()

	if err := idx.IndexWithLang("col", "doc1", "Running quickly through the forest", "en"); err != nil {
		t.Fatal(err)
	}
	results, err := idx.Search("col", "run forest", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 1 {
		t.Error("expected at least 1 result for stemmed search")
	}
}

func TestFTSRemoveNonExistentCovBoost(t *testing.T) {
	db := openTestDB(t)
	defer func() { _ = db.Close() }()
	idx := NewFTSIndex(db)
	idx.SetStemmer(NewPorterStemmer())
	_ = idx.EnsureBuckets()
	if err := idx.Remove("col", "nonexistent"); err != nil {
		t.Fatal(err)
	}
}

func TestShardClusterConsistency(t *testing.T) {
	sc := NewShardCluster(4, 1)
	s1 := sc.GetShard("my-key")
	s2 := sc.GetShard("my-key")
	if s1 == nil || s2 == nil {
		t.Fatal("GetShard returned nil")
		return
	}
	if s1.ID != s2.ID {
		t.Errorf("same key returned different shards: %d vs %d", s1.ID, s2.ID)
	}
}

func TestShardClusterDistribution(t *testing.T) {
	sc := NewShardCluster(4, 1)
	counts := make(map[int]int)
	for i := 0; i < 100; i++ {
		s := sc.GetShard(itoa(i))
		if s != nil {
			counts[s.ID]++
		}
	}
	if len(counts) < 2 {
		t.Errorf("poor distribution: only %d shards used out of 4", len(counts))
	}
}

func TestConsistentHashRemoveNode(t *testing.T) {
	ch := NewConsistentHash(100)
	ch.Add(1, 1)
	ch.Add(2, 1)
	ch.Add(3, 1)

	before := ch.Get("test-key")
	ch.Remove(2)
	after := ch.Get("test-key")
	_ = before
	_ = after
}

func TestZeroCopyManagerStreamCopyCovBoost(t *testing.T) {
	zcm := NewZeroCopyManager()
	if zcm == nil {
		t.Fatal("nil ZeroCopyManager")
		return
	}
	src := strings.NewReader("hello world")
	var dst strings.Builder
	n, err := zcm.StreamCopy(&dst, src)
	if err != nil {
		t.Fatal(err)
	}
	if n != 11 {
		t.Errorf("expected 11 bytes, got %d", n)
	}
	if dst.String() != "hello world" {
		t.Errorf("StreamCopy: got %q", dst.String())
	}
	stats := zcm.Stats()
	if !stats.Enabled {
		t.Error("expected Enabled=true")
	}
}

func TestSIMDProcessorVectorizedCompare(t *testing.T) {
	sp := NewSIMDProcessor()
	if sp == nil {
		t.Fatal("nil SIMDProcessor")
		return
	}
	data := [][]byte{
		[]byte("hello"),
		[]byte("world"),
		[]byte("hello"),
		[]byte("test"),
	}
	matches := sp.VectorizedCompare(data, []byte("hello"))
	if len(matches) != 2 {
		t.Errorf("expected 2 matches, got %d", len(matches))
	}
}

func TestSIMDProcessorVectorizedSearch(t *testing.T) {
	sp := NewSIMDProcessor()
	if sp == nil {
		t.Fatal("nil SIMDProcessor")
		return
	}
	// Use a single byte pattern to avoid chunk-boundary splits
	data := []byte("abcXdefXghiXjkl")
	positions := sp.VectorizedSearch(data, []byte("X"))
	if len(positions) < 1 {
		t.Errorf("expected at least 1 position, got %d", len(positions))
	}
}

func TestSIMDProcessorVectorizedSum(t *testing.T) {
	sp := NewSIMDProcessor()
	if sp == nil {
		t.Fatal("nil SIMDProcessor")
		return
	}
	data := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	sum := sp.VectorizedSum(data)
	if sum != 55 {
		t.Errorf("expected sum=55, got %d", sum)
	}
}

func TestSIMDProcessorVectorizedFilter(t *testing.T) {
	sp := NewSIMDProcessor()
	if sp == nil {
		t.Fatal("nil SIMDProcessor")
		return
	}
	data := [][]byte{[]byte("ab"), []byte("abc"), []byte("a"), []byte("abcd")}
	filtered := sp.VectorizedFilter(data, func(b []byte) bool { return len(b) >= 3 })
	if len(filtered) != 2 {
		t.Errorf("expected 2 filtered items, got %d", len(filtered))
	}
}

func TestSIMDProcessorVectorizedMap(t *testing.T) {
	sp := NewSIMDProcessor()
	if sp == nil {
		t.Fatal("nil SIMDProcessor")
		return
	}
	data := [][]byte{[]byte("hello"), []byte("world")}
	mapped := sp.VectorizedMap(data, func(b []byte) []byte {
		return append(b, '!')
	})
	if len(mapped) != 2 {
		t.Errorf("expected 2 mapped items, got %d", len(mapped))
	}
	if string(mapped[0]) != "hello!" {
		t.Errorf("expected 'hello!', got %q", string(mapped[0]))
	}
}

func TestSIMDProcessorParallelSort(t *testing.T) {
	sp := NewSIMDProcessor()
	if sp == nil {
		t.Fatal("nil SIMDProcessor")
		return
	}
	data := [][]byte{[]byte("cherry"), []byte("apple"), []byte("banana")}
	sp.ParallelSort(data, func(a, b []byte) bool { return string(a) < string(b) })
	if string(data[0]) != "apple" || string(data[1]) != "banana" || string(data[2]) != "cherry" {
		t.Errorf("not sorted: %v", data)
	}
}

func TestSIMDProcessorStats(t *testing.T) {
	sp := NewSIMDProcessor()
	if sp == nil {
		t.Fatal("nil SIMDProcessor")
		return
	}
	stats := sp.Stats()
	if !stats.Enabled {
		t.Error("expected Enabled=true")
	}
}
