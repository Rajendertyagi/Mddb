package compression

import (
	"errors"

	"github.com/golang/snappy"
	"github.com/klauspost/compress/zstd"
)

const (
	FlagUncompressed = byte(0)
	FlagSnappy       = byte(1)
	FlagZstd         = byte(2)
)

var (
	compressionEnabled         = true
	compressionThresholdSmall  = 1024      // 1KB
	compressionThresholdMedium = 10 * 1024 // 10KB
)

// ConfigureCompression sets compression parameters from config.
func ConfigureCompression(enabled bool, smallThreshold, mediumThreshold int) {
	compressionEnabled = enabled
	if smallThreshold > 0 {
		compressionThresholdSmall = smallThreshold
	}
	if mediumThreshold > 0 {
		compressionThresholdMedium = mediumThreshold
	}
}

var (
	zstdEncoder *zstd.Encoder
	zstdDecoder *zstd.Decoder
)

func init() {
	var err error
	// Initialize zstd encoder (level 3 - balanced)
	zstdEncoder, err = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		panic(err)
	}

	// Initialize zstd decoder
	zstdDecoder, err = zstd.NewReader(nil)
	if err != nil {
		panic(err)
	}
}

// CompressDoc compresses document data with adaptive compression levels
func CompressDoc(data []byte) []byte {
	dataLen := len(data)

	// Compression disabled
	if !compressionEnabled {
		result := make([]byte, dataLen+1)
		result[0] = FlagUncompressed
		copy(result[1:], data)
		return result
	}

	// Small documents - no compression
	if dataLen < compressionThresholdSmall {
		result := make([]byte, dataLen+1)
		result[0] = FlagUncompressed
		copy(result[1:], data)
		return result
	}

	// Medium documents (1KB-10KB) - use Snappy (fast)
	if dataLen < compressionThresholdMedium {
		compressed := snappy.Encode(nil, data)

		// Only use if beneficial
		if len(compressed) < dataLen {
			result := make([]byte, len(compressed)+1)
			result[0] = FlagSnappy
			copy(result[1:], compressed)
			return result
		}

		// Compression didn't help
		result := make([]byte, dataLen+1)
		result[0] = FlagUncompressed
		copy(result[1:], data)
		return result
	}

	// Large documents (>10KB) - use Zstd (high ratio)
	compressed := zstdEncoder.EncodeAll(data, nil)

	// Only use if beneficial
	if len(compressed) < dataLen {
		result := make([]byte, len(compressed)+1)
		result[0] = FlagZstd
		copy(result[1:], compressed)
		return result
	}

	// Compression didn't help
	result := make([]byte, dataLen+1)
	result[0] = FlagUncompressed
	copy(result[1:], data)
	return result
}

// DecompressDoc decompresses document data with adaptive decompression
func DecompressDoc(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("empty data")
	}

	flag := data[0]
	payload := data[1:]

	switch flag {
	case FlagUncompressed:
		return payload, nil

	case FlagSnappy:
		decompressed, err := snappy.Decode(nil, payload)
		if err != nil {
			return nil, err
		}
		return decompressed, nil

	case FlagZstd:
		decompressed, err := zstdDecoder.DecodeAll(payload, nil)
		if err != nil {
			return nil, err
		}
		return decompressed, nil

	default:
		// No flag - assume old format (uncompressed)
		return data, nil
	}
}

// CompressionStats returns compression statistics
type CompressionStats struct {
	OriginalSize   int
	CompressedSize int
	Ratio          float64
	Method         string
}

// GetCompressionStats analyzes compression for data
func GetCompressionStats(data []byte) CompressionStats {
	compressed := CompressDoc(data)

	method := "none"
	switch compressed[0] {
	case FlagSnappy:
		method = "snappy"
	case FlagZstd:
		method = "zstd"
	}

	ratio := 1.0
	if len(data) > 0 {
		ratio = float64(len(compressed)) / float64(len(data))
	}

	return CompressionStats{
		OriginalSize:   len(data),
		CompressedSize: len(compressed),
		Ratio:          ratio,
		Method:         method,
	}
}
