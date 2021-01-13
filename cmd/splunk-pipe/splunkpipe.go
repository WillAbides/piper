package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/willabides/piper/internal"
)

var kongVars = kong.Vars{
	"header_help":         `Header to sent with the request in the same format as curl. e.g. '-H "Authorization: Splunk $HEC_KEY"'`,
	"batch_size_help":     `Number of events to send in a batch.`,
	"flush_interval_help": `Time in milliseconds to wait before sending a partial batch. Set to 0 to never send a partial batch.`,
	"endpoint_help":       `Endpoint for posting events`,

	"index_help":      `Value for the "index" field. JMESPath expressions allowed with "jp:" prefix.`,
	"host_help":       `Value for the "host" field. JMESPath expressions allowed with "jp:" prefix.`,
	"sourcetype_help": `Value for the "sourcetype" field. JMESPath expressions allowed with "jp:" prefix.`,
	"source_help":     `Value for the "source" field. JMESPath expressions allowed with "jp:" prefix.`,
	"time_help":       `Value for the "eventTime" field converted from epoch milliseconds. JMESPath expressions allowed with "jp:" prefix.`,
}

type cliOptions struct {
	Endpoint      string   `kong:"arg,required,help=${endpoint_help}"`
	Sourcetype    string   `kong:"short=t,name='sourcetype',help=${sourcetype_help}"`
	Source        string   `kong:"short=s,name='source',help=${source_help}"`
	Time          string   `kong:"name='timestamp',short=T,help=${time_help}"`
	Header        []string `kong:"short=H,help=${header_help}"`
	Host          string   `kong:"short=h,help=${host_help}"`
	Index         string   `kong:"help=${index_help}"`
	BatchSize     int      `kong:"default=10,help=${batch_size_help}"`
	FlushInterval int      `kong:"default=2000,help=${flush_interval_help}"`
}

const helpDescription = `splunk-pipe posts events to splunk.

example:
  $ splunk_endpoint="http://localhost:8080"
  $ splunk_hec_token="shhh_secret_token"
  $ data="$(cat <<"EOF"
      {"action": "obj.add", "@timestamp": 1604953432032, "el_name": "foo", "doc_id": "asdf"}
      {"action": "obj.rem", "@timestamp": 1604953732032, "el_name": "bar", "doc_id": "fdsa"}
    EOF
    )"
  $ echo "$data" | \
    splunk-pipe "$splunk_endpoint" \
    -H "Authorization: Splunk $splunk_hec_token" \
    -T 'jp:"@timestamp"'

Learn about JMESPath syntax at https://jmespath.org
`

func main() {
	var cli cliOptions
	k := kong.Parse(&cli, kongVars, kong.Description(helpDescription))
	scanner := bufio.NewScanner(os.Stdin)
	ctx := context.Background()
	err := run(ctx, &cli, scanner)
	k.FatalIfErrorf(err)
}

func (c *cliOptions) url() (string, error) {
	endpoint := c.Endpoint
	if !strings.Contains(endpoint, `://`) {
		endpoint = "https://" + endpoint
	}
	pURL, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}

	if pURL.Path == "" {
		pURL.Path = `services/collector/event`
	}

	return pURL.String(), nil
}

func run(ctx context.Context, cli *cliOptions, scanner *bufio.Scanner) error {
	sink, err := cli.eventSink()
	if err != nil {
		return err
	}
	publisher := &internal.Publisher{
		MaxQueueSize:  cli.BatchSize,
		Sink:          sink,
		FlushInterval: time.Duration(cli.FlushInterval) * time.Millisecond,
	}

	return publisher.Run(ctx, scanner)
}

func (c *cliOptions) eventSink() (internal.EventSink, error) {
	header := http.Header{}

	for _, hdr := range c.Header {
		parts := strings.SplitN(hdr, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid header %q", hdr)
		}
		header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
	}

	thURL, err := c.url()
	if err != nil {
		return nil, err
	}

	return &eventSink{
		reqHeader: header,
		endpoint:  thURL,
		builder: &eventBuilder{
			parseHelper: internal.NewLineParseHelper(map[string]string{
				"source":     c.Source,
				"sourcetype": c.Sourcetype,
				"host":       c.Host,
				"index":      c.Index,
				"time":       c.Time,
			}),
		},
	}, nil
}

type eventBuilder struct {
	parseHelper *internal.LineParseHelper
}

func (c *eventBuilder) buildEvent(data []byte) (*event, error) {
	ev := new(event)

	ld := internal.NewLineData(data)

	var err error
	parseHelper := c.parseHelper

	ev.Index, err = parseHelper.GetVal("index", ld)
	if err != nil {
		return nil, err
	}

	ev.Host, err = parseHelper.GetVal("host", ld)
	if err != nil {
		return nil, err
	}

	ev.Sourcetype, err = parseHelper.GetVal("sourcetype", ld)
	if err != nil {
		return nil, err
	}

	ev.Source, err = parseHelper.GetVal("source", ld)
	if err != nil {
		return nil, err
	}

	ev.Time, err = c.eventTime(ld)
	if err != nil {
		return nil, err
	}

	ev.Event = json.RawMessage(data)

	return ev, nil
}

func (c *eventBuilder) eventTime(ld *internal.LineData) (float64, error) {
	parseHelper := c.parseHelper
	strVal, err := parseHelper.GetVal("time", ld)
	if err != nil {
		return 0, err
	}
	if strVal == "" {
		return 0, nil
	}
	iVal, err := strconv.ParseInt(strVal, 10, 64)
	if err != nil {
		return 0, err
	}
	secs := float64(iVal) / 1000
	return secs, nil
}

type eventSink struct {
	reqHeader  http.Header
	httpClient *http.Client
	endpoint   string
	builder    *eventBuilder
}

func (e eventSink) FlushEvents(ctx context.Context, cache [][]byte) error {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	var err error
	for _, data := range cache {
		var ev *event
		ev, err = e.builder.buildEvent(data)
		if err != nil {
			return err
		}
		err = encoder.Encode(ev)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, &buf)
	if err != nil {
		return err
	}
	req.Header = e.reqHeader
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	httpClient := e.httpClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("not OK, statusCode: %d", resp.StatusCode)
	}
	return nil
}

type event struct {
	Time       float64     `json:"time,omitempty"`
	Host       string      `json:"host,omitempty"`
	Source     string      `json:"source,omitempty"`
	Sourcetype string      `json:"sourcetype,omitempty"`
	Index      string      `json:"index,omitempty"`
	Event      interface{} `json:"event,omitempty"`
}
