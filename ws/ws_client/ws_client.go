package ws_client

import (
	"fmt"
	"github.com/gorilla/websocket"
	"math/rand"
	"strconv"
	"strings"
	"vrt/logger"
)

type WSClientCallback func(client *WSClient)

type WSClient struct {
	SessionId    int64
	Ip           string
	Port         int
	Socket       *websocket.Conn
	IsConnected  bool
	RecvBuff     chan []byte
	onDisconnect WSClientCallback
}

func Create() *WSClient {
	client := &WSClient{SessionId: rand.Int63()}

	return client
}

func CreateFromConnection(conn *websocket.Conn) *WSClient {
	client := Create()
	client.Socket = conn
	client.IsConnected = true

	address := conn.RemoteAddr().String()
	addressArray := strings.Split(address, ":")
	assignedIpString := addressArray[0]
	assignedPortString := addressArray[1]
	assignedPortInt64, _ := strconv.ParseInt(assignedPortString, 10, 64)

	assignedPortInt := int(assignedPortInt64)

	client.Ip = assignedIpString
	client.Port = assignedPortInt

	logger.Debug(fmt.Sprintf("WS client #%d: Connected from %s:%d", client.SessionId, assignedIpString, assignedPortInt))

	go run(client)

	return client
}

func (client *WSClient) Send(messageType int, message []byte) error {
	err := client.Socket.WriteMessage(messageType, message)
	logger.Junk(fmt.Sprintf("Sent %d bytes to WS client #%d %s:%d", len(message), client.SessionId, client.Ip, client.Port))
	return err
}

func (client *WSClient) SetCallback(callback WSClientCallback) {
	client.onDisconnect = callback
}

func (client *WSClient) Disconnect() error {
	client.IsConnected = false
	err := client.Socket.Close()
	client.onDisconnect(client)

	logger.Debug(fmt.Sprintf("WS client #%d: Disconnected ", client.SessionId))

	return err
}

func run(client *WSClient) {
	for client.IsConnected {
		_, message, err := client.Socket.ReadMessage()
		if err != nil {
			logger.Error(err.Error())
			err := client.Disconnect()
			if err != nil {
				logger.Error(err.Error())
			}
		}
		client.RecvBuff <- message
	}
}
