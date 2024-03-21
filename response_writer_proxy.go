package GatewayFlowController

import (
	"net/http"
)

// Refer to https://stackoverflow.com/questions/53272536/how-do-i-get-response-statuscode-in-golang-middleware

type ResponseWriterProxy struct {
	http.ResponseWriter

	statusCode int
}

func NewResponseWriterProxy(rw http.ResponseWriter) *ResponseWriterProxy {
	return &ResponseWriterProxy{
		rw,
		http.StatusOK, // Default status
	}
}

func (rwp *ResponseWriterProxy) WriteHeader(statusCode int) {
	// Update self
	rwp.statusCode = statusCode
	// Then proxy
	rwp.ResponseWriter.WriteHeader(statusCode)
}

func (rwp *ResponseWriterProxy) StatusCode() int {
	return rwp.statusCode
}
