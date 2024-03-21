package Gateway_FlowController

import (
	"net"
	"net/http"
	"strings"
)

func GetIP(req *http.Request) string {
	// Try X-Forwarded-For
	xff := req.Header.Get("X-Forwarded-For")
	xffs := strings.Split(xff, ",")

	if len(xffs) > 1 {
		// Get first
		return strings.TrimSpace(xffs[0])
	}

	// No X-Forwarded-For , use RemoteAddr
	ip, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}

	return ip
}
