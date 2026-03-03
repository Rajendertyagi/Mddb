package main

import (
	"testing"
)

// --- metadataEqual ---

func TestMetadataEqual_BothNil(t *testing.T) {
	if !metadataEqual(nil, nil) {
		t.Error("expected nil == nil to be true")
	}
}

func TestMetadataEqual_BothEmpty(t *testing.T) {
	a := map[string][]string{}
	b := map[string][]string{}
	if !metadataEqual(a, b) {
		t.Error("expected empty == empty to be true")
	}
}

func TestMetadataEqual_NilVsEmpty(t *testing.T) {
	// nil map has len 0, empty map has len 0 => equal
	if !metadataEqual(nil, map[string][]string{}) {
		t.Error("expected nil == empty to be true")
	}
}

func TestMetadataEqual_IdenticalSingleKey(t *testing.T) {
	a := map[string][]string{"tag": {"go"}}
	b := map[string][]string{"tag": {"go"}}
	if !metadataEqual(a, b) {
		t.Error("expected identical single-key maps to be equal")
	}
}

func TestMetadataEqual_IdenticalMultiKey(t *testing.T) {
	a := map[string][]string{"tag": {"go", "db"}, "author": {"alice"}}
	b := map[string][]string{"tag": {"go", "db"}, "author": {"alice"}}
	if !metadataEqual(a, b) {
		t.Error("expected identical multi-key maps to be equal")
	}
}

func TestMetadataEqual_DifferentKeys(t *testing.T) {
	a := map[string][]string{"tag": {"go"}}
	b := map[string][]string{"cat": {"go"}}
	if metadataEqual(a, b) {
		t.Error("expected maps with different keys to be not equal")
	}
}

func TestMetadataEqual_DifferentValues(t *testing.T) {
	a := map[string][]string{"tag": {"go"}}
	b := map[string][]string{"tag": {"python"}}
	if metadataEqual(a, b) {
		t.Error("expected maps with different values to be not equal")
	}
}

func TestMetadataEqual_DifferentValueCount(t *testing.T) {
	a := map[string][]string{"tag": {"go", "db"}}
	b := map[string][]string{"tag": {"go"}}
	if metadataEqual(a, b) {
		t.Error("expected maps with different value counts to be not equal")
	}
}

func TestMetadataEqual_DifferentKeyCount(t *testing.T) {
	a := map[string][]string{"tag": {"go"}, "x": {"y"}}
	b := map[string][]string{"tag": {"go"}}
	if metadataEqual(a, b) {
		t.Error("expected maps with different key counts to be not equal")
	}
}

func TestMetadataEqual_OrderMatters(t *testing.T) {
	a := map[string][]string{"tag": {"go", "db"}}
	b := map[string][]string{"tag": {"db", "go"}}
	if metadataEqual(a, b) {
		t.Error("expected maps with different value order to be not equal (order matters)")
	}
}

func TestMetadataEqual_ExtraKeyInB(t *testing.T) {
	a := map[string][]string{"tag": {"go"}}
	b := map[string][]string{"tag": {"go"}, "extra": {"val"}}
	if metadataEqual(a, b) {
		t.Error("expected maps with extra key in b to be not equal")
	}
}

func TestMetadataEqual_EmptyValues(t *testing.T) {
	a := map[string][]string{"tag": {}}
	b := map[string][]string{"tag": {}}
	if !metadataEqual(a, b) {
		t.Error("expected maps with empty value slices to be equal")
	}
}

// --- metadataChanged ---

func TestMetadataChanged_BothNil(t *testing.T) {
	if metadataChanged(nil, nil) {
		t.Error("expected both nil to report no change")
	}
}

func TestMetadataChanged_BothEmpty(t *testing.T) {
	a := map[string][]string{}
	b := map[string][]string{}
	if metadataChanged(a, b) {
		t.Error("expected both empty to report no change")
	}
}

func TestMetadataChanged_EmptyExistingNewHasData(t *testing.T) {
	existing := map[string][]string{}
	newMeta := map[string][]string{"tag": {"go"}}
	if !metadataChanged(existing, newMeta) {
		t.Error("expected change when existing is empty and new has data")
	}
}

func TestMetadataChanged_NilExistingNewHasData(t *testing.T) {
	newMeta := map[string][]string{"tag": {"go"}}
	if !metadataChanged(nil, newMeta) {
		t.Error("expected change when existing is nil and new has data")
	}
}

func TestMetadataChanged_NilExistingNilNew(t *testing.T) {
	if metadataChanged(nil, nil) {
		t.Error("expected no change when both are nil")
	}
}

