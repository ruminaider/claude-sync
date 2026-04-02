package sliceutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppendUnique(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c"}, AppendUnique([]string{"a", "b"}, []string{"b", "c"}))
	assert.Equal(t, []string{"a"}, AppendUnique([]string{"a"}, []string{"a"}))
	assert.Equal(t, []string{"a"}, AppendUnique([]string{"a"}, nil))
	assert.Equal(t, []string{"a"}, AppendUnique(nil, []string{"a"}))
	assert.Nil(t, AppendUnique(nil, nil))
}

func TestRemoveAll(t *testing.T) {
	assert.Equal(t, []string{"a"}, RemoveAll([]string{"a", "b", "c"}, []string{"b", "c"}))
	assert.Nil(t, RemoveAll([]string{"a"}, []string{"a"}))
	assert.Equal(t, []string{"a", "b"}, RemoveAll([]string{"a", "b"}, nil))
	assert.Nil(t, RemoveAll(nil, []string{"a"}))
}
