package tcp_client

import (
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"vrt/logger"
)

type onDisconnectCallback func(client *TcpClient)

type TcpClient struct {
	SessionId    int64
	Ip           string
	Port         int
	Socket       *net.TCPConn
	IO           *bufio.ReadWriter
	IsConnected  bool
	OnDisconnect onDisconnectCallback
	//RecvBuff    chan []byte
}

func Create() *TcpClient {
	id := rand.Int63()
	client := &TcpClient{
		SessionId: id,
		//RecvBuff: make(chan []byte, 65535),
	}

	//logger.Junk(fmt.Sprintf("TCP client #%d Created", id))

	return client
}

func CreateFromConnection(connection *net.TCPConn) (client *TcpClient, err error) {
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
	client.IO = bufio.NewReadWriter(bufio.NewReader(connection), bufio.NewWriter(connection))
	client.IsConnected = true

	//go client.run()

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
	client.IO = bufio.NewReadWriter(bufio.NewReader(connection), bufio.NewWriter(connection))
	client.IsConnected = true

	logger.Debug(fmt.Sprintf("TCP client #%d: Connected to %s:%d", client.SessionId, ip, port))

	//go client.run()

	return err
}

func (client *TcpClient) Send(message string) (bytesWritten int, err error) {
	bytesWritten, err = client.IO.WriteString(message)
	if err != nil {
		if err == io.EOF {
			err = client.Disconnect()
		}
		return 0, err
	}

	err = client.IO.Flush()
	if err != nil {
		return 0, err
	}

	logger.Junk(fmt.Sprintf("TCP client #%d: Sent %d bytes to %s:%d", client.SessionId, bytesWritten, client.Ip, client.Port))
	logger.Junk(message)

	return bytesWritten, err
}

func (client *TcpClient) ReadBytes(bytesToRead int) (bytes []byte, bytesRead int, err error) {
	if bytesToRead == 0 {
		return []byte{}, 0, nil
	}

	bytes = make([]byte, bytesToRead)

	bytesRead, err = client.IO.Read(bytes)
	if err != nil {
		if err == io.EOF {
			err = client.Disconnect()
		}
		return []byte{}, 0, nil
	}

	if bytesRead == 0 {
		return []byte{}, 0, nil
	}

	bytes = bytes[:bytesRead]

	logger.Junk(fmt.Sprintf("TCP client #%d: Received %d bytes from %s:%d", client.SessionId, bytesRead, client.Ip, client.Port))

	return bytes, bytesRead, err
}

func (client *TcpClient) ReadLine() (message string, err error) {
	messageBytes, _, err := client.IO.ReadLine()

	if err != nil {
		if err == io.EOF {
			err = client.Disconnect()
		}
		return "", err
	}

	message = string(messageBytes)

	logger.Junk(fmt.Sprintf("TCP client #%d: Received %d bytes from %s:%d", client.SessionId, len(messageBytes), client.Ip, client.Port))

	return message, err
}

//DEPRECATED To read message you have to know protocol specifics and protocol message ending

//func (client *TcpClient) ReadMessage() (message string, err error) {
//	bytes := make([]byte, 65535)
//
//	var bytesReadTotal, bytesReadCurrent int
//	var bytesTotal []byte
//	var endReached = false
//
//	for !endReached {
//		bytesReadCurrent, err = client.Socket.Read(bytes)
//		bytesReadTotal += bytesReadCurrent
//		bytesTotal = append(bytesTotal, bytes...)
//
//		endReached = bytesReadCurrent == 0
//		bytes = bytes[:0]
//	}
//
//	if bytesReadTotal == 0 {
//		return "", nil
//	}
//
//	message = string(bytesTotal)
//
//	logger.Junk(fmt.Sprintf("TCP client #%d: Received %d bytes from %s:%d", client.SessionId, bytesReadTotal, client.Ip, client.Port))
//	logger.Junk(message)
//
//	return message, err
//}

func (client *TcpClient) Disconnect() error {
	client.IsConnected = false

	logger.Debug(fmt.Sprintf("TCP client #%d: Closed connection to %s:%d", client.SessionId, client.Ip, client.Port))

	err := client.Socket.Close()

	if client.OnDisconnect != nil {
		client.OnDisconnect(client)
	}

	return err
}

//
//func (client *TcpClient) run() {
//	for client.IsConnected {
//		bytes, _, err := client.ReadBytes()
//		if err != nil {
//			logger.Error(err.Error())
//		}
//		client.RecvBuff <- bytes
//	}
//}
