package ws_server

import (
	"fmt"
	"github.com/gorilla/websocket"
	"math/rand"
	"net/http"
	"sync"
	"vrt/logger"
	"vrt/tcp/tcp_server"
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
	HttpTcpServer      *tcp_server.TcpServer
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

	server.HttpServer.Addr = fmt.Sprintf("%s:%d", ip, port)

	tcpServer := tcp_server.Create()
	err := tcpServer.Start("", port)
	if err != nil {
		return err
	}
	server.HttpTcpServer = tcpServer

	go server.HttpServer.Serve(tcpServer.Socket)

	server.IsRunning = true

	logger.Info(fmt.Sprintf("WS server #%d started on %s:%d", server.SessionId, tcpServer.Ip, tcpServer.Port))

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
