package rtsp_server

import (
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"vrt/logger"
	"vrt/rtsp/rtsp_client"
	"vrt/tcp/tcp_client"
	"vrt/tcp/tcp_server"
	"vrt/udp/udp_client"
	"vrt/udp/udp_server"
)

type OnClientConnectCallback func(client *rtsp_client.RtspClient)

type RtspServer struct {
	SessionId     int32
	Login         string
	Password      string
	Server        *tcp_server.TcpServer
	Ip            string
	Port          int
	RtspAddress   string
	StreamAddress string
	RtpClient     *udp_client.UdpClient
	RtpServer     *udp_server.UdpServer
	RtpLocalPort  int
	RtcpServer    *udp_server.UdpServer
	RtcpLocalPort int
	sync.Mutex
	Clients                  map[int32]*rtsp_client.RtspClient
	OnClientConnectListeners map[int32]OnClientConnectCallback
	IsRunning                bool
}

func Create() *RtspServer {
	id := rand.Int31()
	server := &RtspServer{SessionId: id, Login: "user", Password: "qwerty"}
	server.Clients = map[int32]*rtsp_client.RtspClient{}

	return server
}

func (rtspServer *RtspServer) Start(ip string, port int, rtpClientLocalPort int) error {
	tcpServer := tcp_server.Create()
	tcpServer.OnConnectListeners[rtspServer.SessionId] = rtspServer.connectRtspClient

	err := tcpServer.Start(ip, port)
	if err != nil {
		return err
	}
	rtspServer.Server = tcpServer
	rtspServer.Ip = tcpServer.Ip
	rtspServer.Port = tcpServer.Port

	rtpClient := udp_client.Create()
	rtpClient.LocalPort = rtpClientLocalPort
	rtspServer.RtpClient = rtpClient

	randomPort := rand.Intn(64000) + 1024

	rtpServer := udp_server.Create()
	err = rtpServer.Start(ip, randomPort)
	rtspServer.RtpServer = rtpServer

	rtcpServer := udp_server.Create()
	err = rtcpServer.Start(ip, randomPort+1)
	rtspServer.RtcpServer = rtcpServer

	rtspAddress := fmt.Sprintf("rtsp://%s:%s@%s:%d", rtspServer.Login, rtspServer.Password, tcpServer.Ip, tcpServer.Port)
	streamAddress := rtspAddress + "/" + "stream1"
	rtspServer.RtspAddress = rtspAddress
	rtspServer.StreamAddress = streamAddress
	rtspServer.IsRunning = true

	logger.Info(fmt.Sprintf("RTSP rtspServer started at %s", rtspAddress))

	return err
}

func (rtspServer *RtspServer) Stop() error {
	if !rtspServer.IsRunning {
		return nil
	}

	rtspServer.IsRunning = false

	for _, client := range rtspServer.Clients {
		if client.IsConnected {
			_, _ = client.TearDown()
			_ = client.Disconnect()
		}
	}

	err := rtspServer.Server.Stop()
	if err != nil {
		return err
	}

	if rtspServer.RtpClient.IsConnected {
		err = rtspServer.RtpClient.Disconnect()
	}

	logger.Info(fmt.Sprintf("RTSP rtspServer #%d stopped", rtspServer.SessionId))

	return err
}

func (rtspServer *RtspServer) connectRtspClient(connectedTcpClient *tcp_client.TcpClient) {
	rtspClient := rtsp_client.CreateFromConnection(connectedTcpClient)

	rtspServer.Lock()
	rtspServer.Clients[rtspClient.SessionId] = rtspClient
	rtspServer.Unlock()

	rtspClient.OnDisconnectListeners[rtspServer.SessionId] = func(client *rtsp_client.RtspClient) {
		delete(rtspServer.Clients, client.SessionId)
		logger.Debug(fmt.Sprintf("Cleaned up #%d rtsp rtspClient from rtspServer clients", client.SessionId))
		logger.Debug(fmt.Sprintf("Current number of clients:%d", len(rtspServer.Clients)))
	}

	for _, listener := range rtspServer.OnClientConnectListeners {
		go listener(rtspClient)
	}

	go rtspServer.handleRtspClient(rtspClient)
}

