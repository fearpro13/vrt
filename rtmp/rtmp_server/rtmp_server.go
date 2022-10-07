package rtmp_server

import (
	"fmt"
	"math/rand"
	"sync"
	"vrt/logger"
	"vrt/rtmp/rtmp_client"
	"vrt/tcp/tcp_client"
	"vrt/tcp/tcp_server"
)

type OnConnectCallback func(client *rtmp_client.RtmpClient)

type RtmpServer struct {
	SessionId  int32
	Login      string
	Password   string
	TcpHandler *tcp_server.TcpServer

	RtmpAddress string
	//VideoStreamAddress string
	//AudioStreamAddress string

	//VideoId int
	//AudioId int

	//SdpRaw string

	//RtpVideoClient *udp_client.UdpClient
	//RtpAudioClient *udp_client.UdpClient
	//RtpServer      *udp_server.UdpServer

	//RtpLocalPort  int
	//RtcpServer    *udp_server.UdpServer
	//RtcpLocalPort int

	sync.Mutex
	Clients                  map[int32]*rtmp_client.RtmpClient
	OnClientConnectListeners map[int32]OnConnectCallback
	IsRunning                bool
}

func Create() *RtmpServer {
	server := &RtmpServer{
		SessionId:                rand.Int31(),
		Clients:                  map[int32]*rtmp_client.RtmpClient{},
		OnClientConnectListeners: map[int32]OnConnectCallback{},
	}

	return server
}

func (rtmpServer *RtmpServer) Start(ip string, port int) error {
	tcpHandler := tcp_server.Create()

	rtmpServer.TcpHandler = tcpHandler

	err := tcpHandler.Start(ip, port)
	if err != nil {
		return err
	}

	tcpHandler.OnConnectListeners[rtmpServer.SessionId] = func(client *tcp_client.TcpClient) {
		rtmpClient := rtmp_client.CreateFromConnection(client)

		rtmpServer.Lock()
		rtmpServer.Clients[rtmpClient.SessionId] = rtmpClient
		rtmpServer.Unlock()

		rtmpClient.OnDisconnectListeners[rtmpServer.SessionId] = func(client *rtmp_client.RtmpClient) {
			rtmpServer.Lock()
			delete(rtmpServer.Clients, rtmpClient.SessionId)
			rtmpServer.Unlock()
		}
	}

	rtmpServer.IsRunning = true

	logger.Info(fmt.Sprintf("RTMP server #%d: started at %s:%d", rtmpServer.SessionId, tcpHandler.Ip, tcpHandler.Port))

	return nil
}

func (rtmpServer *RtmpServer) Stop() error {
	rtmpServer.IsRunning = false

	for _, client := range rtmpServer.Clients {
		client.Disconnect()
	}

	err := rtmpServer.TcpHandler.Stop()

	logger.Info(fmt.Sprintf("RTMP server #%d stop", rtmpServer.SessionId))

	return err
}

func (rtmpServer *RtmpServer) run() {
}
