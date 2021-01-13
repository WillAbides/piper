package testutil

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// MockPutter is a mock Putter
type MockPutter struct {
	t    testing.TB
	want [][][]byte
}

// NewMockPutter returns a new MockPutter
func NewMockPutter(t testing.TB) *MockPutter {
	t.Helper()
	p := &MockPutter{
		t: t,
	}
	t.Cleanup(func() {
		assert.Empty(t, p.want)
	})
	return p
}

// Expect expect this call to FlushEvents
func (p *MockPutter) Expect(expect [][]byte) {
	p.want = append(p.want, expect)
}

// FlushEvents mock FlushEvents
func (p *MockPutter) FlushEvents(_ context.Context, cache [][]byte) error {
	t := p.t
	t.Helper()
	if len(p.want) == 0 {
		err := fmt.Errorf("unexpected request")
		assert.NoError(t, err)
		return err
	}
	assert.Equal(t, p.want[0], cache)
	p.want = p.want[1:]
	return nil
}
