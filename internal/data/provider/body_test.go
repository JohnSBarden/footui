package provider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReadBodyNeutralisesControlEscapes(t *testing.T) {
	in := `{"name":"Evil\u001b]0;pwned\u0007 FC","c1":"a\u009bb","ok":"Müller\n","lit":"\\u001b"}`
	body, err := ReadBody(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("result is not valid JSON: %v\n%s", err, body)
	}
	want := map[string]string{
		"name": "Evil\uFFFD]0;pwned\uFFFD FC",
		"c1":   "a\uFFFDb",
		"ok":   "Müller\n", // short escapes and non-ASCII are left alone
		"lit":  "\\ufffd",  // an escaped backslash stays valid JSON
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s = %q, want %q", k, got[k], w)
		}
	}
}

func TestReadBodyRejectsOversizedBody(t *testing.T) {
	big := strings.NewReader(strings.Repeat("x", MaxBody+1))
	if _, err := ReadBody(big); err == nil {
		t.Fatal("want an error for a body over MaxBody")
	}
}
