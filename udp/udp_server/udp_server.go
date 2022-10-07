package udp_server

import (
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"vrt/logger"
)

type UdpServer struct {
	Id        int64
	Ip        string
	Port      int
	Socket    *net.UDPConn
	IsRunning bool
	RecvBuff  chan []byte
}

func Create() *UdpServer {
	id := rand.Int63()
	udpServer := &UdpServer{
		Id:        id,
		IsRunning: false,
		RecvBuff:  make(chan []byte, 1024),
	}

	return udpServer
}

func (server *UdpServer) Start(ip string, port int) error {
	udpAddress := net.UDPAddr{IP: net.ParseIP(ip), Port: port}
	conn, err := net.ListenUDP("udp4", &udpAddress)

	if err != nil {
		return err
	}

	assignedAddress := strings.Split(conn.LocalAddr().String(), ":")
	assignedIpString := assignedAddress[0]

	assignedPortString := assignedAddress[1]
	assignedPortInt64, err := strconv.ParseInt(assignedPortString, 10, 32)
	if err != nil {
		return err
	}
	assignedPortInt := int(assignedPortInt64)

	server.Ip = assignedIpString
	server.Port = assignedPortInt
	server.Socket = conn
	server.IsRunning = true

	logger.Debug(fmt.Sprintf("Udp server #%d started at %s:%d", server.Id, assignedIpString, assignedPortInt))

	go server.run()

	return err
}

func (server *UdpServer) Stop() error {
	err := server.Socket.Close()
	if err != nil {
		return err
	}

	logger.Debug(fmt.Sprintf("UDP server #%d stopped", server.Id))

	server.IsRunning = false
	server.Ip = ""
	server.Port = 0
	server.Id = 0

	return err
}

func (server *UdpServer) run() {
	var buff []byte

	for server.IsRunning {
		buff = make([]byte, 2048)
		bytesRead, addr, err := server.Socket.ReadFromUDP(buff)
		buff = buff[:bytesRead]
		if err != nil {
			server.IsRunning = false
			logger.Error(err.Error())
			return
		}
		ip := addr.IP.String()
		port := addr.Port
		logger.Junk(fmt.Sprintf("UDP server #%d received %d bytes from %s:%d", server.Id, bytesRead, ip, port))

		server.RecvBuff <- buff

		buff = buff[:0]
	}

	return
}