func (rtspServer *RtspServer) handleRtspClient(rtspClient *rtsp_client.RtspClient) {
	for rtspClient.IsConnected && !rtspClient.IsPlaying {
		request, err := rtspClient.ReadMessage()
		if err != nil {
			logger.Error(err.Error())
		}
		response, err := parseRequest(rtspServer, rtspClient, request)
		if err != nil {
			logger.Error(err.Error())
		}
		_, err = rtspClient.TcpClient.SendString(response)
		if err != nil {
			logger.Error(err.Error())
		}
	}
}

func parseRequest(server *RtspServer, client *rtsp_client.RtspClient, request string) (response string, err error) {
	requestLines := strings.Split(request, "\r\n")

	methodExp, err := regexp.Compile("^\\w+")

	if err != nil {
		return "", err
	}

	method := strings.ToLower(methodExp.FindString(requestLines[0]))

	if method == "" {
		return "", errors.New(fmt.Sprintf("rtsp server #%d: Некорректный метод запроса %s", server.SessionId, requestLines[0]))
	}

	cseqExp := regexp.MustCompile("[cC][sS][eE][qQ]:\\s+(\\d+)")
	if !cseqExp.MatchString(request) {
		return "", errors.New(fmt.Sprintf("rtsp server #%d: Отсутствует заголовок Cseq", server.SessionId))
	} else {
		cseqString := cseqExp.FindStringSubmatch(request)[1]
		client.CSeq, err = strconv.Atoi(cseqString)
		if err != nil {
			return "", err
		}
	}

	now := time.Now()
	nowFormatted := now.Format("Mon, Jan 02 2006 15:04:05 MST")

	if method == "describe" {
		response = ""
		response += "RTSP/1.0 200 OK\r\n" +
			fmt.Sprintf("CSeq: %d\r\n", client.CSeq) +
			"Content-Type: application/sdp\r\n" +
			fmt.Sprintf("Content-Base: %s/\r\n", server.RtspAddress)

		rtpInfo := fmt.Sprintf("v=0\r\n"+
			"o=- 1663158230155577 1663158230155577 IN IP4 0.0.0.0\r\n"+
			"s=Media Presentation\r\n"+
			"e=NONE\r\n"+
			"b=AS:5050\r\n"+
			"t=0 0\r\n"+
			"a=control:%s\r\n"+
			"m=video 0 RTP/AVP 96\r\n"+
			"c=IN IP4 0.0.0.0\r\n"+
			"b=AS:5000\r\n"+
			"a=recvonly\r\n"+
			"a=x-dimensions:1280,720\r\n"+
			"a=control:%s\r\n"+
			"a=rtpmap:96 H264/90000\r\n"+
			"a=fmtp:96 profile-level-id=420029; packetization-mode=1; sprop-parameter-sets=Z00AH5WoFAFuhAAAHCAABX5AEA==,aO48gA==\r\n"+
			"a=Media_header:MEDIAINFO=494D4B48010200000400000100000000000000000000000000000000000000000000000000000000;\r\n"+
			"a=appversion:1.0\r\n", server.RtspAddress, server.StreamAddress)

		response += fmt.Sprintf("Content-Length: %d\r\n", len([]byte(rtpInfo))) +
			"\r\n" +
			rtpInfo
	}

	if method == "options" {
		response = "RTSP/1.0 200 OK\r\n" +
			fmt.Sprintf("CSeq: %d\r\n", client.CSeq) +
			"Public: OPTIONS, DESCRIBE, PLAY, PAUSE, SETUP, TEARDOWN, SET_PARAMETER, GET_PARAMETER\r\n" +
			fmt.Sprintf("Date: %s\r\n", nowFormatted) +
			//"Content-Length: 0\r\n\r\n"
			"\r\n"
	}

	if method == "setup" {
		transportExp := regexp.MustCompile("[rR][tT][pP]/[aA][vV][pP]/(\\w+)")
		var transport string
		if !transportExp.MatchString(request) {
			logger.Warning(fmt.Sprintf("RTSP client #%d: Could not detect transport protocol", client.SessionId))
			logger.Warning(fmt.Sprintf("RTSP client #%d: Trying to use UDP as a transport protocol", client.SessionId))
			transport = rtsp_client.RtspTransportUdp
		} else {
			transport = strings.ToLower(transportExp.FindStringSubmatch(request)[1])
		}

		transportInfo := ""
		switch transport {
		case rtsp_client.RtspTransportTcp:
			client.Transport = rtsp_client.RtspTransportTcp
			transportInfo = fmt.Sprintf("Transport: RTP/AVP/TCP;unicast;interleaved=0-1;ssrc=60d45a65;mode=\"play\"\r\n")
		case rtsp_client.RtspTransportUdp:
			client.Transport = rtsp_client.RtspTransportUdp

			transportPortExp, err := regexp.Compile("(\\d+)-(\\d+)")
			if err != nil {
				return "", err
			}
			if !transportPortExp.MatchString(request) {
				return "", errors.New(fmt.Sprintf("RTSP server #%d: Tried to establish UDP connection, but client did not send port range", server.SessionId))
			}
			portMatches := transportPortExp.FindStringSubmatch(request)
			clientRtpPortLeftInt64, err := strconv.ParseInt(portMatches[1], 10, 64)
			clientRtpPortRightInt64, err := strconv.ParseInt(portMatches[2], 10, 64)

			clientRtpPortLeftInt := int(clientRtpPortLeftInt64)
			clientRtpPortRightInt := int(clientRtpPortRightInt64)

			err = client.RtpClient.Connect(client.TcpClient.Ip, clientRtpPortLeftInt)
			serverRtpPort := client.RtpClient.LocalPort

			transportInfo = fmt.Sprintf("Transport: RTP/AVP/UDP;unicast;client_port=%d-%d;server_port=%d-%d;ssrc=60d45a65;mode=\"play\"\r\n", clientRtpPortLeftInt, clientRtpPortRightInt, serverRtpPort, serverRtpPort+1)
		default:
			return "", errors.New(fmt.Sprintf("RTSP server #%d: transport %s is not supported", server.SessionId, transport))
		}

		logger.Info(fmt.Sprintf("RTSP client #%d: Selected %s as a transport protocol", client.SessionId, transport))

		response = "RTSP/1.0 200 OK\r\n" +
			transportInfo +
			fmt.Sprintf("CSeq: %d\r\n", client.CSeq) +
			fmt.Sprintf("Session: %d;timeout=60\r\n", client.SessionId)

		response += "Content-Length: 0\r\n\r\n"
	}

	if method == "play" {
		response = "RTSP/1.0 200 OK\r\n" +
			fmt.Sprintf("CSeq: %d\r\n", client.CSeq) +
			fmt.Sprintf("Session:%d\r\n", client.SessionId) +
			fmt.Sprintf("RTP-Info: url=%s;seq=4563;rtptime=1435052840\r\n", server.StreamAddress) +
			fmt.Sprintf("Date: %s\r\n", nowFormatted) +
			"Content-Length: 0\r\n\r\n"

		client.IsPlaying = true
		for _, listener := range client.OnStartPlayingListeners {
			go listener(client)
		}
	}

	if method == "teardown" {
		response = "RTSP/1.0 200 OK\r\n" +
			fmt.Sprintf("CSeq: %d\r\n", client.CSeq) +
			fmt.Sprintf("Session:%d\r\n", client.SessionId) +
			fmt.Sprintf("%s\r\n", nowFormatted) +
			"Content-Length: 0\r\n\r\n"

		client.IsPlaying = false
	}

	return response, err
}
