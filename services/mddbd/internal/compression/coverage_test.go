package compression

import (
	"bytes"
	"crypto/rand"
	"testing"
)

// restoreDefaults resets the package-global compression config after a test that
// mutates it via ConfigureCompression, so it cannot leak into sibling tests.
func restoreDefaults(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { ConfigureCompression(true, 1024, 10*1024) })
}

func TestConfigureCompressionThresholds(t *testing.T) {
	restoreDefaults(t)

	// Positive thresholds are applied.
	ConfigureCompression(true, 512, 4096)
	doc := bytes.Repeat([]byte("ab"), 400) // 800 bytes: >= 512 (small) and < 4096 (medium)
	if got := CompressDoc(doc)[0]; got != FlagSnappy {
		t.Errorf("after threshold reconfig, 800B repetitive doc flag = %d, want FlagSnappy", got)
	}

	// Zero/negative thresholds are ignored (the >0 == false branches): the 512/
	// 4096 values from above must still hold, so the same doc still uses Snappy.
	ConfigureCompression(true, 0, -1)
	if got := CompressDoc(doc)[0]; got != FlagSnappy {
		t.Errorf("zero thresholds should not change config; flag = %d, want FlagSnappy", got)
	}
}

func TestCompressDocDisabled(t *testing.T) {
	restoreDefaults(t)
	ConfigureCompression(false, 0, 0)

	// Even a large, highly compressible doc must pass through uncompressed.
	big := bytes.Repeat([]byte("compressible payload "), 2000) // ~42KB
	out := CompressDoc(big)
	if out[0] != FlagUncompressed {
		t.Errorf("disabled compression: flag = %d, want FlagUncompressed", out[0])
	}
	if !bytes.Equal(out[1:], big) {
		t.Error("disabled compression must preserve the payload verbatim")
	}
}

func TestCompressDocIncompressibleFallback(t *testing.T) {
	restoreDefaults(t)
	ConfigureCompression(true, 1024, 10*1024)

	// Random data does not shrink, so both the Snappy (medium) and Zstd (large)
	// branches must fall back to FlagUncompressed rather than store a larger blob.
	medium := make([]byte, 4*1024) // 1KB..10KB -> Snappy attempt
	if _, err := rand.Read(medium); err != nil {
		t.Fatal(err)
	}
	if got := CompressDoc(medium)[0]; got != FlagUncompressed {
		t.Errorf("incompressible medium: flag = %d, want FlagUncompressed (Snappy didn't help)", got)
	}

	large := make([]byte, 32*1024) // >10KB -> Zstd attempt
	if _, err := rand.Read(large); err != nil {
		t.Fatal(err)
	}
	if got := CompressDoc(large)[0]; got != FlagUncompressed {
		t.Errorf("incompressible large: flag = %d, want FlagUncompressed (Zstd didn't help)", got)
	}
}
