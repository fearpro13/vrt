package rtsp

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"vrt/logger"
)

const CSeq = "CSeq"
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
	}

	return request
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

	nameSplit := strings.Split(name, "")
	nameSplit[0] = strings.ToUpper(nameSplit[0])
	return strings.Join(nameSplit, "")
}

func parseResponse(message string) (response *Response, err error) {
	messageLines := strings.Split(message, "\r\n")

	contentLenExp := regexp.MustCompile("^[cC]ontent-[lL]ength:\\s*(\\d+)$")
	endOfResponse := false
	hasContent := false
	var contentLen int

	statusLineExp := regexp.MustCompile("^[rR][tT][sS][pP]/(\\d+.\\d+)\\s+(\\d+)\\s+(\\w+)$")

	firstResponseLine := messageLines[0]

	if !statusLineExp.MatchString(firstResponseLine) {
		return response, errors.New("response does not have status line")
	}

	statusLine := statusLineExp.FindStringSubmatch(firstResponseLine)
	response.rtspVersion = statusLine[1]
	response.Status, _ = strconv.Atoi(statusLine[2])
	response.Reason = statusLine[3]

	curPos := 0
	for !endOfResponse {
		responseLine := messageLines[curPos]

		if responseLine == "" {
			if hasContent {
				sdpContent, _, _ := rtspClient.TcpClient.ReadBytes(contentLen)
				message += string(sdpContent)
			}
			endOfResponse = true
		} else if contentLenExp.MatchString(responseLine) {
			hasContent = true
			contentLenString := contentLenExp.FindStringSubmatch(responseLine)[1]
			contentLen, _ = strconv.Atoi(contentLenString)
		}

		message += responseLine + "\r\n"
	}

	logger.Junk(fmt.Sprintf("Rtsp rtspClient #%d: received message:", rtspClient.SessionId))
	logger.Junk(message)

	return statusCode, message, err
}

//message := ""
//message += fmt.Sprintf("DESCRIBE %s RTSP/1.0\r\n", rtspClient.RemoteAddress)
//message += fmt.Sprintf("CSeq: %d\r\n", rtspClient.CSeq)
//message += "Accept: application/sdp\r\n"
//if rtspClient.Auth != nil {
//message += fmt.Sprintf("%s\r\n", rtspClient.Auth.GetHeader())
//}
//message += "\r\n"
//
//rtspClient.CSeq++
//_, err = rtspClient.TcpClient.SendString(message)
//if err != nil {
//return 0, "", nil
//}
//
//status, response, err = rtspClient.ReadResponse()
//if err != nil {
//return status, "", err
//}
//
//if status == 401 && !rtspClient.Auth.Tried {
//rtspClient.Auth.Tried = true
//rtspClient.Describe()
//}
//
//parseSdp(rtspClient, &response)
//
//return status, response, err
