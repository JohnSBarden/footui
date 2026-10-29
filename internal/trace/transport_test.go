package trace

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestTransportLogsEachRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()
	client := &http.Client{Transport: Transport(nil)}

	// Off, nothing is written and the request still goes through.
	resp, err := client.Get(srv.URL + "/quiet")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	var b strings.Builder
	defer startWriter(&b, time.Now)()
	resp, err = client.Get(srv.URL + "/teams")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	want := regexp.MustCompile(`^\S+ \S+ DBUG GET http://\S+/teams → 418 \(\S+\)\n$`)
	if !want.MatchString(b.String()) {
		t.Errorf("log %q", b.String())
	}
}
