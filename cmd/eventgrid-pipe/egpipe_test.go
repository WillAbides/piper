package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testServer struct {
	t      testing.TB
	want   []interface{}
	server *httptest.Server
}

func newTestServer(t testing.TB, want []interface{}) *testServer {
	t.Helper()
	ts := &testServer{
		t:    t,
		want: want,
	}
	ts.server = httptest.NewServer(ts)
	t.Cleanup(func() {
		ts.server.Close()
		assert.Empty(t, ts.want)
	})
	return ts
}

func (s *testServer) expect(expect ...map[string]interface{}) {
	s.want = append(s.want, expect)
}

func (s *testServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	t := s.t
	t.Helper()
	if len(s.want) == 0 {
		t.Error("unexpected request")
		return
	}
	want, err := json.Marshal(s.want[0])
	if !assert.NoError(t, err) {
		return
	}
	s.want = s.want[1:]
	body, err := ioutil.ReadAll(req.Body)
	if !assert.NoError(t, err) {
		return
	}
	assert.JSONEq(t, string(want), string(body))
}

func Test_cliOptions_url(t *testing.T) {
	for _, td := range []struct {
		input string
		want  string
	}{
		{
			input: `dummy.westus2-1.eventgrid.azure.net`,
			want:  `https://dummy.westus2-1.eventgrid.azure.net/api/events?api-version=2018-01-01`,
		},
		{
			input: `https://dummy.westus2-1.eventgrid.azure.net`,
			want:  `https://dummy.westus2-1.eventgrid.azure.net/api/events?api-version=2018-01-01`,
		},
		{
			input: `https://dummy.westus2-1.eventgrid.azure.net/foo/bar`,
			want:  `https://dummy.westus2-1.eventgrid.azure.net/foo/bar?api-version=2018-01-01`,
		},
		{
			input: `http://127.0.0.1:1234`,
			want:  `http://127.0.0.1:1234/api/events?api-version=2018-01-01`,
		},
	} {
		t.Run(fmt.Sprintf("%q", td.input), func(t *testing.T) {
			c := &cliOptions{
				TopicEndpoint: td.input,
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
{"id": "foo", "time": "1608309835000", "type": "foo"}

   
{"id": "bar", "time": "1608309835000", "type": "bar"}
{"id": "baz", "time": "1608309835000", "type": "baz"}
{"id": "qux", "time": 1608309835000, "type": "qux"}
`
	scanner := bufio.NewScanner(strings.NewReader(lines))
	ts := newTestServer(t, []interface{}{})

	ts.expect(
		map[string]interface{}{
			"id":          "foo",
			"eventTime":   "2020-12-18T16:43:55Z",
			"dataVersion": "1.0",
			"subject":     "my subject",
			"eventType":   "foo",
			"data": map[string]interface{}{
				"id": "foo", "time": "1608309835000", "type": "foo",
			},
		},
		map[string]interface{}{
			"id":          "bar",
			"eventTime":   "2020-12-18T16:43:55Z",
			"dataVersion": "1.0",
			"subject":     "my subject",
			"eventType":   "bar",
			"data": map[string]interface{}{
				"id": "bar", "time": "1608309835000", "type": "bar",
			},
		},
		map[string]interface{}{
			"id":          "baz",
			"eventTime":   "2020-12-18T16:43:55Z",
			"dataVersion": "1.0",
			"subject":     "my subject",
			"eventType":   "baz",
			"data": map[string]interface{}{
				"id": "baz", "time": "1608309835000", "type": "baz",
			},
		},
	)
	ts.expect(map[string]interface{}{
		"id":          "qux",
		"eventTime":   "2020-12-18T16:43:55Z",
		"dataVersion": "1.0",
		"subject":     "my subject",
		"eventType":   "qux",
		"data": map[string]interface{}{
			"id": "qux", "time": 1608309835000, "type": "qux",
		},
	})
	cli := &cliOptions{
		TopicEndpoint: ts.server.URL,
		Header:        []string{"foo: bar"},
		ID:            "jp:id",
		Subject:       "my subject",
		EventType:     "jp:type",
		EventTime:     "jp:time",
		DataVersion:   "1.0",
		BatchSize:     3,
	}
	err := run(ctx, cli, scanner)
	require.NoError(t, err)
}
