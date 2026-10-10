package memorydb

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/pierrec/lz4/v4"
	"github.com/vmihailenco/msgpack/v5"
)

var tables map[string][]byte

const (
	aesKeyHex  = "36436230313332314545356536624265"
	aesIVHex   = "45666341656634434165356536446141"
	lz4ExtCode = int8(99)
)

func Init(binPath string) error {
	encrypted, err := os.ReadFile(binPath)
	if err != nil {
		return fmt.Errorf("read bin.e: %w", err)
	}

	decrypted, err := decrypt(encrypted)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}

	toc, dataBlob, err := parseHeader(decrypted)
	if err != nil {
		return fmt.Errorf("parse header: %w", err)
	}

	tables = make(map[string][]byte, len(toc))
	for name, offLen := range toc {
		off := offLen[0]
		length := offLen[1]
		if off+length > len(dataBlob) {
			return fmt.Errorf("table %q: offset %d + length %d exceeds data blob size %d", name, off, length, len(dataBlob))
		}
		tables[name] = dataBlob[off : off+length]
	}

	return nil
}

func TableCount() int {
	return len(tables)
}

// TableNames returns the names of all tables present in the loaded bin.
func TableNames() []string {
	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	return names
}

func TableBytes(key string) ([]byte, bool) {
	b, ok := tables[key]
	return b, ok
}

func ReadTable[T any](key string) ([]T, error) {
	raw, ok := TableBytes(key)
	if !ok {
		return nil, fmt.Errorf("table %q not found in master data", key)
	}
	return decompressAndUnmarshal[T](raw)
}

func decrypt(data []byte) ([]byte, error) {
	key, err := hex.DecodeString(aesKeyHex)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}
	iv, err := hex.DecodeString(aesIVHex)
	if err != nil {
		return nil, fmt.Errorf("decode iv: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}

	if len(data)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext length %d is not a multiple of block size %d", len(data), aes.BlockSize)
	}

	decrypted := make([]byte, len(data))
	cbc := cipher.NewCBCDecrypter(block, iv)
	cbc.CryptBlocks(decrypted, data)

	decrypted, err = pkcs7Unpad(decrypted, aes.BlockSize)
	if err != nil {
		return nil, fmt.Errorf("unpad: %w", err)
	}
	return decrypted, nil
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > blockSize || padLen > len(data) {
		return nil, fmt.Errorf("invalid padding length %d", padLen)
	}
	for _, b := range data[len(data)-padLen:] {
		if int(b) != padLen {
			return nil, fmt.Errorf("invalid padding byte")
		}
	}
	return data[:len(data)-padLen], nil
}

func parseHeader(data []byte) (map[string][2]int, []byte, error) {
	// Decode the header (first msgpack object) using Decode into interface{},
	// then compute how many bytes it consumed to find the data blob start.
	r := bytes.NewReader(data)
	dec := msgpack.NewDecoder(r)
	dec.UseLooseInterfaceDecoding(true)

	var headerRaw interface{}
	if err := dec.Decode(&headerRaw); err != nil {
		return nil, nil, fmt.Errorf("decode header: %w", err)
	}

	headerMap, ok := headerRaw.(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("header is not a map, got %T", headerRaw)
	}

	toc := make(map[string][2]int, len(headerMap))
	for name, val := range headerMap {
		arr, ok := val.([]interface{})
		if !ok || len(arr) != 2 {
			return nil, nil, fmt.Errorf("table %q: expected [offset, length] array, got %T", name, val)
		}
		offset, err := toInt(arr[0])
		if err != nil {
			return nil, nil, fmt.Errorf("table %q offset: %w", name, err)
		}
		length, err := toInt(arr[1])
		if err != nil {
			return nil, nil, fmt.Errorf("table %q length: %w", name, err)
		}
		toc[name] = [2]int{offset, length}
	}

	consumed := int(int64(len(data)) - int64(r.Len()))
	return toc, data[consumed:], nil
}

func toInt(v interface{}) (int, error) {
	switch n := v.(type) {
	case int8:
		return int(n), nil
	case int16:
		return int(n), nil
	case int32:
		return int(n), nil
	case int64:
		return int(n), nil
	case uint8:
		return int(n), nil
	case uint16:
		return int(n), nil
	case uint32:
		return int(n), nil
	case uint64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

func decompressAndUnmarshal[T any](raw []byte) ([]T, error) {
	// Peek at the raw msgpack to check if it's an ext type (LZ4 compressed)
	// or a plain array.
	if len(raw) == 0 {
		return nil, nil
	}

	// Try to decode as ext type first
	dec := msgpack.NewDecoder(bytes.NewReader(raw))
	code, extData, err := decodeExt(dec)
	if err == nil && code == lz4ExtCode {
		uncompressedSize, lz4Data, err := readLZ4ExtHeader(extData)
		if err != nil {
			return nil, fmt.Errorf("read lz4 ext header: %w", err)
		}

		decompressed := make([]byte, uncompressedSize)
		n, err := lz4.UncompressBlock(lz4Data, decompressed)
		if err != nil {
			return nil, fmt.Errorf("lz4 decompress: %w", err)
		}
		decompressed = decompressed[:n]

		var result []T
		if err := msgpack.Unmarshal(decompressed, &result); err != nil {
			return nil, fmt.Errorf("unmarshal decompressed table: %w", err)
		}
		return result, nil
	}

	// Not LZ4 compressed, try as plain array
	var result []T
	if err := msgpack.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("unmarshal plain table: %w", err)
	}
	return result, nil
}

func decodeExt(dec *msgpack.Decoder) (int8, []byte, error) {
	var ext msgpack.RawMessage
	if err := dec.Decode(&ext); err != nil {
		return 0, nil, err
	}
	// ext is the full msgpack ext bytes including the header.
	// Re-decode just the header to get the type code, then the body is the rest.
	innerDec := msgpack.NewDecoder(bytes.NewReader(ext))
	extID, extLen, err := innerDec.DecodeExtHeader()
	if err != nil {
		return 0, nil, err
	}
	extData := make([]byte, extLen)
	if _, err := innerDec.Buffered().Read(extData); err != nil {
		return 0, nil, fmt.Errorf("read ext data: %w", err)
	}
	return extID, extData, nil
}

func readLZ4ExtHeader(data []byte) (int, []byte, error) {
	if len(data) == 0 {
		return 0, nil, fmt.Errorf("empty ext data")
	}
	tag := data[0]
	switch {
	case tag == 0xd2: // big-endian int32
		if len(data) < 5 {
			return 0, nil, fmt.Errorf("not enough data for int32 size")
		}
		size := int(int32(binary.BigEndian.Uint32(data[1:5])))
		return size, data[5:], nil
	case tag == 0xce: // big-endian uint32
		if len(data) < 5 {
			return 0, nil, fmt.Errorf("not enough data for uint32 size")
		}
		size := int(binary.BigEndian.Uint32(data[1:5]))
		return size, data[5:], nil
	case tag <= 0x7f: // positive fixint
		return int(tag), data[1:], nil
	default:
		return 0, nil, fmt.Errorf("unexpected tag 0x%02x in LZ4 ext header", tag)
	}
}
