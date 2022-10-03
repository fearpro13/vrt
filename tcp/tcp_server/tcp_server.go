package tcp_server

import (
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"vrt/logger"
	"vrt/tcp/tcp_client"
)

type onConnectCallback func(client *tcp_client.TcpClient)

type TcpServer struct {
	SessionId int32
	Ip        string
	Port      int
	Socket    *net.TCPListener
	sync.Mutex
	Clients            map[int32]*tcp_client.TcpClient
	OnConnectListeners map[int32]onConnectCallback
	IsRunning          bool
}

func Create() (server *TcpServer) {
	id := rand.Int31()
	server = &TcpServer{
		SessionId:          id,
		Clients:            map[int32]*tcp_client.TcpClient{},
		OnConnectListeners: map[int32]onConnectCallback{},
	}

	return server
}

func (server *TcpServer) Start(ip string, port int) error {
	ipPtr := net.TCPAddr{IP: net.ParseIP(ip), Port: port}
	socket, err := net.ListenTCP("tcp4", &ipPtr)

	if err != nil {
		return err
	}

	tcpAddress := socket.Addr().String()
	tcpAddressArray := strings.Split(tcpAddress, ":")
	assignedIpString := tcpAddressArray[0]
	assignedPortString := tcpAddressArray[1]
	assignedPortInt64, err := strconv.ParseInt(assignedPortString, 10, 64)
	assignedPortInt := int(assignedPortInt64)

	server.Ip = assignedIpString
	server.Port = assignedPortInt
	server.Socket = socket
	server.IsRunning = true

	go server.run()

	logger.Debug(fmt.Sprintf("TCP server #%d started on %s:%d", server.SessionId, assignedIpString, assignedPortInt))

	return nil
}

func (server *TcpServer) Stop() (err error) {
	server.IsRunning = false

	for _, client := range server.Clients {
		if client.IsConnected {
			_ = client.Disconnect()
		}
	}

	err = server.Socket.Close()

	if err != nil {
		return err
	}

	logger.Debug(fmt.Sprintf("TCP server #%d stopped", server.SessionId))

	return nil
}

func (server *TcpServer) run() {
	for server.IsRunning {
		connection, err := server.Socket.AcceptTCP()
		if err != nil {
			logger.Error(err.Error())
			return
		}

		client, err := tcp_client.CreateFromConnection(connection)
		server.Lock()
		server.Clients[client.SessionId] = client
		server.Unlock()

		client.OnDisconnectListeners[server.SessionId] = func(client *tcp_client.TcpClient) {
			server.Lock()
			delete(server.Clients, client.SessionId)
			server.Unlock()
		}

		for _, listener := range server.OnConnectListeners {
			go listener(client)
		}
	}
}
