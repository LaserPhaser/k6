package combinators

import (
	"fmt"
	"strings"
)

type Error struct {
	Input    []rune
	Err      error
	Expected []string
}

func (e *Error) Error() string {
	return fmt.Sprintf("expected %v", strings.Join(e.Expected, ", "))
}

func (e *Error) Unwrap() error {
	return e.Err
}

func (e *Error) ErrorAtChar(fullInput []rune) string {
	char := len(fullInput) - len(e.Input)
	return fmt.Sprintf("char at position %v, %v", char+1, e.Error())
}

func (e *Error) IsFatal() bool {
	return e.Err != nil
}

func (e *Error) Add(from *Error) {
	e.Expected = append(e.Expected, from.Expected...)
	if e.Err == nil {
		e.Err = from.Err
	}
}

func NewFatalError(input []rune, err error, expected ...string) *Error {
	return &Error{Input: input, Err: err, Expected: expected}
}

func NewError(input []rune, expected ...string) *Error {
	return &Error{
		input,
		nil,
		expected,
	}
}
