package monitor

func HttpCodeCheck(httpCode int) string {
	// 429 Too Many Requests
	if httpCode == 429 {
		return "Too Many Requests, code:429"
	}
	// 503 Service Unavailable
	if httpCode == 503 {
		return "Service Unavailable, code:503"
	}
	// 504 Gateway Timeout
	if httpCode == 504 {
		return "Gateway Timeout, code:504"
	}
	return ""
}
