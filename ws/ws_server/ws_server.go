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
	SessionId int64
	Ip        string
	Port      int
	handler   *http.Server
	sync.Mutex
	Clients   map[int64]*ws_client.WSClient
	IsRunning bool
	Callback  ws_client.WSClientCallback
}

func Create() *WSServer {
	server := &WSServer{SessionId: rand.Int63(), Clients: map[int64]*ws_client.WSClient{}}

	return server
}

func (server *WSServer) Start(ip string, port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", server.entryPointHandler)
	httpServer := &http.Server{Addr: fmt.Sprintf("%s:%d", ip, port), Handler: mux}
	go httpServer.ListenAndServe()

	server.IsRunning = true

	logger.Info(fmt.Sprintf("WS server #%d started on %s", server.SessionId, httpServer.Addr))

	go server.sync()

	return nil
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

func (server *WSServer) entryPointHandler(w http.ResponseWriter, r *http.Request) {
	connection, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Error(err.Error())
		return
	}

	wsClient := ws_client.CreateFromConnection(connection)

	server.Lock()
	server.Clients[wsClient.SessionId] = wsClient
	server.Unlock()

	if server.Callback != nil {
		server.Callback(wsClient)
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
