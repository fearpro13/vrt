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
	Id        int64
	Ip        string
	Port      int
	Socket    *net.TCPListener
	Clients   []*tcp_client.TcpClient
	OnConnect onConnectCallback
	IsRunning bool
}

func Create() (server TcpServer) {
	id := rand.Int63()
	server = TcpServer{Id: id}

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
	server.Clients = []*tcp_client.TcpClient{}
	server.IsRunning = true

	go server.run()

	logger.Debug(fmt.Sprintf("TCP server #%d started on %s:%d", server.Id, assignedIpString, assignedPortInt))

	return nil
}

func (server *TcpServer) Stop() (err error) {
	server.IsRunning = false
	err = server.Socket.Close()

	if err != nil {
		return err
	}

	logger.Debug(fmt.Sprintf("TCP server #%d stopped", server.Id))

	return nil
}

func (server *TcpServer) Restart() (err error) {
	return nil
}

func (server *TcpServer) run() {
	var block sync.Mutex
	for server.IsRunning {
		connection, err := server.Socket.AcceptTCP()
		if err != nil {
			logger.Error(err.Error())
			return
		}

		client, err := tcp_client.CreateFromConnection(connection)
		block.Lock()
		server.Clients = append(server.Clients, client)
		block.Unlock()

		if server.OnConnect != nil {
			server.OnConnect(client)
		}
	}
}
