# eventgrid-pipe

## Install

```
go get github.com/willabides/piper/cmd/eventgrid-pipe
```

## Usage

```
Usage: eventgrid-pipe --subject=STRING --type=STRING <topic-endpoint>

eventgrid-pipe posts events to Azure Event Grid.

example:

    $ topic_endpoint='https://mytopicendpoint.westus2-1.eventgrid.azure.net'
    $ topic_key='shhh_secret_topic_key'
    $ data="$(cat <<"EOF"
        {"action": "obj.add", "@timestamp": 1604953432032, "el_name": "foo", "doc_id": "asdf"}
        {"action": "obj.rem", "@timestamp": 1604953732032, "el_name": "bar", "doc_id": "fdsa"}
      EOF
      )"
    $ echo "$data" | \
      eventgrid-pipe "$topic_endpoint" \
      -H "aeg-sas-key: $topic_key" \
      -T 'jp:"@timestamp"' \
      -t 'audit-log' \
      -s 'jp:action' \
      -i 'jp:doc_id'

Learn about JMESPath syntax at https://jmespath.org

Arguments:
  <topic-endpoint>    Endpoint for posting events

Flags:
  -h, --help                   Show context-sensitive help.
  -i, --id=STRING              Value for the "id" field. If unset, a uuid will
                               be generated for each event. JMESPath expressions
                               allowed with "jp:" prefix.
  -s, --subject=STRING         Value for the "subject" field. JMESPath
                               expressions allowed with "jp:" prefix.
  -t, --type=STRING            Value for the "eventType" field. JMESPath
                               expressions allowed with "jp:" prefix.
  -T, --timestamp="now"        Value for the "eventTime" field converted from
                               epoch milliseconds. If unset, the current system
                               time will be used.JMESPath expressions allowed
                               with "jp:" prefix.
  -H, --header=HEADER,...      Header to sent with the request in the same
                               format as curl. e.g. '-H "aeg-sas-key: $EVKEY"'
      --data-version="1.0"     Value for the "dataVersion" field. JMESPath
                               expressions allowed with "jp:" prefix.
      --batch-size=10          Number of events to send in a batch.
      --flush-interval=2000    Time in milliseconds to wait before sending a
                               partial batch. Set to 0 to never send a partial
                               batch.
```

