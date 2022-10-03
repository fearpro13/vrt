package ws_client

import (
	"fmt"
	"github.com/gorilla/websocket"
	"math/rand"
	"strconv"
	"strings"
	"vrt/logger"
)

type WSClientCallback func(client *WSClient, relativeURLPath string)

type WSClient struct {
	SessionId             int32
	Ip                    string
	Port                  int
	Socket                *websocket.Conn
	IsConnected           bool
	RecvBuff              chan []byte
	OnDisconnectListeners map[int32]WSClientCallback
}

func Create() *WSClient {
	buff := make(chan []byte, 2048)
	client := &WSClient{
		SessionId:             rand.Int31(),
		RecvBuff:              buff,
		OnDisconnectListeners: map[int32]WSClientCallback{},
	}

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

	logger.Info(fmt.Sprintf("WS client #%d: Connected from %s:%d", client.SessionId, assignedIpString, assignedPortInt))

	return client
}

func (client *WSClient) Send(messageType int, message []byte) error {
	err := client.Socket.WriteMessage(messageType, message)
	if err != nil {
		_ = client.Disconnect()
		return err
	}

	logger.Junk(fmt.Sprintf("Sent %d bytes to WS client #%d %s:%d", len(message), client.SessionId, client.Ip, client.Port))

	return err
}

func (client *WSClient) Disconnect() error {
	if !client.IsConnected {
		return nil
	}

	client.IsConnected = false
	err := client.Socket.Close()

	for _, listener := range client.OnDisconnectListeners {
		go listener(client, "")
	}

	logger.Info(fmt.Sprintf("WS client #%d: Disconnected ", client.SessionId))

	return err
}

func (client *WSClient) run() {
	for client.IsConnected {
		_, message, err := client.Socket.ReadMessage()
		if err != nil {
			_ = client.Disconnect()
			return
		}
		client.RecvBuff <- message
	}
}
