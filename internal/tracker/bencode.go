package tracker

import (
	"bytes"
	"fmt"
	"strconv"
)

type Response struct {
	Interval    int
	MinInterval int
	Seeders     int
	Leechers    int
	Failure     string
}

// maxBencodeDepth 限制 bencode 解析的最大嵌套深度，防止恶意/异常的深层嵌套响应
// 耗尽 goroutine 栈内存。tracker 响应已被 maxTrackerResponseBytes 限制为 1MB，
// 但纯嵌套结构（如 "dddd..."）仍可产生数十万层递归。
const maxBencodeDepth = 200

func ParseResponse(data []byte) (Response, error) {
	v, rest, err := parseValue(data, 0)
	if err != nil {
		return Response{}, err
	}
	if len(bytes.TrimSpace(rest)) != 0 {
		return Response{}, fmt.Errorf("trailing bencode data")
	}
	dict, ok := v.(map[string]any)
	if !ok {
		return Response{}, fmt.Errorf("tracker response is not a dictionary")
	}
	var r Response
	r.Interval = intValue(dict["interval"])
	r.MinInterval = intValue(dict["min interval"])
	r.Seeders = intValue(dict["complete"]) - 1
	if r.Seeders < 0 {
		r.Seeders = 0
	}
	r.Leechers = intValue(dict["incomplete"])
	if s, ok := dict["failure reason"].(string); ok {
		r.Failure = s
	}
	return r, nil
}

func parseValue(data []byte, depth int) (any, []byte, error) {
	if depth > maxBencodeDepth {
		return nil, nil, fmt.Errorf("bencode nesting too deep (>%d)", maxBencodeDepth)
	}
	if len(data) == 0 {
		return nil, nil, fmt.Errorf("unexpected end of bencode")
	}
	switch data[0] {
	case 'i':
		end := bytes.IndexByte(data, 'e')
		if end < 0 {
			return nil, nil, fmt.Errorf("unterminated int")
		}
		n, err := strconv.ParseInt(string(data[1:end]), 10, 64)
		if err != nil {
			return nil, nil, err
		}
		return n, data[end+1:], nil
	case 'd':
		out := map[string]any{}
		rest := data[1:]
		for len(rest) > 0 && rest[0] != 'e' {
			k, next, err := parseValue(rest, depth+1)
			if err != nil {
				return nil, nil, err
			}
			key, ok := k.(string)
			if !ok {
				return nil, nil, fmt.Errorf("dictionary key is not string")
			}
			val, next, err := parseValue(next, depth+1)
			if err != nil {
				return nil, nil, err
			}
			out[key] = val
			rest = next
		}
		if len(rest) == 0 {
			return nil, nil, fmt.Errorf("unterminated dict")
		}
		return out, rest[1:], nil
	case 'l':
		var out []any
		rest := data[1:]
		for len(rest) > 0 && rest[0] != 'e' {
			v, next, err := parseValue(rest, depth+1)
			if err != nil {
				return nil, nil, err
			}
			out = append(out, v)
			rest = next
		}
		if len(rest) == 0 {
			return nil, nil, fmt.Errorf("unterminated list")
		}
		return out, rest[1:], nil
	default:
		if data[0] < '0' || data[0] > '9' {
			return nil, nil, fmt.Errorf("unexpected bencode byte %q", data[0])
		}
		colon := bytes.IndexByte(data, ':')
		if colon < 0 {
			return nil, nil, fmt.Errorf("invalid string")
		}
		n, err := strconv.Atoi(string(data[:colon]))
		if err != nil {
			return nil, nil, err
		}
		start := colon + 1
		end := start + n
		if end > len(data) {
			return nil, nil, fmt.Errorf("string exceeds input")
		}
		return string(data[start:end]), data[end:], nil
	}
}

func intValue(v any) int {
	switch n := v.(type) {
	case int64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}
