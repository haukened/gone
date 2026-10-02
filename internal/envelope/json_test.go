package envelope

import (
	"encoding/json"
	"testing"
)

func TestAppendJSONString(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", `""`},
		{"abc", `"abc"`},
		{`"\`, `"\"\\"`},
		{"\b\t\n\f\r", `"\b\t\n\f\r"`},
		{"\x00\x01\x1f", `"\u0000\u0001\u001f"`},
		{"\x7f", "\"\x7f\""},
		{"<>&'/", `"<>&'/"`},
		{"é\u2028😀", "\"é\u2028😀\""},
	}
	for _, c := range cases {
		got := string(appendJSONString(nil, c.in))
		if got != c.want {
			t.Errorf("appendJSONString(%q) = %s, want %s", c.in, got, c.want)
		}
		var back string
		if err := json.Unmarshal([]byte(got), &back); err != nil || back != c.in {
			t.Errorf("round trip %q: %q %v", c.in, back, err)
		}
	}
}

func TestCanonicalHeader(t *testing.T) {
	cases := []struct {
		msg   int
		files []fileHeader
		want  string
	}{
		{0, nil, `{"v":2,"msg":0,"files":[]}`},
		{5, []fileHeader{{"a.txt", "text/plain", 3}}, `{"v":2,"msg":5,"files":[{"name":"a.txt","type":"text/plain","size":3}]}`},
		{0, []fileHeader{{"a", DefaultType, 0}, {`q"`, DefaultType, 9007199254740991}},
			`{"v":2,"msg":0,"files":[{"name":"a","type":"application/octet-stream","size":0},{"name":"q\"","type":"application/octet-stream","size":9007199254740991}]}`},
	}
	for _, c := range cases {
		if got := string(canonicalHeader(c.msg, c.files)); got != c.want {
			t.Errorf("canonicalHeader = %s, want %s", got, c.want)
		}
	}
}
