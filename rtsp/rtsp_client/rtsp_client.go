package rtsp_client

import (
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"vrt/logger"
	"vrt/tcp/tcp_client"
	"vrt/udp/udp_client"
	"vrt/udp/udp_server"
)

type RtspClient struct {
	TcpClient           *tcp_client.TcpClient
	RtpServer           *udp_server.UdpServer
	RtpClient           *udp_client.UdpClient
	SessionId           int64
	RemoteAddress       string
	RemoteStreamAddress string
	IsConnected         bool
}

func Create() RtspClient {
	sessionId := rand.Int63()
	tcpClient := tcp_client.Create()
	udpServer := udp_server.Create()
	udpClient := udp_client.Create()

	//logger.Info(fmt.Sprintf("RTSP client #%d created", sessionId))

	return RtspClient{SessionId: sessionId, TcpClient: &tcpClient, RtpServer: &udpServer, RtpClient: &udpClient}
}

func (client *RtspClient) Connect(address string) error {
	//rtsp://login:pass@10.0.43.19
	hasRtsp, _ := regexp.MatchString("^rtsp:/{2}", address)
	hasCreds, _ := regexp.MatchString("\\w+:\\w+", address)

	if !hasRtsp {
		return errors.New("отсутствует префикс rtsp://")
	}

	if !hasCreds {
		return errors.New("отсутсвуют данные для авторизации BasicAuth")
	}

	ipExp, _ := regexp.Compile("\\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\\b")
	ip := ipExp.FindString(address)

	if ip == "" {
		return errors.New("отсутствует IP адрес")
	}

	portExp, _ := regexp.Compile("(:\\d+)")
	portString := portExp.FindString(address)

	var portInt int
	if portString == "" {
		portInt = 554
	} else {
		portString = strings.Replace(portString, ":", "", 1)
		portInt64, _ := strconv.ParseInt(portString, 10, 0)
		portInt = int(portInt64)
	}

	client.RemoteAddress = address
	client.IsConnected = true

	err := client.TcpClient.Connect(ip, portInt)

	logger.Info(fmt.Sprintf("RTSP client #%d connected", client.SessionId))

	go client.run()

	return err
}

func (client *RtspClient) Disconnect() error {
	client.IsConnected = false

	/*
		Если rtsp клиент используется для подключения к удалённому серверу - RTP client не создаётся, создаётся только RTP сервер
		Если входящее соединение рассматривается как RTP клиент - RTP client создаётся, но не создаётся RTP сервер
	*/
	if client.RtpClient.IsConnected {
		err := client.RtpClient.Disconnect()
		if err != nil {
			return err
		}
	}

	if client.RtpServer.IsRunning {
		err := client.RtpServer.Stop()
		if err != nil {
			return err
		}
	}

	err := client.TcpClient.Disconnect()

	logger.Info(fmt.Sprintf("RTSP client #%d disconnected", client.SessionId))

	return err
}

func (client *RtspClient) ReadMessage() (message string, err error) {
	message, err = client.TcpClient.ReadMessage()
	parseMessage(client, &message)
	return message, err
}

func (client *RtspClient) Describe() error {
	message := ""
	message += fmt.Sprintf("DESCRIBE %s RTSP/1.0\r\n", client.RemoteAddress)
	message += "CSeq: 1\r\n"
	message += "Accept: application/sdp, application/rtsl, application/mheg\r\n"
	message += "\r\n"

	_, err := client.TcpClient.Send(message)
	return err
}

func (client *RtspClient) Options() error {
	message := ""
	message += fmt.Sprintf("OPTIONS %s RTSP/1.0\r\n", client.RemoteAddress)
	message += "CSeq: 1\r\n"
	message += "Accept: application/sdp, application/rtsl, application/mheg\r\n"
	message += "\r\n"
	_, err := client.TcpClient.Send(message)
	return err
}

func (client *RtspClient) Setup() error {
	randPortInt := rand.Intn(64000) + 1024
	err := client.RtpServer.Start("", randPortInt-(randPortInt%2))

	if err != nil {
		return err
	}

	portMin := client.RtpServer.Port
	portMax := portMin + 1

	message := ""
	message += fmt.Sprintf("SETUP %s RTSP/1.0\r\n", client.RemoteStreamAddress)
	message += "CSeq: 2\r\n"
	message += fmt.Sprintf("Transport: RTP/AVP/UDP;unicast;client_port=%d-%d\r\n", portMin, portMax)
	message += "\r\n"

	_, err = client.TcpClient.Send(message)

	return err
}

func (client *RtspClient) Play() error {
	if client.SessionId == 0 {
		return errors.New("для начала воспроизведения необходимо получить sessionId от сервера")
	}

	message := ""
	message += fmt.Sprintf("PLAY %s RTSP/1.0\r\n", client.RemoteAddress)
	message += "CSeq: 1\r\n"
	message += fmt.Sprintf("Session:%d", client.SessionId)
	message += "Accept: application/sdp, application/rtsl, application/mheg\r\n"
	message += "\r\n"
	_, err := client.TcpClient.Send(message)
	return err
}

func (client *RtspClient) Pause() error {
	if client.SessionId == 0 {
		return errors.New("для начала воспроизведения необходимо получить sessionId от сервера")
	}

	message := ""
	message += fmt.Sprintf("PAUSE %s RTSP/1.0\r\n", client.RemoteAddress)
	message += "CSeq: 1\r\n"
	message += fmt.Sprintf("Session:%d", client.SessionId)
	message += "Accept: application/sdp, application/rtsl, application/mheg\r\n"
	message += "\r\n"
	_, err := client.TcpClient.Send(message)
	return err
}

func (client *RtspClient) TearDown() error {
	if client.SessionId == 0 {
		return errors.New("для начала воспроизведения необходимо получить sessionId от сервера")
	}

	message := ""
	message += fmt.Sprintf("TEARDOWN %s RTSP/1.0\r\n", client.RemoteAddress)
	message += "CSeq: 1\r\n"
	message += fmt.Sprintf("Session:%d", client.SessionId)
	message += "Accept: application/sdp, application/rtsl, application/mheg\r\n"
	message += "\r\n"

	_, err := client.TcpClient.Send(message)

	return err
}

func parseMessage(client *RtspClient, message *string) {
	lines := strings.Split(*message, "\r\n")
	for _, line := range lines {
		matches, _ := regexp.MatchString("a=control", line)
		if matches {
			//a=control:rtsp://stream:Tv4m6ag6@10.3.43.140:554/trackID=1
			remoteStreamAddressExp, _ := regexp.Compile(":(.+)")
			remoteStreamAddress := remoteStreamAddressExp.FindStringSubmatch(line)[1]
			client.RemoteStreamAddress = remoteStreamAddress
		}

		matches, _ = regexp.MatchString("Session.*:.*\\d+", line)
		if matches {
			sessionIdExp, _ := regexp.Compile("\\d+")
			sessionIdString := sessionIdExp.FindString(line)
			sessionIdInt64, _ := strconv.ParseInt(sessionIdString, 10, 64)
			client.SessionId = sessionIdInt64
		}
	}
}

func (client *RtspClient) run() {
	for client.IsConnected {
		buff := <-client.TcpClient.RecvBuff
		if len(buff) > 0 {
			message := string(buff)
			parseMessage(client, &message)
			logger.Debug(message)
		}
	}
}
