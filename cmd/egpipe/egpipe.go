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
	"github.com/google/uuid"
	"github.com/willabides/piper/internal"
)

var kongVars = kong.Vars{
	"header_help":         `Header to sent with the request in the same format as curl. e.g. '-H "aeg-sas-key: $EVKEY"'`,
	"data_version_help":   `Value for the "dataVersion" field. JMESPath expressions allowed with "jp:" prefix.`,
	"batch_size_help":     `Number of events to send in a batch.`,
	"flush_interval_help": `Time in milliseconds to wait before sending a partial batch. Set to 0 to never send a partial batch.`,
	"topic_endpoint_help": `Endpoint for posting events`,

	"id_help":      `Value for the "id" field. If unset, a uuid will be generated for each event. JMESPath expressions allowed with "jp:" prefix.`,
	"subject_help": `Value for the "subject" field. JMESPath expressions allowed with "jp:" prefix.`,
	"type_help":    `Value for the "eventType" field. JMESPath expressions allowed with "jp:" prefix.`,
	"time_help": `Value for the "eventTime" field converted from epoch milliseconds. If unset, the current 
system time will be used.JMESPath expressions allowed with "jp:" prefix.`,
}

type cliOptions struct {
	TopicEndpoint string   `kong:"arg,required,help=${topic_endpoint_help}"`
	ID            string   `kong:"short=i,help=${id_help}"`
	Subject       string   `kong:"required,short=s,help=${subject_help}"`
	EventType     string   `kong:"required,short=t,name='type',help=${type_help}"`
	EventTime     string   `kong:"name='timestamp',short=T,default='now',help=${time_help}"`
	Header        []string `kong:"short=H,help=${header_help}"`
	DataVersion   string   `kong:"default=1.0,help=${data_version_help}"`
	BatchSize     int      `kong:"default=10,help=${batch_size_help}"`
	FlushInterval int      `kong:"default=2000,help=${flush_interval_help}"`
}

const helpDescription = `egpipe posts events to Azure Event Grid.

example:
  $ topic_endpoint='https://mytopicendpoint.westus2-1.eventgrid.azure.net'
  $ topic_key='shhh_secret_topic_key'
  $ data="$(cat <<"EOF"
      {"action": "obj.add", "@timestamp": 1604953432032, "el_name": "foo", "doc_id": "asdf"}
      {"action": "obj.rem", "@timestamp": 1604953732032, "el_name": "bar", "doc_id": "fdsa"}
    EOF
    )"
  $ echo "$data" | \
    egpipe "$topic_endpoint" \
    -H "aeg-sas-key: $topic_key" \
    -T 'jp:"@timestamp"' \
    -t 'audit-log' \
    -s 'jp:action' \
    -i 'jp:doc_id'

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
	th := c.TopicEndpoint
	if !strings.Contains(th, `://`) {
		th = "https://" + th
	}
	pURL, err := url.Parse(th)
	if err != nil {
		return "", err
	}

	if pURL.Path == "" {
		pURL.Path = `api/events`
	}
	query := pURL.Query()
	if query.Get("api-version") == "" {
		query.Set("api-version", "2018-01-01")
	}
	pURL.RawQuery = query.Encode()

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
				"subject":     c.Subject,
				"id":          c.ID,
				"eventType":   c.EventType,
				"eventTime":   c.EventTime,
				"dataVersion": c.DataVersion,
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

	ev.ID, err = parseHelper.GetVal("id", ld)
	if err != nil {
		return nil, err
	}
	if ev.ID == "" {
		ev.ID = uuid.New().String()
	}

	ev.Subject, err = parseHelper.GetVal("subject", ld)
	if err != nil {
		return nil, err
	}

	ev.DataVersion, err = parseHelper.GetVal("dataVersion", ld)
	if err != nil {
		return nil, err
	}

	ev.EventTime, err = c.eventTime(ld)
	if err != nil {
		return nil, err
	}

	ev.EventType, err = parseHelper.GetVal("eventType", ld)
	if err != nil {
		return nil, err
	}

	ev.Data = json.RawMessage(data)

	return ev, nil
}

func (c *eventBuilder) eventTime(ld *internal.LineData) (string, error) {
	strVal, err := c.parseHelper.GetVal("eventTime", ld)
	if err != nil {
		return "", err
	}
	if strVal == "now" {
		return time.Now().UTC().Format(time.RFC3339Nano), nil
	}
	iVal, err := strconv.ParseInt(strVal, 10, 64)
	if err != nil {
		return "", err
	}
	secs := iVal / 1000
	ms := iVal % 1000
	ns := ms * int64(time.Millisecond)
	return time.Unix(secs, ns).UTC().Format(time.RFC3339Nano), nil
}

type eventSink struct {
	reqHeader  http.Header
	httpClient *http.Client
	endpoint   string
	builder    *eventBuilder
}

func (e eventSink) FlushEvents(ctx context.Context, cache [][]byte) error {
	var err error
	events := make([]*event, len(cache))
	for i, data := range cache {
		events[i], err = e.builder.buildEvent(data)
		if err != nil {
			return err
		}
	}
	var buf bytes.Buffer
	err = json.NewEncoder(&buf).Encode(events)
	if err != nil {
		return err
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

// event properties of an event published to an event Grid topic using the EventGrid Schema.
type event struct {
	// ID - An unique identifier for the event.
	ID string `json:"id,omitempty"`
	// Topic - The resource path of the event source.
	Topic string `json:"topic,omitempty"`
	// Subject - A resource path relative to the topic path.
	Subject string `json:"subject,omitempty"`
	// Data - event data specific to the event type.
	Data interface{} `json:"data,omitempty"`
	// EventType - The type of the event that occurred.
	EventType string `json:"eventType,omitempty"`
	// EventTime - The time (in UTC) the event was generated.
	EventTime string `json:"eventTime,omitempty"`
	// DataVersion - The schema version of the data object.
	DataVersion string `json:"dataVersion,omitempty"`
}
