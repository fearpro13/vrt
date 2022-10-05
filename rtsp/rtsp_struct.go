package rtsp

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const CSEQ = "CSEQ"
const DESCRIBE = "DESCRIBE"
const OPTIONS = "OPTIONS"
const SETUP = "SETUP"
const PLAY = "PLAY"
const PAUSE = "PAUSE"
const TEARDOWN = "TEARDOWN"

type Request struct {
	Method      string
	Uri         string
	Headers     map[string]string
	Body        string
	rtspVersion string
	userAgent   string
}

type Response struct {
	Status      int
	Reason      string
	Headers     map[string]string
	Body        string
	rtspVersion string
}

func NewRequest(method string, uri string) *Request {
	request := &Request{
		Method:      method,
		Uri:         uri,
		Headers:     map[string]string{},
		Body:        "",
		rtspVersion: "1.0",
		userAgent:   "VRT 1.0",
	}

	return request
}

func ParseRequest(message string) (request *Request, err error) {
	request = NewRequest("", "")

	messageLines := strings.Split(message, "\r\n")

	contentLenExp := regexp.MustCompile("^[cC]ontent-[lL]ength:\\s*(\\d+)$")

	endOfResponse := false
	hasContent := false
	var contentLen int

	statusLineExp := regexp.MustCompile("^(\\w+)\\s+(.+)\\s[rRtTsSpP]+/(\\d+.\\d+)$")

	firstResponseLine := messageLines[0]

	if !statusLineExp.MatchString(firstResponseLine) {
		return nil, errors.New("request does not have status line")
	}

	statusLine := statusLineExp.FindStringSubmatch(firstResponseLine)
	request.Method = strings.ToUpper(statusLine[1])
	request.Uri = statusLine[2]
	request.rtspVersion = statusLine[3]

	headerExp := regexp.MustCompile("^(.*)\\s*:\\s*(.*)$")

	curPos := 1
	for !endOfResponse {
		responseLine := messageLines[curPos]

		if responseLine == "" {
			if hasContent {
				body := strings.Join(messageLines[curPos:], "\r\n")
				if len(body) > contentLen {
					return nil, errors.New("body contains more data than described in header 'Content-Length'")
				}
				request.Body = body
			}
			endOfResponse = true
		} else if contentLenExp.MatchString(responseLine) {
			hasContent = true
			contentLenString := contentLenExp.FindStringSubmatch(responseLine)[1]
			contentLen, _ = strconv.Atoi(contentLenString)
		} else if headerExp.MatchString(responseLine) {
			headerSplit := headerExp.FindStringSubmatch(responseLine)
			headerName := headerSplit[1]
			headerValue := headerSplit[2]
			request.AddHeader(headerName, headerValue)
		}

		curPos++
	}

	return request, nil
}

func (request *Request) AddHeader(name string, value string) {
	request.Headers[formatHeaderName(name)] = value
}

func (request *Request) GetHeader(name string) (headerValue string, exist bool) {
	headerValue, exist = request.Headers[formatHeaderName(name)]
	return headerValue, exist
}

func (request *Request) AddBody(body string) {
	request.Body = body
}

func (request *Request) GetBody() string {
	return request.Body
}

func (request *Request) ToString() string {
	requestString := fmt.Sprintf("%s %s RTSP/%s\r\n", request.Method, request.Uri, request.rtspVersion)
	for headerName, headerValue := range request.Headers {
		requestString += fmt.Sprintf("%s: %s\r\n", headerName, headerValue)
	}

	contentLen := len(request.Body)

	if contentLen > 0 {
		requestString += fmt.Sprintf("Content-length: %d\r\n\r\n", contentLen)
		requestString += request.Body
	} else {
		requestString += "\r\n"
	}

	return requestString
}

func formatHeaderName(name string) string {
	name = strings.TrimSpace(name)

	if name == "" {
		return name
	}

	name = strings.ToLower(name)
	nameSplit := strings.Split(name, "")
	nameSplit[0] = strings.ToUpper(nameSplit[0])
	return strings.Join(nameSplit, "")
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func NewResponse(status int, reason string) *Response {
	response := &Response{
		Status:      status,
		Reason:      reason,
		Headers:     map[string]string{},
		Body:        "",
		rtspVersion: "1.0",
	}

	return response
}

func ParseResponse(message string) (response *Response, err error) {
	response = NewResponse(0, "")

	messageLines := strings.Split(message, "\r\n")

	contentLenExp := regexp.MustCompile("^[cC]ontent-[lL]ength:\\s*(\\d+)$")

	endOfResponse := false
	hasContent := false
	var contentLen int

	statusLineExp := regexp.MustCompile("^[rRtTsSpP]+/(\\d+.\\d+)\\s+(\\d+)\\s+(.*)$")

	firstResponseLine := messageLines[0]

	if !statusLineExp.MatchString(firstResponseLine) {
		return nil, errors.New("response does not have status line")
	}

	statusLine := statusLineExp.FindStringSubmatch(firstResponseLine)
	response.rtspVersion = statusLine[1]
	response.Status, _ = strconv.Atoi(statusLine[2])
	response.Reason = statusLine[3]

	headerExp := regexp.MustCompile("^(.*)\\s*:\\s*(.*)$")

	curPos := 1
	for !endOfResponse {
		responseLine := messageLines[curPos]

		if responseLine == "" {
			if hasContent {
				body := strings.Join(messageLines[curPos+1:len(messageLines)-1], "\r\n")
				bodyLen := len(body)
				if bodyLen > contentLen {
					return nil, errors.New("body contains more data than described in header 'Content-Length'")
				}
				response.Body = body
			}
			endOfResponse = true
		} else if contentLenExp.MatchString(responseLine) {
			hasContent = true
			contentLenString := contentLenExp.FindStringSubmatch(responseLine)[1]
			contentLen, _ = strconv.Atoi(contentLenString)
		} else if headerExp.MatchString(responseLine) {
			headerSplit := headerExp.FindStringSubmatch(responseLine)
			headerName := headerSplit[1]
			headerValue := headerSplit[2]
			response.AddHeader(headerName, headerValue)
		}

		curPos++
	}

	return response, nil
}

func (response *Response) AddHeader(name string, value string) {
	response.Headers[formatHeaderName(name)] = value
}

func (response *Response) GetHeader(name string) (headerValue string, exist bool) {
	headerValue, exist = response.Headers[formatHeaderName(name)]
	return headerValue, exist
}

func (response *Response) AddBody(body string) {
	response.Body = body
}

func (response *Response) GetBody() string {
	return response.Body
}

func (response *Response) ToString() string {
	now := time.Now()
	nowFormatted := now.Format("Mon, Jan 02 2006 15:04:05 MST")

	response.AddHeader("Data", nowFormatted)

	responseString := fmt.Sprintf("RTSP/%s %d %s\r\n", response.rtspVersion, response.Status, response.Reason)
	for headerName, headerValue := range response.Headers {
		responseString += fmt.Sprintf("%s: %s\r\n", headerName, headerValue)
	}

	contentLen := len(response.Body)

	if contentLen > 0 {
		responseString += fmt.Sprintf("Content-length: %d\r\n\r\n", contentLen)
		responseString += response.Body
	} else {
		responseString += "\r\n"
	}

	return responseString
}
