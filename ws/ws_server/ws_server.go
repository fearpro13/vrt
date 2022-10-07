package ws_server

import (
	"fmt"
	"github.com/gorilla/websocket"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"vrt/logger"
	"vrt/ws/ws_client"
)

type WSServer struct {
	SessionId int32
	Ip        string
	Port      int
	handler   *http.Server
	sync.Mutex
	Clients            map[int32]*ws_client.WSClient
	IsRunning          bool
	OnConnectListeners map[int32]ws_client.WSClientCallback
	HttpServer         *http.Server
	HttpHandler        *http.ServeMux
	HttpTcpHandler     *net.TCPListener
}

func Create() *WSServer {
	mux := http.NewServeMux()
	httpServer := &http.Server{Handler: mux}
	server := &WSServer{
		SessionId:          rand.Int31(),
		Clients:            map[int32]*ws_client.WSClient{},
		HttpHandler:        mux,
		HttpServer:         httpServer,
		OnConnectListeners: map[int32]ws_client.WSClientCallback{},
	}

	return server
}

func (server *WSServer) Start(path string, ip string, port int) error {
	if path != "" {
		server.HttpHandler.HandleFunc(path, server.UpgradeToWebsocket)
	}

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

	server.HttpTcpHandler = socket

	//server.HttpServer.Addr = fmt.Sprintf("%s:%d", ip, port)
	go server.HttpServer.Serve(socket)

	server.IsRunning = true

	logger.Info(fmt.Sprintf("WS server #%d started on %s:%d", server.SessionId, server.Ip, server.Port))

	return nil
}

func (server *WSServer) Stop() error {
	server.IsRunning = false

	for _, client := range server.Clients {
		_ = client.Disconnect()
	}

	err := server.handler.Close()
	logger.Info(fmt.Sprintf("WS #%d stopped", server.SessionId))

	return err
}

var upgrader = websocket.Upgrader{
	HandshakeTimeout: 0,
	ReadBufferSize:   0,
	WriteBufferSize:  0,
	WriteBufferPool:  nil,
	Subprotocols:     nil,
	Error:            nil,
	CheckOrigin: func(r *http.Request) bool {
		//TODO Убрать это решение из продакшен кода, использовать только для локальной разработки
		//Пропускает соединения ws с любого хоста ( в браузере)
		return true
	},
	EnableCompression: false,
}

func (server *WSServer) UpgradeToWebsocket(w http.ResponseWriter, r *http.Request) {
	connection, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error(err.Error())
		return
	}

	wsClient := ws_client.CreateFromConnection(connection)

	server.Lock()
	server.Clients[wsClient.SessionId] = wsClient
	server.Unlock()

	wsClient.OnDisconnectListeners[server.SessionId] = func(client *ws_client.WSClient, relativeURLPath string) {
		server.Lock()
		delete(server.Clients, wsClient.SessionId)
		server.Unlock()
	}

	for _, listener := range server.OnConnectListeners {
		go listener(wsClient, r.URL.Path)
	}
}
