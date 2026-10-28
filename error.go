package enigma

import (
	"errors"
	"fmt"
	"strings"
)

// ErrObjectNotFound is returned by methods such as Doc.GetObject, Doc.GetField and Doc.GetVariableByName
// when the app has no object with the given id or name. The engine doesn't treat that as an error and
// answers with a null handle instead.
var ErrObjectNotFound = errors.New("enigma: object not found")

type (
	// Error extends the built in error type with extra error information provided by the Qlik Associative Engine
	Error interface {
		error
		Code() int
		Parameter() string
		Message() string
	}

	qixError struct {
		ErrorCode      int    `json:"code"`
		ErrorParameter string `json:"parameter"`
		ErrorMessage   string `json:"message"`
	}
)

func (err *qixError) Error() string {
	errorType := err.codeLookup()
	return fmt.Sprintf("%s: %s (%d %s)", err.ErrorParameter, err.ErrorMessage, err.ErrorCode, errorType)
}

func (err *qixError) Code() int {
	return err.ErrorCode
}

func (err *qixError) Parameter() string {
	return err.ErrorParameter
}

func (err *qixError) Message() string {
	return err.ErrorMessage
}

func (err *qixError) codeLookup() string {
	errorType := ErrorCodeLookup(err.ErrorCode)
	if len(errorType) < 7 {
		// All error codes should result in a string prefixed with LOCERR_
		// if the code is valid, so this is probably invalid.
		// Just return whatever was returned by the lookup to be safe.
		return errorType
	}
	if errorType[:7] == "LOCERR_" {
		errorType = errorType[7:]
	}
	errorType = strings.Replace(errorType, "_", " ", -1)
	return errorType
}
