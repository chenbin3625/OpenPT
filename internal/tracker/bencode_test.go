package tracker

import (
	"strings"
	"testing"
)

func TestParseTrackerResponse(t *testing.T) {
	resp, err := ParseResponse([]byte("d8:intervali1800e8:completei3e10:incompletei2ee"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Interval != 1800 || resp.Seeders != 2 || resp.Leechers != 2 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestParseTrackerFailure(t *testing.T) {
	resp, err := ParseResponse([]byte("d14:failure reason14:not registerede"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Failure != "not registered" {
		t.Fatalf("failure = %q", resp.Failure)
	}
}

func TestParseTrackerMinInterval(t *testing.T) {
	// tracker 同时返回 interval 与 min interval，应正确解析 min interval
	resp, err := ParseResponse([]byte("d8:intervali1800e12:min intervali2400e8:completei3e10:incompletei2ee"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Interval != 1800 || resp.MinInterval != 2400 {
		t.Fatalf("interval=%d minInterval=%d, want 1800/2400", resp.Interval, resp.MinInterval)
	}
}

func TestParseRejectsDeeplyNested(t *testing.T) {
	// 构造超过 maxBencodeDepth 的嵌套列表，应返回错误而非耗尽栈
	depth := maxBencodeDepth + 50
	nested := strings.Repeat("l", depth) + strings.Repeat("e", depth)
	if _, err := ParseResponse([]byte(nested)); err == nil {
		t.Fatal("expected error for deeply nested bencode, got nil")
	}
}

func TestParseRejectsOverflowStringLength(t *testing.T) {
	// 字符串长度前缀为 MaxInt64：start+n 会回绕为负数，必须返回错误而非切片越界 panic
	overflow := "9223372036854775807:abc"
	if _, err := ParseResponse([]byte(overflow)); err == nil {
		t.Fatal("expected error for overflowing string length, got nil")
	}

	// 负数长度（digit 检查已拦截，仍应报错而非 panic）
	if _, err := ParseResponse([]byte("-1:abc")); err == nil {
		t.Fatal("expected error for negative string length, got nil")
	}
}
