package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveResultMsg_Success(t *testing.T) {
	msg := resolveResultMsg(true, "all good", nil)
	assert.Equal(t, "all good", msg)
}

func TestResolveResultMsg_ErrorWithMessage(t *testing.T) {
	msg := resolveResultMsg(false, "custom error", nil)
	assert.Equal(t, "custom error", msg)
}

func TestResolveResultMsg_ErrorWithoutMessage(t *testing.T) {
	msg := resolveResultMsg(false, "", assert.AnError)
	assert.Equal(t, assert.AnError.Error(), msg)
}

func TestResolveResultMsg_UnknownError(t *testing.T) {
	msg := resolveResultMsg(false, "", nil)
	assert.Equal(t, "Unknown error", msg)
}
