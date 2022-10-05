package rtsp_server

import (
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"vrt/logger"
	"vrt/rtsp"
	"vrt/rtsp/rtsp_client"
	"vrt/tcp/tcp_client"
	"vrt/tcp/tcp_server"
	"vrt/udp/udp_client"
)

type OnClientConnectCallback func(client *rtsp_client.RtspClient)

type RtspServer struct {
	SessionId int32
	Login     string
	Password  string
	Server    *tcp_server.TcpServer
	Ip        string
	Port      int

	RtspAddress        string
	VideoStreamAddress string
	AudioStreamAddress string

	VideoId int
	AudioId int

	SdpRaw string

	RtpVideoClient *udp_client.UdpClient
	RtpAudioClient *udp_client.UdpClient
	//RtpServer      *udp_server.UdpServer

	//RtpLocalPort  int
	//RtcpServer    *udp_server.UdpServer
	//RtcpLocalPort int

	sync.Mutex
	Clients                  map[int32]*rtsp_client.RtspClient
	OnClientConnectListeners map[int32]OnClientConnectCallback
	IsRunning                bool
}

func Create() *RtspServer {
	id := rand.Int31()
	server := &RtspServer{
		SessionId: id,
		Login:     "user",
		Password:  "qwerty",
		VideoId:   0,
		AudioId:   2,
	}
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
	rtspServer.RtpVideoClient = rtpClient

	//randomPort := rand.Intn(64000) + 1024

	//rtpServer := udp_server.Create()
	//err = rtpServer.Start(ip, randomPort)
	//rtspServer.RtpServer = rtpServer

	//rtcpServer := udp_server.Create()
	//err = rtcpServer.Start(ip, randomPort+1)
	//rtspServer.RtcpServer = rtcpServer

	rtspAddress := fmt.Sprintf("rtsp://%s:%s@%s:%d", rtspServer.Login, rtspServer.Password, tcpServer.Ip, tcpServer.Port)
	rtspServer.RtspAddress = rtspAddress
	rtspServer.VideoStreamAddress = rtspAddress + "/" + "video1"
	rtspServer.AudioStreamAddress = rtspAddress + "/" + "audio1"
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

	if rtspServer.RtpVideoClient.IsConnected {
		err = rtspServer.RtpVideoClient.Disconnect()
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
	rtspClient.RemoteAddress = rtspServer.RtspAddress
	for rtspClient.IsConnected && !rtspClient.IsPlaying {
		request, err := rtspClient.ReadRequest()
		if err != nil {
			logger.Error(err.Error())
			rtspClient.TearDown()
			rtspClient.Disconnect()
			return
		}

		response, err := parseRequest(rtspServer, rtspClient, request)
		if err != nil {
			logger.Error(err.Error())
			rtspClient.TearDown()
			return
		}

		_, err = rtspClient.TcpClient.SendString(response.ToString())
		if err != nil {
			logger.Error(err.Error())
			rtspClient.TearDown()
			return
		}
	}
}

func parseRequest(server *RtspServer, client *rtsp_client.RtspClient, requestRaw string) (response *rtsp.Response, err error) {
	request, err := rtsp.ParseRequest(requestRaw)
	if err != nil {
		return nil, err
	}

	method := request.Method

	if method == "" {
		return nil, errors.New(fmt.Sprintf("rtsp server #%d: RTSP client #%d - Некорректный метод запроса %s", server.SessionId, client.SessionId, method))
	}

	cSeq, exist := request.GetHeader(rtsp.CSEQ)

	if !exist {
		return nil, errors.New(fmt.Sprintf("rtsp server #%d: RTSP client #%d - Отсутствует заголовок Cseq", server.SessionId, client.SessionId))
	}

	client.CSeq, _ = strconv.Atoi(cSeq)

	response = rtsp.NewResponse(200, "OK")
	response.AddHeader(rtsp.CSEQ, cSeq)
	response.AddHeader("Content-Type", "application/sdp")
	response.AddHeader("Content-Base", server.RtspAddress)
	if method == rtsp.DESCRIBE {
		response.AddBody(server.SdpRaw)
	}

	if method == rtsp.OPTIONS {
		response.AddHeader("Public", "OPTIONS, DESCRIBE, PLAY, PAUSE, SETUP, TEARDOWN, SET_PARAMETER, GET_PARAMETER")
	}

	if method == rtsp.SETUP {
		header, exist := request.GetHeader("Transport")
		if !exist {
			return nil, errors.New("request does not contain transport info")
		}

		transportExp := regexp.MustCompile("[rR][tT][pP]/[aA][vV][pP]/(\\w+)")
		var transport string
		if !transportExp.MatchString(header) {
			logger.Warning(fmt.Sprintf("RTSP client #%d: Could not detect transport protocol", client.SessionId))
			logger.Warning(fmt.Sprintf("RTSP client #%d: Trying to use UDP as a transport protocol", client.SessionId))
			transport = rtsp_client.RtspTransportUdp
		} else {
			transport = strings.ToLower(transportExp.FindStringSubmatch(header)[1])
		}

		switch transport {
		case rtsp_client.RtspTransportTcp:
			client.Transport = rtsp_client.RtspTransportTcp

			var interleaveChannelId int
			switch request.Uri {
			case server.VideoStreamAddress:
				interleaveChannelId = server.VideoId
			case server.AudioStreamAddress:
				interleaveChannelId = server.AudioId
			default:
			}

			response.AddHeader("Transport", fmt.Sprintf("RTP/AVP/TCP;unicast;interleaved=%d-%d;ssrc=60d45a65;mode=\"play\"", interleaveChannelId, interleaveChannelId+1))
		case rtsp_client.RtspTransportUdp:
			client.Transport = rtsp_client.RtspTransportUdp

			transportPortExp, err := regexp.Compile("(\\d+)-(\\d+)")
			if err != nil {
				return nil, err
			}
			if !transportPortExp.MatchString(header) {
				return nil, errors.New(fmt.Sprintf("RTSP server #%d: Tried to establish UDP connection, but client did not send port range", server.SessionId))
			}
			portMatches := transportPortExp.FindStringSubmatch(header)
			clientRtpPortLeftInt64, err := strconv.ParseInt(portMatches[1], 10, 64)
			clientRtpPortRightInt64, err := strconv.ParseInt(portMatches[2], 10, 64)

			clientRtpPortLeftInt := int(clientRtpPortLeftInt64)
			clientRtpPortRightInt := int(clientRtpPortRightInt64)

			var serverRtpPort int
			switch request.Uri {
			case server.VideoStreamAddress:
				rtpVideoClient := udp_client.Create()
				err = rtpVideoClient.Connect(client.TcpClient.Ip, clientRtpPortLeftInt)
				client.RtpVideoClient = rtpVideoClient

				serverRtpPort = rtpVideoClient.LocalPort
			case server.AudioStreamAddress:
				rtpAudioClient := udp_client.Create()
				err = rtpAudioClient.Connect(client.TcpClient.Ip, clientRtpPortLeftInt)
				client.RtpAudioClient = rtpAudioClient

				serverRtpPort = client.RtpAudioClient.LocalPort
			default:
			}

			response.AddHeader("Transport", fmt.Sprintf("RTP/AVP/UDP;unicast;client_port=%d-%d;server_port=%d-%d;ssrc=60d45a65;mode=\"play\"", clientRtpPortLeftInt, clientRtpPortRightInt, serverRtpPort, serverRtpPort+1))
		default:
			return nil, errors.New(fmt.Sprintf("RTSP server #%d: transport %s is not supported", server.SessionId, transport))
		}

		logger.Info(fmt.Sprintf("RTSP client #%d: Selected %s as a transport protocol", client.SessionId, transport))
		response.AddHeader("Session", fmt.Sprintf("%d;timeout=60", client.SessionId))
	}

	if method == rtsp.PLAY {
		response.AddHeader("Session", fmt.Sprintf("%d", client.SessionId))
		response.AddHeader("RTP-Info", fmt.Sprintf("url=%s;seq=4563;rtptime=1435052840", server.VideoStreamAddress))
		client.IsPlaying = true
		for _, listener := range client.OnStartPlayingListeners {
			go listener(client)
		}
	}

	if method == rtsp.TEARDOWN {
		response.AddHeader("Session", fmt.Sprintf("%d", client.SessionId))
		client.IsPlaying = false
	}

	return response, err
}
