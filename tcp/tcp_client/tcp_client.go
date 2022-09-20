package tcp_client

import (
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"vrt/logger"
)

type TcpClient struct {
	Id          int64
	Ip          string
	Port        int
	Socket      *net.TCPConn
	IsConnected bool
	RecvBuff    chan []byte
}

func Create() TcpClient {
	id := rand.Int63()
	logger.Junk(fmt.Sprintf("TCP client #%d Created", id))
	return TcpClient{Id: id, RecvBuff: make(chan []byte, 65535)}
}

func CreateFromConnection(connection *net.TCPConn) (client TcpClient, err error) {
	client = Create()

	tcpAddress := (*connection).RemoteAddr().String()
	tcpAddressArray := strings.Split(tcpAddress, ":")
	assignedIpString := tcpAddressArray[0]
	assignedPortString := tcpAddressArray[1]
	assignedPortInt64, err := strconv.ParseInt(assignedPortString, 10, 64)
	if err != nil {
		return client, err
	}
	assignedPortInt := int(assignedPortInt64)

	client.Ip = assignedIpString
	client.Port = assignedPortInt
	client.Socket = connection
	client.IsConnected = true

	go client.run()

	return client, err
}

func (client *TcpClient) Connect(ip string, port int) error {
	tcpAddress := net.TCPAddr{IP: net.ParseIP(ip), Port: port}
	connection, err := net.DialTCP("tcp4", nil, &tcpAddress)

	if err != nil {
		return nil
	}

	client.Ip = ip
	client.Port = port
	client.Socket = connection
	client.IsConnected = true

	logger.Debug(fmt.Sprintf("TCP client #%d: Connected to %s:%d", client.Id, ip, port))

	go client.run()

	return err
}

func (client *TcpClient) Send(message string) (bytesWritten int, err error) {
	bytesWritten, err = client.Socket.Write([]byte(message))
	logger.Junk(fmt.Sprintf("TCP client #%d: Sent %d bytes to %s:%d", client.Id, bytesWritten, client.Ip, client.Port))
	logger.Junk(message)
	return bytesWritten, err
}

func (client *TcpClient) ReadBytes(bytesToRead int) (bytes []byte, bytesRead int, err error) {
	bytes = make([]byte, bytesRead)
	bytesRead, err = client.Socket.Read(bytes)

	if bytesRead == 0 {
		return []byte{}, 0, nil
	}

	bytes = bytes[:bytesRead]

	logger.Junk(fmt.Sprintf("TCP client #%d: Received %d bytes from %s:%d", client.Id, bytesRead, client.Ip, client.Port))

	return bytes, bytesRead, err
}

func (client *TcpClient) ReadMessage() (message string, err error) {
	bytes := make([]byte, 65535)

	var bytesReadTotal, bytesReadCurrent int
	var bytesTotal []byte
	var endReached = false

	for !endReached {
		bytesReadCurrent, err = client.Socket.Read(bytes)
		bytesReadTotal += bytesReadCurrent
		bytesTotal = append(bytesTotal, bytes...)

		endReached = bytesReadCurrent == 0
		bytes = bytes[:0]
	}

	if bytesReadTotal == 0 {
		return "", nil
	}

	message = string(bytesTotal)

	logger.Junk(fmt.Sprintf("TCP client #%d: Received %d bytes from %s:%d", client.Id, bytesReadTotal, client.Ip, client.Port))
	logger.Junk(message)

	return message, err
}

func (client *TcpClient) Disconnect() error {
	logger.Debug(fmt.Sprintf("TCP client #%d: Closed connection to %s:%d", client.Id, client.Ip, client.Port))
	return client.Socket.Close()
}

func (client *TcpClient) run() {
	//for client.IsConnected {
	//	bytes, _, err := client.ReadBytes()
	//	if err != nil {
	//		logger.Error(err.Error())
	//	}
	//	client.RecvBuff <- bytes
	//}
}
