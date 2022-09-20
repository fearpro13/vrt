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
	rtspClient "vrt/rtsp/rtsp_client"
	"vrt/tcp/tcp_server"
	"vrt/udp/udp_client"
)

type RtspServer struct {
	Id            int64
	Login         string
	Password      string
	Server        *tcp_server.TcpServer
	Ip            string
	Port          int
	RtspAddress   string
	StreamAddress string
	RtpClient     *udp_client.UdpClient
	RtpServer     *tcp_server.TcpServer
	RtpLocalPort  int
	RtcpServer    *tcp_server.TcpServer
	RtcpLocalPort int
	Clients       map[int64]*rtspClient.RtspClient
	IsRunning     bool
}

func Create() RtspServer {
	id := rand.Int63()
	server := RtspServer{Id: id, Login: "user", Password: "qwerty"}
	server.Clients = map[int64]*rtspClient.RtspClient{}

	//logger.Debug(fmt.Sprintf("Created Rtsp server #%d", id))

	return server
}

func (server *RtspServer) Start(ip string, port int, rtpClientLocalPort int) error {
	tcpServer := tcp_server.Create()
	err := tcpServer.Start(ip, port)
	if err != nil {
		return err
	}
	server.Server = &tcpServer

	rtpClient := udp_client.Create()
	rtpClient.LocalPort = rtpClientLocalPort
	server.RtpClient = &rtpClient

	random := rand.Uint32()
	randomPort := int(random >> 16)

	rtpServer := tcp_server.Create()
	err = rtpServer.Start(ip, randomPort)
	server.RtpServer = &rtpServer

	rtcpServer := tcp_server.Create()
	err = rtcpServer.Start(ip, randomPort+1)
	server.RtcpServer = &rtcpServer

	rtspAddress := fmt.Sprintf("rtsp://%s:%s@%s:%d", server.Login, server.Password, tcpServer.Ip, tcpServer.Port)
	streamAddress := rtspAddress + "/" + "stream1"
	server.RtspAddress = rtspAddress
	server.StreamAddress = streamAddress
	server.IsRunning = true

	go server.run()
	go server.sync()

	logger.Info(fmt.Sprintf("RTSP server started at %s", rtspAddress))

	return err
}

func (server *RtspServer) Stop() error {
	server.IsRunning = false

	err := server.Server.Stop()
	if err != nil {
		return nil
	}
	err = server.RtpClient.Disconnect()

	return err
}

func (server *RtspServer) sync() {
	var block sync.Mutex

	for server.IsRunning {
		for _, tcpClient := range server.Server.Clients {
			block.Lock()
			currClient, alreadyMapped := server.Clients[tcpClient.Id]
			block.Unlock()
			if !alreadyMapped {
				client := rtspClient.Create()
				//TODO Добавить метод createFromTcpConnection для rtspClient
				client.TcpClient = tcpClient
				client.IsConnected = true
				server.Clients[tcpClient.Id] = &client

				logger.Info(fmt.Sprintf("RTSP client #%d connected", client.SessionId))
			} else if !currClient.IsConnected {
				delete(server.Clients, tcpClient.Id)
				logger.Debug(fmt.Sprintf("Cleaned up #%d rtsp client from server clients", currClient.SessionId))
				logger.Debug(fmt.Sprintf("Current number of clients:%d", len(server.Clients)))
			}
		}
		time.Sleep(time.Millisecond * 5)
	}
}

func (server *RtspServer) run() {
	for server.IsRunning {
		for _, client := range server.Clients {
			select {
			case bytes := <-client.TcpClient.RecvBuff:
				message := string(bytes)
				if message != "" {
					response, err := parseRequest(server, client, message)
					if err != nil {
						logger.Error(err.Error())
					}
					if response != "" {
						_, err = client.TcpClient.Send(response)
						if err != nil {
							logger.Error(err.Error())
						}
					}
				}
			default:
				continue
			}
		}
		time.Sleep(time.Millisecond * 5)
	}
}

func parseRequest(server *RtspServer, client *rtspClient.RtspClient, message string) (response string, err error) {
	responseLines := strings.Split(message, "\r\n")

	methodExp, err := regexp.Compile("^\\w+")

	if err != nil {
		return "", err
	}

	method := strings.ToLower(methodExp.FindString(responseLines[0]))

	if method == "" {
		return "", errors.New("некорректный метод запроса")
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
			"Content-Length: 0\r\n\r\n"
	}

	if method == "setup" {
		transportExp, err := regexp.Compile("(\\d+)-(\\d+)")
		if err != nil {
			return "", nil
		}
		portMatches := transportExp.FindStringSubmatch(message)
		clientRtpPortLeftInt64, err := strconv.ParseInt(portMatches[1], 10, 64)
		clientRtpPortRightInt64, err := strconv.ParseInt(portMatches[2], 10, 64)

		clientRtpPortLeftInt := int(clientRtpPortLeftInt64)
		clientRtpPortRightInt := int(clientRtpPortRightInt64)

		session := client.SessionId

		err = client.RtpClient.Connect(client.TcpClient.Ip, clientRtpPortLeftInt)
		serverRtpPort := client.RtpClient.LocalPort

		response = "RTSP/1.0 200 OK\r\n" +
			fmt.Sprintf("CSeq: %d\r\n", client.CSeq) +
			fmt.Sprintf("Session: %d;timeout=60\r\n", session) +
			fmt.Sprintf("Transport: RTP/AVP/UDP;unicast;client_port=%d-%d;server_port=%d-%d;ssrc=60d45a65;mode=\"play\"\r\n", clientRtpPortLeftInt, clientRtpPortRightInt, serverRtpPort, serverRtpPort+1) +
			"Content-Length: 0\r\n\r\n"
	}

	if method == "play" {
		response = "RTSP/1.0 200 OK\r\n" +
			fmt.Sprintf("CSeq: %d\r\n", client.CSeq) +
			fmt.Sprintf("Session:%d\r\n", client.SessionId) +
			fmt.Sprintf("RTP-Info: url=%s;seq=4563;rtptime=1435052840\r\n", server.StreamAddress) +
			fmt.Sprintf("Date: %s\r\n", nowFormatted) +
			"Content-Length: 0\r\n\r\n"
	}

	if method == "teardown" {
		response = "RTSP/1.0 200 OK\r\n" +
			fmt.Sprintf("CSeq: %d\r\n", client.CSeq) +
			fmt.Sprintf("Session:%d\r\n", client.SessionId) +
			fmt.Sprintf("%s\r\n", nowFormatted)
	}

	client.CSeq++

	return response, err
}
