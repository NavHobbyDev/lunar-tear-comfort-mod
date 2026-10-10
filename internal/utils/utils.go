package utils

import (
	"encoding/json"
	"fmt"
	"lunar-tear/server/internal/masterdata/memorydb"
)

// ReadTable deserializes a master data table from the in-memory binary store.
// The key is the snake_case table name as it appears in the binary header
// (e.g. "m_weapon", "m_costume").
func ReadTable[T any](key string) ([]T, error) {
	return memorydb.ReadTable[T](key)
}

func EncodeJSONMaps(records ...map[string]any) (string, error) {
	jsonBytes, err := json.Marshal(records)
	if err != nil {
		return "", fmt.Errorf("json marshal maps: %w", err)
	}
	return string(jsonBytes), nil
}