func TestMetadataChanged_SameMeta(t *testing.T) {
	a := map[string][]string{"tag": {"go", "db"}}
	b := map[string][]string{"tag": {"go", "db"}}
	if metadataChanged(a, b) {
		t.Error("expected no change when metadata is identical")
	}
}

func TestMetadataChanged_DifferentMeta(t *testing.T) {
	a := map[string][]string{"tag": {"go"}}
	b := map[string][]string{"tag": {"python"}}
	if !metadataChanged(a, b) {
		t.Error("expected change when metadata values differ")
	}
}

func TestMetadataChanged_AddedKey(t *testing.T) {
	a := map[string][]string{"tag": {"go"}}
	b := map[string][]string{"tag": {"go"}, "author": {"alice"}}
	if !metadataChanged(a, b) {
		t.Error("expected change when new has additional key")
	}
}

func TestMetadataChanged_RemovedKey(t *testing.T) {
	a := map[string][]string{"tag": {"go"}, "author": {"alice"}}
	b := map[string][]string{"tag": {"go"}}
	if !metadataChanged(a, b) {
		t.Error("expected change when key removed")
	}
}

func TestMetadataChanged_ValueOrderChanged(t *testing.T) {
	a := map[string][]string{"tag": {"go", "db"}}
	b := map[string][]string{"tag": {"db", "go"}}
	if !metadataChanged(a, b) {
		t.Error("expected change when value order changed (order matters)")
	}
}

func TestMetadataChanged_ExistingHasDataNewEmpty(t *testing.T) {
	a := map[string][]string{"tag": {"go"}}
	b := map[string][]string{}
	if !metadataChanged(a, b) {
		t.Error("expected change when existing has data but new is empty")
	}
}

func TestMetadataChanged_ExistingHasDataNewNil(t *testing.T) {
	a := map[string][]string{"tag": {"go"}}
	if !metadataChanged(a, nil) {
		t.Error("expected change when existing has data but new is nil")
	}
}

// --- Table-driven tests ---

func TestMetadataEqual_TableDriven(t *testing.T) {
	tests := []struct {
		name string
		a, b map[string][]string
		want bool
	}{
		{"both nil", nil, nil, true},
		{"both empty", map[string][]string{}, map[string][]string{}, true},
		{"nil vs empty", nil, map[string][]string{}, true},
		{"single match", map[string][]string{"k": {"v"}}, map[string][]string{"k": {"v"}}, true},
		{"multi match", map[string][]string{"a": {"1", "2"}, "b": {"3"}}, map[string][]string{"a": {"1", "2"}, "b": {"3"}}, true},
		{"diff value", map[string][]string{"k": {"v1"}}, map[string][]string{"k": {"v2"}}, false},
		{"diff key", map[string][]string{"k1": {"v"}}, map[string][]string{"k2": {"v"}}, false},
		{"extra key a", map[string][]string{"k": {"v"}, "x": {"y"}}, map[string][]string{"k": {"v"}}, false},
		{"extra key b", map[string][]string{"k": {"v"}}, map[string][]string{"k": {"v"}, "x": {"y"}}, false},
		{"order differs", map[string][]string{"k": {"a", "b"}}, map[string][]string{"k": {"b", "a"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metadataEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("metadataEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestMetadataChanged_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		existing map[string][]string
		newMeta  map[string][]string
		want     bool
	}{
		{"both nil", nil, nil, false},
		{"both empty", map[string][]string{}, map[string][]string{}, false},
		{"nil->data", nil, map[string][]string{"k": {"v"}}, true},
		{"empty->data", map[string][]string{}, map[string][]string{"k": {"v"}}, true},
		{"same", map[string][]string{"k": {"v"}}, map[string][]string{"k": {"v"}}, false},
		{"changed", map[string][]string{"k": {"v1"}}, map[string][]string{"k": {"v2"}}, true},
		{"added key", map[string][]string{"k": {"v"}}, map[string][]string{"k": {"v"}, "k2": {"v2"}}, true},
		{"removed key", map[string][]string{"k": {"v"}, "k2": {"v2"}}, map[string][]string{"k": {"v"}}, true},
		{"data->nil", map[string][]string{"k": {"v"}}, nil, true},
		{"data->empty", map[string][]string{"k": {"v"}}, map[string][]string{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metadataChanged(tt.existing, tt.newMeta)
			if got != tt.want {
				t.Errorf("metadataChanged(%v, %v) = %v, want %v", tt.existing, tt.newMeta, got, tt.want)
			}
		})
	}
}
