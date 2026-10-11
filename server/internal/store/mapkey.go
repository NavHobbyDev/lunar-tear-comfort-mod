package store

import (
	"fmt"
	"strconv"
	"strings"
)

func marshalKey(vals ...int64) []byte {
	b := strconv.AppendInt(nil, vals[0], 10)
	for _, v := range vals[1:] {
		b = append(b, ':')
		b = strconv.AppendInt(b, v, 10)
	}
	return b
}

func unmarshalKey(text []byte, name string, n int) ([]int64, error) {
	parts := strings.Split(string(text), ":")
	if len(parts) != n {
		return nil, fmt.Errorf("invalid %s: %s", name, text)
	}
	out := make([]int64, n)
	for i, p := range parts {
		v, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}
