## splunk-pipe

### Install

#### go get

```
go get github.com/willabides/piper/cmd/splunk-pipe
```

#### bindown

Add a [bindown](https://github.com/willabides/bindown) dependency:

```
$ bindown template-source add piper https://github.com/WillAbides/piper/releases/latest/download/bindown-templates.yml
$ bindown dependency add splunk-pipe piper#splunk-pipe
```

### Usage

```
Usage: splunk-pipe <endpoint>

splunk-pipe posts events to splunk.

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

Arguments:
  <endpoint>    Endpoint for posting events

Flags:
  -h, --help                   Show context-sensitive help.
  -t, --sourcetype=STRING      Value for the "sourcetype" field. JMESPath
                               expressions allowed with "jp:" prefix.
  -s, --source=STRING          Value for the "source" field. JMESPath
                               expressions allowed with "jp:" prefix.
  -T, --timestamp=STRING       Value for the "eventTime" field converted from
                               epoch milliseconds. JMESPath expressions allowed
                               with "jp:" prefix.
  -H, --header=HEADER,...      Header to sent with the request in the same
                               format as curl. e.g. '-H "Authorization: Splunk
                               $HEC_KEY"'
  -h, --host=STRING            Value for the "host" field. JMESPath expressions
                               allowed with "jp:" prefix.
      --index=STRING           Value for the "index" field. JMESPath expressions
                               allowed with "jp:" prefix.
      --batch-size=10          Number of events to send in a batch.
      --flush-interval=2000    Time in milliseconds to wait before sending a
                               partial batch. Set to 0 to never send a partial
                               batch.
```

