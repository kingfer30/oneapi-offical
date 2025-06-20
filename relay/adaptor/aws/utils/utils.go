package utils

import (
	"net/http"

	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

func WrapErr(err error, status int) *relaymodel.ErrorWithStatusCode {
	if status == 0 {
		status = http.StatusInternalServerError
	}
	return &relaymodel.ErrorWithStatusCode{
		StatusCode: status,
		Error: relaymodel.Error{
			Message: err.Error(),
		},
	}
}
