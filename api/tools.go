package api

import (
	"fmt"
	"log"
	"net/http"
	"runtime"

	"github.com/pkg/errors"
)

type HttpError struct {
	Cause string
	Code  int
}

func (e HttpError) Error() string {
	return fmt.Sprint(e.Code, " ", e.Cause)
}

// HttpErrorOnPanic recovers from a panic, and will return a HTTP request with an
// error. If the error is a HttpError, the response code will be used.
//
// Usage: put this at the top of the request
// defer HttpErrorOnPanic(w)
func HttpErrorOnPanic(w http.ResponseWriter, defaultErrorCode int) {
	if err := recover(); err != nil {
		errorCode := defaultErrorCode
		if _, ok := err.(runtime.Error); ok {
			http.Error(w, "Unexpected Server Error", defaultErrorCode)
			log.Printf("in request: %+v\n", errors.WithStack(err.(error)))
		} else {
			if e, ok := err.(HttpError); ok && e.Code != 0 {
				errorCode = e.Code
			}
			http.Error(w, err.(error).Error(), errorCode)
		}
	}
}

func Check(err error, status string, errorCode int) {
	if err != nil {
		panic(HttpError{"Error " + status + ": " + err.Error(), errorCode})
	}
}
