package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/alecthomas/kong"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eventbridge"
	"github.com/willabides/piper/internal"
)

var kongVars = kong.Vars{
	"batch_size_help":     `Number of events to send in a batch.`,
	"flush_interval_help": `Time in milliseconds to wait before sending a partial batch. Set to 0 to never send a partial batch.`,
	"region_help":         `The aws region to publish events to.`,
	"detail_type_help":    `Value for the DetailType field. JMESPath expressions allowed with "jp:" prefix.`,
	"event_bus_help":      `Value for the "EventBusName" field.`,
	"resource_help":       `An element for the list in the "Resources" array. JMESPath expressions allowed with "jp:" prefix.`,
	"source_help":         `Value for the "Source" field. JMESPath expressions allowed with "jp:" prefix.`,
	"time_help":           `Value for the "Time" field converted from epoch milliseconds. JMESPath expressions allowed with "jp:" prefix.`,
}

type cliOptions struct {
	Region        string   `kong:"default=us-east-1,help=${region_help}"`
	DetailType    string   `kong:"required,name=type,short=t,help=${detail_type_help}"`
	EventBus      string   `kong:"short=b,help=${event_bus_help}"`
	Resource      []string `kong:"short=r,help=${resource_help}"`
	Source        string   `kong:"required,short=s,help=${source_help}"`
	Time          string   `kong:"name=timestamp,short=T,help=${time_help}"`
	BatchSize     int      `kong:"default=10,help=${batch_size_help}"`
	FlushInterval int      `kong:"default=2000,help=${flush_interval_help}"`

	_putter internal.EventSink
}

const helpDescription = `eventbridge-pipe posts events to AWS EventBridge.

example:
  $ AWS_ACCESS_KEY='AKIA****************'
  $ AWS_SECRET_KEY='shhh_this_is_a_secret'
  $ data="$(cat <<"EOF"
      {"action": "obj.add", "@timestamp": 1604953432032, "el_name": "foo", "doc_id": "asdf"}
      {"action": "obj.rem", "@timestamp": 1604953732032, "el_name": "bar", "doc_id": "fdsa"}
    EOF
    )"
  $ echo "$data" | \
    eventbridge-pipe -s 'test-source' -t 'jp:action' -b 'my-bus' -T 'jp:"@timestamp"' \
    -r 'jp:"el_name"' 

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

func run(ctx context.Context, cli *cliOptions, scanner *bufio.Scanner) error {
	if cli.BatchSize > 10 {
		return fmt.Errorf("batch size exceeds aws maximum")
	}
	p, err := cli.eventSink()
	if err != nil {
		return err
	}

	publisher := &internal.Publisher{
		MaxQueueSize:  cli.BatchSize,
		Sink:          p,
		FlushInterval: time.Duration(cli.FlushInterval) * time.Millisecond,
	}

	return publisher.Run(ctx, scanner)
}

func (c *cliOptions) eventBuilder() *eventBuilder {
	defs := map[string]string{
		"DetailType": c.DetailType,
		"Source":     c.Source,
		"Time":       c.Time,
	}
	for i, s := range c.Resource {
		defs[resourceName(i)] = s
	}
	return &eventBuilder{
		eventBus:    c.EventBus,
		resources:   c.Resource,
		parseHelper: internal.NewLineParseHelper(defs),
	}
}

func (c *cliOptions) eventSink() (internal.EventSink, error) {
	if c._putter != nil {
		return c._putter, nil
	}
	config := aws.NewConfig()
	config = config.WithRegion(c.Region)
	config = config.WithCredentials(
		credentials.NewEnvCredentials(),
	)
	sess, err := session.NewSession(config)
	if err != nil {
		return nil, err
	}

	return &eventSink{
		bridge:  eventbridge.New(sess),
		builder: c.eventBuilder(),
	}, nil
}

type eventBuilder struct {
	eventBus    string
	parseHelper *internal.LineParseHelper
	resources   []string
}

func (c *eventBuilder) buildEvent(data []byte) (*eventbridge.PutEventsRequestEntry, error) {
	dataStr := string(data)
	ev := eventbridge.PutEventsRequestEntry{
		Detail: &dataStr,
	}
	if c.eventBus != "" {
		ev.EventBusName = &c.eventBus
	}

	ld := internal.NewLineData(data)

	parseHelper := c.parseHelper

	detailType, err := parseHelper.GetVal("DetailType", ld)
	if err != nil {
		return nil, err
	}
	if detailType != "" {
		ev.DetailType = &detailType
	}

	source, err := parseHelper.GetVal("Source", ld)
	if err != nil {
		return nil, err
	}
	if source != "" {
		ev.Source = &source
	}

	eventTime, err := c.eventTime(ld)
	if err != nil {
		return nil, err
	}
	if eventTime != nil {
		ev.Time = eventTime
	}

	resources, err := c.buildResources(ld)
	if err != nil {
		return nil, err
	}
	if len(resources) != 0 {
		ev.Resources = resources
	}

	return &ev, nil
}

func resourceName(i int) string {
	return fmt.Sprintf("resource_%d", i)
}

func (c *eventBuilder) buildResources(ld *internal.LineData) ([]*string, error) {
	if len(c.resources) == 0 {
		return nil, nil
	}
	result := make([]*string, len(c.resources))
	for i := range c.resources {
		val, err := c.parseHelper.GetVal(resourceName(i), ld)
		if err != nil {
			return nil, err
		}
		result[i] = &val
	}
	return result, nil
}

func (c *eventBuilder) eventTime(ld *internal.LineData) (*time.Time, error) {
	strVal, err := c.parseHelper.GetVal("Time", ld)
	if err != nil {
		return nil, err
	}
	switch strVal {
	case "":
		return nil, nil
	case "now":
		now := time.Now().UTC()
		return &now, nil
	}
	iVal, err := strconv.ParseInt(strVal, 10, 64)
	if err != nil {
		return nil, err
	}
	secs := iVal / 1000
	ms := iVal % 1000
	ns := ms * int64(time.Millisecond)
	tm := time.Unix(secs, ns).UTC()
	return &tm, nil
}

type eventSink struct {
	bridge  *eventbridge.EventBridge
	builder *eventBuilder
}

func (e *eventSink) FlushEvents(ctx context.Context, cache [][]byte) error {
	var err error
	events := make([]*eventbridge.PutEventsRequestEntry, len(cache))
	for i, data := range cache {
		events[i], err = e.builder.buildEvent(data)
		if err != nil {
			return err
		}
	}
	var resp *eventbridge.PutEventsOutput
	resp, err = e.bridge.PutEventsWithContext(ctx, &eventbridge.PutEventsInput{
		Entries: events,
	})
	if err != nil {
		return err
	}
	if resp.FailedEntryCount != nil {
		if *resp.FailedEntryCount != 0 {
			return fmt.Errorf("one or more failed entries: %s", resp.String())
		}
	}
	return nil
}
