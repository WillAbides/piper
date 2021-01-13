package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testServer struct {
	t      testing.TB
	want   [][]interface{}
	server *httptest.Server
}

func newTestServer(t testing.TB) *testServer {
	t.Helper()
	ts := &testServer{
		t: t,
	}
	ts.server = httptest.NewServer(ts)
	t.Cleanup(func() {
		ts.server.Close()
		assert.Empty(t, ts.want)
	})
	return ts
}

func (s *testServer) expect(expect ...map[string]interface{}) {
	ex := make([]interface{}, len(expect))
	for i, x := range expect {
		ex[i] = x
	}

	s.want = append(s.want, ex)
}

func (s *testServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	t := s.t
	t.Helper()
	if len(s.want) == 0 {
		t.Error("unexpected request")
		return
	}
	want := s.want[0]
	s.want = s.want[1:]
	decoder := json.NewDecoder(req.Body)
	var got []string
	var err error
	for decoder.More() {
		var msg json.RawMessage
		err = decoder.Decode(&msg)
		if !assert.NoError(t, err) {
			return
		}
		got = append(got, string(msg))
	}
	if !assert.Equal(t, len(want), len(got)) {
		return
	}
	if got == nil {
		t.Error("got is nil")
		return
	}

	for i, w := range want {
		b, err := json.Marshal(w)
		if !assert.NoError(t, err) {
			return
		}
		if !assert.JSONEq(t, string(b), got[i]) {
			return
		}
	}
}

func Test_cliOptions_url(t *testing.T) {
	for _, td := range []struct {
		input string
		want  string
	}{
		{
			input: `foo.bar`,
			want:  `https://foo.bar/services/collector/event`,
		},
		{
			input: `https://foo.bar`,
			want:  `https://foo.bar/services/collector/event`,
		},
		{
			input: `https://foo.bar/foo/bar`,
			want:  `https://foo.bar/foo/bar`,
		},
		{
			input: `http://127.0.0.1:1234`,
			want:  `http://127.0.0.1:1234/services/collector/event`,
		},
	} {
		t.Run(fmt.Sprintf("%q", td.input), func(t *testing.T) {
			c := &cliOptions{
				Endpoint: td.input,
			}
			got, err := c.url()
			require.NoError(t, err)
			require.Equal(t, td.want, got)
		})
	}
}

func Test_run(t *testing.T) {
	ctx := context.Background()

	lines := `
{"id": "foo", "@timestamp": "1608309835123", "type": "foo"}

   
{"id": "bar", "@timestamp": "1608309835123", "type": "bar"}
{"id": "baz", "@timestamp": "1608309835123", "type": "baz"}
{"id": "qux", "@timestamp": 1608309835123, "type": "qux"}
`
	scanner := bufio.NewScanner(strings.NewReader(lines))
	ts := newTestServer(t)

	ts.expect(
		map[string]interface{}{
			"index":      "an index",
			"host":       "a host",
			"time":       1608309835.123,
			"source":     "the source",
			"sourcetype": "foo",
			"event": map[string]interface{}{
				"id": "foo", "@timestamp": "1608309835123", "type": "foo",
			},
		},
		map[string]interface{}{
			"index":      "an index",
			"host":       "a host",
			"time":       1608309835.123,
			"source":     "the source",
			"sourcetype": "bar",
			"event": map[string]interface{}{
				"id": "bar", "@timestamp": "1608309835123", "type": "bar",
			},
		},
		map[string]interface{}{
			"index":      "an index",
			"host":       "a host",
			"time":       1608309835.123,
			"source":     "the source",
			"sourcetype": "baz",
			"event": map[string]interface{}{
				"id": "baz", "@timestamp": "1608309835123", "type": "baz",
			},
		},
	)
	ts.expect(map[string]interface{}{
		"index":      "an index",
		"host":       "a host",
		"time":       1608309835.123,
		"source":     "the source",
		"sourcetype": "qux",
		"event": map[string]interface{}{
			"id": "qux", "@timestamp": 1608309835123, "type": "qux",
		},
	})
	cli := &cliOptions{
		Endpoint:   ts.server.URL,
		Header:     []string{"foo: bar"},
		Time:       `jp:"@timestamp"`,
		Source:     "the source",
		Sourcetype: `jp:type`,
		Host:       `a host`,
		Index:      `an index`,
		BatchSize:  3,
	}
	err := run(ctx, cli, scanner)
	require.NoError(t, err)
}
