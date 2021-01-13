package internal

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jmespath/go-jmespath"
)

// NewLineParseHelper returns a LineParseHelper
func NewLineParseHelper(optDefs map[string]string) *LineParseHelper {
	return &LineParseHelper{
		optDefs: optDefs,
	}
}

// LineParseHelper helper for parsing line data
type LineParseHelper struct {
	optDefs   map[string]string
	jmespaths map[string]*jmespath.JMESPath
}

func (c *LineParseHelper) jmespath(name, val string) (*jmespath.JMESPath, error) {
	if c.jmespaths == nil {
		c.jmespaths = map[string]*jmespath.JMESPath{}
	}
	var err error
	if !strings.HasPrefix(val, jmespathPrefix) {
		return nil, nil
	}
	if c.jmespaths == nil {
		c.jmespaths = map[string]*jmespath.JMESPath{}
	}
	if c.jmespaths[name] == nil {
		c.jmespaths[name], err = jmespath.Compile(strings.TrimPrefix(val, jmespathPrefix))
		if err != nil {
			return nil, err
		}
	}
	return c.jmespaths[name], nil
}

// GetVal returns a val
func (c *LineParseHelper) GetVal(valName string, data *LineData) (string, error) {
	optDef := c.optDefs[valName]
	if optDef == "" {
		optDef = valName
	}
	if strings.HasPrefix(optDef, jmespathPrefix) {
		jp, err := c.jmespath(valName, optDef)
		if err != nil {
			return "", err
		}
		jd, err := data.unmarshalled()
		if err != nil {
			return "", err
		}
		return jmespathString(jp, jd)
	}
	return optDef, nil
}

func jmespathString(jp *jmespath.JMESPath, data interface{}) (string, error) {
	got, err := jp.Search(data)
	if err != nil {
		return "", err
	}
	switch val := got.(type) {
	case string:
		return val, nil
	case float64:
		return fmt.Sprintf("%.0f", val), nil
	default:
		return fmt.Sprintf("%v", val), nil
	}
}

const jmespathPrefix = "jp:"

// NewLineData returns a new LineData
func NewLineData(data []byte) *LineData {
	return &LineData{
		data: data,
	}
}

// LineData caches the unmarshalled json so we only need to unmarshal once
type LineData struct {
	data  []byte
	iface interface{}
}

func (l LineData) unmarshalled() (interface{}, error) {
	if l.iface == nil {
		err := json.Unmarshal(l.data, &l.iface)
		if err != nil {
			return nil, err
		}
	}
	return l.iface, nil
}
