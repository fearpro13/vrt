package ws_server

import (
	"fmt"
	"github.com/gorilla/websocket"
	"math/rand"
	"net/http"
	"sync"
	"time"
	"vrt/logger"
	"vrt/ws/ws_client"
)

type WSServer struct {
	SessionId int32
	Ip        string
	Port      int
	handler   *http.Server
	sync.Mutex
	Clients     map[int32]*ws_client.WSClient
	IsRunning   bool
	Callback    ws_client.WSClientCallback // DEPRECATED, USE Listeners instead
	Listeners   map[int32]ws_client.WSClientCallback
	HttpServer  *http.Server
	HttpHandler *http.ServeMux
}

func Create() *WSServer {
	mux := http.NewServeMux()
	httpServer := &http.Server{Handler: mux}
	server := &WSServer{
		SessionId:   rand.Int31(),
		Clients:     map[int32]*ws_client.WSClient{},
		HttpHandler: mux,
		HttpServer:  httpServer,
		Listeners:   map[int32]ws_client.WSClientCallback{},
	}

	return server
}

func (server *WSServer) Start(path string, ip string, port int) error {
	server.HttpHandler.HandleFunc(path, server.UpgradeToWebsocket)
	server.HttpServer.Addr = fmt.Sprintf("%s:%d", ip, port)
	go server.HttpServer.ListenAndServe()

	server.IsRunning = true

	logger.Info(fmt.Sprintf("WS server #%d started on %s", server.SessionId, server.HttpServer.Addr))

	go server.sync()

	return nil
}

func (server *WSServer) AddRoute(route string, handlerFunc http.HandlerFunc) {
	server.HttpHandler.HandleFunc(route, handlerFunc)
}

func (server *WSServer) Stop() error {
	server.IsRunning = false

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

	for _, listener := range server.Listeners {
		if listener != nil {
			go listener(wsClient, r.URL.Path)
		}
	}

	if server.Callback != nil {
		server.Callback(wsClient, r.URL.Path)
	}
}

func (server *WSServer) sync() {
	for server.IsRunning {
		for _, client := range server.Clients {
			if !client.IsConnected {
				server.Lock()
				delete(server.Clients, client.SessionId)
				server.Unlock()
				logger.Debug(fmt.Sprintf("Cleaned up #%d WS client from WS server", client.SessionId))
				logger.Debug(fmt.Sprintf("Current number of WS server clients:%d", len(server.Clients)))
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
}
