package main

import (
	"bufio"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/eventbridge"
	"github.com/stretchr/testify/require"
	"github.com/willabides/piper/internal/testutil"
)

func Test_eventBuilder_buildEvent(t *testing.T) {
	cli := &cliOptions{
		Region:     "us-east-1",
		DetailType: "jp:type",
		EventBus:   "a-bus",
		Resource:   []string{"a resource", "jp:type"},
		Source:     "the-cloud",
		Time:       "1608309835000",
	}

	builder := cli.eventBuilder()
	data := `{"id": "foo", "time": "1608309835000", "type": "foo"}`
	tm := time.Unix(1608309835, 0).UTC()
	want := &eventbridge.PutEventsRequestEntry{
		Detail:       &data,
		DetailType:   aws.String("foo"),
		EventBusName: aws.String("a-bus"),
		Resources:    []*string{aws.String("a resource"), aws.String("foo")},
		Source:       aws.String("the-cloud"),
		Time:         &tm,
	}
	got, err := builder.buildEvent([]byte(data))
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func Test_run(t *testing.T) {
	ctx := context.Background()

	lines := []string{
		`{"id": "foo", "time": "1608309835000", "type": "foo"}`,
		``,
		` `,
		`{"id": "bar", "time": "1608309835000", "type": "bar"}`,
		`{"id": "baz", "time": "1608309835000", "type": "baz"}`,
		`{"id": "qux", "time": 1608309835000, "type": "qux"}`,
	}

	scanner := bufio.NewScanner(strings.NewReader(strings.Join(lines, "\n")))

	q := testutil.NewMockPutter(t)
	q.Expect([][]byte{[]byte(lines[0]), []byte(lines[3]), []byte(lines[4])})
	q.Expect([][]byte{[]byte(lines[5])})

	cli := &cliOptions{
		Region:        "us-east-1",
		DetailType:    "jp:type",
		EventBus:      "a-bus",
		Resource:      []string{"a resource", "jp:type"},
		Source:        "the-cloud",
		Time:          "1608309835000",
		BatchSize:     3,
		FlushInterval: 0,
		_putter:       q,
	}

	err := run(ctx, cli, scanner)
	require.NoError(t, err)
}
