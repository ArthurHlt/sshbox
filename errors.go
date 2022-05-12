package sshbox

import (
	"fmt"
	"strings"
)

type ErrLoad struct {
	content string
}

func (e ErrLoad) Error() string {
	return e.content
}

func errLoadErrorf(format string, a ...interface{}) *ErrLoad {
	return &ErrLoad{content: fmt.Sprintf(format, a...)}
}

func errLoadWrap(err error) *ErrLoad {
	return &ErrLoad{content: err.Error()}
}

type TerminalError struct {
	content []byte
}

func errTerminalError(content []byte) *TerminalError {
	return &TerminalError{content: content}
}

func (e TerminalError) Error() string {
	return "Detected error: \n  " + strings.Replace(string(e.content), "\n", "\n  ", -1)
}

func IsTerminalError(err error) (*TerminalError, bool) {
	if errTerm, ok := err.(*TerminalError); ok {
		return errTerm, true
	}
	return nil, false
}
