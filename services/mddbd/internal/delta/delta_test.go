package delta

import (
	"bytes"
	"testing"
)

func TestNewDeltaEncoder(t *testing.T) {
	de := NewDeltaEncoder()
	if de == nil {
		t.Fatal("NewDeltaEncoder returned nil")
	}
}

func TestDeltaEncoder_EncodeDecodeFullData(t *testing.T) {
	de := NewDeltaEncoder()

	// When oldData is empty, should store full data
	newData := []byte("hello world")
	encoded := de.Encode(nil, newData)

	if encoded[0] != 0 {
		t.Errorf("flag byte = %d, want 0 (full data)", encoded[0])
	}

	decoded, err := de.Decode(nil, encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !bytes.Equal(decoded, newData) {
		t.Errorf("decoded = %q, want %q", decoded, newData)
	}
}

func TestDeltaEncoder_EncodeDecodeEmptyOldData(t *testing.T) {
	de := NewDeltaEncoder()

	newData := []byte("new content")
	encoded := de.Encode([]byte{}, newData)

	// Empty old data -> full data flag
	if encoded[0] != 0 {
		t.Errorf("flag = %d, want 0", encoded[0])
	}

	decoded, err := de.Decode([]byte{}, encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !bytes.Equal(decoded, newData) {
		t.Errorf("decoded = %q, want %q", decoded, newData)
	}
}

func TestDeltaEncoder_EncodeDelta(t *testing.T) {
	de := NewDeltaEncoder()

	oldData := []byte("Hello, World! This is a test document with some content.")
	newData := []byte("Hello, World! This is a modified document with some content.")

	encoded := de.Encode(oldData, newData)
	if len(encoded) == 0 {
		t.Fatal("Encode returned empty data")
	}

	decoded, err := de.Decode(oldData, encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !bytes.Equal(decoded, newData) {
		t.Errorf("decoded = %q, want %q", decoded, newData)
	}
}

func TestDeltaEncoder_EncodeIdenticalData(t *testing.T) {
	de := NewDeltaEncoder()

	data := []byte("identical content")
	encoded := de.Encode(data, data)

	decoded, err := de.Decode(data, encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !bytes.Equal(decoded, data) {
		t.Errorf("decoded = %q, want %q", decoded, data)
	}
}

func TestDeltaEncoder_DecodeEmptyEncodedData(t *testing.T) {
	de := NewDeltaEncoder()

	_, err := de.Decode(nil, []byte{})
	if err == nil {
		t.Error("Decode with empty encoded data should return error")
	}
}

func TestDeltaEncoder_DecodeDeltaWithoutBase(t *testing.T) {
	de := NewDeltaEncoder()

	oldData := []byte("Hello, World! base content here for delta")
	newData := []byte("Hello, World! modified content here for delta")
	encoded := de.Encode(oldData, newData)

	// If encoded as delta, decoding without base should fail
	if encoded[0] == 1 {
		_, err := de.Decode(nil, encoded)
		if err == nil {
			t.Error("Decode delta without base data should return error")
		}
	}
}

func TestDeltaEncoder_DecodeInvalidDelta(t *testing.T) {
	de := NewDeltaEncoder()

	// Flag=1 (delta) but only a few bytes of data (too short for header)
	_, err := de.Decode([]byte("base"), []byte{1, 0, 0})
	if err == nil {
		t.Error("Decode with invalid delta format should return error")
	}
}

func TestDeltaEncoder_LargeData(t *testing.T) {
	de := NewDeltaEncoder()

	// Create large data with a small change
	oldData := bytes.Repeat([]byte("A"), 10000)
	newData := make([]byte, 10000)
	copy(newData, oldData)
	newData[5000] = 'B'

	encoded := de.Encode(oldData, newData)

	decoded, err := de.Decode(oldData, encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !bytes.Equal(decoded, newData) {
		t.Error("large data round-trip failed")
	}
}

func TestDeltaEncoder_Stats(t *testing.T) {
	de := NewDeltaEncoder()

	oldData := []byte("Hello, World! This is a test document with some content that is fairly long.")
	newData := []byte("Hello, World! This is a modified document with some content that is fairly long.")

	origSize, deltaSize, ratio := de.Stats(oldData, newData)

	if origSize != len(newData) {
		t.Errorf("originalSize = %d, want %d", origSize, len(newData))
	}
	if deltaSize <= 0 {
		t.Errorf("deltaSize = %d, want > 0", deltaSize)
	}
	if ratio <= 0 {
		t.Errorf("ratio = %f, want > 0", ratio)
	}
}

func TestDeltaEncoder_StatsEmptyOld(t *testing.T) {
	de := NewDeltaEncoder()

	origSize, deltaSize, ratio := de.Stats(nil, []byte("new data"))

	if origSize != 8 {
		t.Errorf("originalSize = %d, want 8", origSize)
	}
	if deltaSize <= 0 {
		t.Error("deltaSize should be > 0")
	}
	if ratio <= 0 {
		t.Error("ratio should be > 0")
	}
}

func TestDeltaEncoder_StatsEmptyNew(t *testing.T) {
	de := NewDeltaEncoder()

	origSize, _, ratio := de.Stats([]byte("old"), []byte{})

	if origSize != 0 {
		t.Errorf("originalSize = %d, want 0", origSize)
	}
	if ratio != 0 {
		t.Errorf("ratio = %f, want 0", ratio)
	}
}

func TestDeltaEncoder_PrefixOnlyChange(t *testing.T) {
	de := NewDeltaEncoder()

	oldData := []byte("XXXXcommon suffix remains the same here")
	newData := []byte("YYYYcommon suffix remains the same here")

	encoded := de.Encode(oldData, newData)
	decoded, err := de.Decode(oldData, encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !bytes.Equal(decoded, newData) {
		t.Errorf("prefix change: decoded = %q, want %q", decoded, newData)
	}
}

func TestDeltaEncoder_SuffixOnlyChange(t *testing.T) {
	de := NewDeltaEncoder()

	oldData := []byte("common prefix remains the same hereXXXX")
	newData := []byte("common prefix remains the same hereYYYY")

	encoded := de.Encode(oldData, newData)
	decoded, err := de.Decode(oldData, encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !bytes.Equal(decoded, newData) {
		t.Errorf("suffix change: decoded = %q, want %q", decoded, newData)
	}
}

func TestDeltaEncoder_CompletelyDifferent(t *testing.T) {
	de := NewDeltaEncoder()

	oldData := []byte("AAAAAAAAAA")
	newData := []byte("BBBBBBBBBB")

	encoded := de.Encode(oldData, newData)
	decoded, err := de.Decode(oldData, encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !bytes.Equal(decoded, newData) {
		t.Errorf("completely different: decoded = %q, want %q", decoded, newData)
	}
}

func TestDeltaEncoder_DifferentLengths(t *testing.T) {
	de := NewDeltaEncoder()

	tests := []struct {
		name    string
		oldData []byte
		newData []byte
	}{
		{"shorter to longer", []byte("short"), []byte("this is much longer content")},
		{"longer to shorter", []byte("this is much longer content"), []byte("short")},
		{"one byte old", []byte("X"), []byte("Hello World")},
		{"one byte new", []byte("Hello World"), []byte("X")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := de.Encode(tt.oldData, tt.newData)
			decoded, err := de.Decode(tt.oldData, encoded)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if !bytes.Equal(decoded, tt.newData) {
				t.Errorf("decoded = %q, want %q", decoded, tt.newData)
			}
		})
	}
}
