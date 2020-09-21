package sshbox

import (
	"fmt"
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
