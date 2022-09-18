package udp_client

import (
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"vrt/logger"
)

type UdpClient struct {
	Id          int64
	Ip          string
	Port        int
	LocalIp     string
	LocalPort   int
	Socket      *net.UDPConn
	IsConnected bool
}

func Create() UdpClient {
	id := rand.Int63()
	logger.Junk(fmt.Sprintf("UDP client #%d created", id))
	return UdpClient{Id: id}
}

func (client *UdpClient) Connect(ip string, port int) error {
	udpAddress := net.UDPAddr{IP: net.ParseIP(ip), Port: port}
	localUdpAddress := net.UDPAddr{IP: net.ParseIP(client.LocalIp), Port: client.LocalPort}
	connection, err := net.DialUDP("udp4", &localUdpAddress, &udpAddress)

	if err != nil {
		return nil
	}

	localUdpAddressAssigned := (*connection).LocalAddr().String()
	localUdpAddressAssignedArray := strings.Split(localUdpAddressAssigned, ":")
	localUdpAddressAssignedIpString := localUdpAddressAssignedArray[0]
	localUdpAddressAssignedPortString := localUdpAddressAssignedArray[1]
	localUdpAddressAssignedPortInt64, err := strconv.ParseInt(localUdpAddressAssignedPortString, 10, 64)
	if err != nil {
		return err
	}
	localUdpAddressAssignedPortInt := int(localUdpAddressAssignedPortInt64)

	client.LocalIp = localUdpAddressAssignedIpString
	client.LocalPort = localUdpAddressAssignedPortInt
	client.Ip = ip
	client.Port = port
	client.Socket = connection
	client.IsConnected = true

	logger.Debug(fmt.Sprintf("UDP client #%d: Connected to %s:%d", client.Id, ip, port))

	return err
}

func (client *UdpClient) Send(bytes []byte) error {
	bytesWritten, err := client.Socket.Write(bytes)
	logger.Junk(fmt.Sprintf("UDP client #%d sent %d bytes to %s:%d", client.Id, bytesWritten, client.Ip, client.Port))
	return err
}

func (client *UdpClient) SendMessage(message string) error {
	bytesWritten, err := client.Socket.Write([]byte(message))
	logger.Junk(fmt.Sprintf("UDP client #%d sent %d bytes to %s:%d", client.Id, bytesWritten, client.Ip, client.Port))
	return err
}

func (client *UdpClient) Read() (bytes []byte, err error) {
	buff := make([]byte, 2048)
	bytesRead, err := client.Socket.Read(buff)

	logger.Junk(fmt.Sprintf("UDP client #%d: Received %d bytes from %s:%d", client.Id, bytesRead, client.Ip, client.Port))

	return buff, err
}

func (client *UdpClient) ReadMessage() (message string, err error) {
	buff := make([]byte, 2048)

	var bytesReadTotal, bytesReadCurrent int
	var buffTotal []byte
	var endReached = false

	for !endReached {
		bytesReadCurrent, err = client.Socket.Read(buff)
		bytesReadTotal += bytesReadCurrent
		buffTotal = append(buffTotal, buff...)

		endReached = bytesReadCurrent == 0
		buff = buff[:0]
	}

	message = string(buffTotal)

	logger.Junk(fmt.Sprintf("UDP client #%d: Received %d bytes from %s:%d", client.Id, bytesReadTotal, client.Ip, client.Port))
	return message, err
}

func (client *UdpClient) Disconnect() error {
	client.IsConnected = false

	err := client.Socket.Close()
	logger.Debug(fmt.Sprintf("UDP client #%d disconnected", client.Id))
	return err
}
