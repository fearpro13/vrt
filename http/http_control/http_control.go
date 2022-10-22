package http_control

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
	"vrt/logger"
	"vrt/rtsp/rtsp_client"
	"vrt/rtsp/rtsp_proxy"
	"vrt/rtsp_to_ws"
	"vrt/ws/ws_server"
)

type HttpControlServer struct {
	RtspProxies              map[int32]*rtsp_proxy.RtspProxy
	WsBroadcasts             map[int32]*rtsp_to_ws.Broadcast
	WsServer                 *ws_server.WSServer
	routes                   []string
	Signals                  chan os.Signal
	proxyByRemoteAddress     map[string]*rtsp_proxy.RtspProxy
	broadcastByRemoteAddress map[string]*rtsp_to_ws.Broadcast
	sync.Mutex
	BroadcastTimers map[int32]*BroadcastTimer
	IsRunning       bool
}

type BroadcastTimer struct {
	Broadcast      *rtsp_to_ws.Broadcast
	ActiveTime     int
	LastActiveTime int64
	LifeTimeEnd    int64
}

type AddProxyJson struct {
	RemoteAddress string `json:"remote_address"`
	LocalPort     int    `json:"local_port"`
}

type StopProxyJson struct {
	Id int32 `json:"id"`
}

type ProxyJson struct {
	Id            int32  `json:"id"`
	RemoteAddress string `json:"remote_address"`
	LocalAddress  string `json:"local_address"`
	Clients       int    `json:"clients"`
}

type AddWsBroadcastJson struct {
	LifeTime      int    `json:"lifetime"`
	ActiveTime    int    `json:"active_time"`
	RemoteAddress string `json:"remote_address"`
}

type StopBroadcastJson struct {
	Id int32 `json:"id"`
}

type BroadcastJson struct {
	Id            int32  `json:"id"`
	RemoteAddress string `json:"remote_address"`
	LocalAddress  string `json:"local_address"`
	Clients       int    `json:"clients"`
}

func NewHttpControlServer() *HttpControlServer {
	controlServer := &HttpControlServer{
		RtspProxies:              map[int32]*rtsp_proxy.RtspProxy{},
		WsBroadcasts:             map[int32]*rtsp_to_ws.Broadcast{},
		proxyByRemoteAddress:     map[string]*rtsp_proxy.RtspProxy{},
		broadcastByRemoteAddress: map[string]*rtsp_to_ws.Broadcast{},
		WsServer:                 ws_server.Create(),
		routes:                   []string{},
		Signals:                  make(chan os.Signal, 1),
		BroadcastTimers:          map[int32]*BroadcastTimer{},
	}

	return controlServer
}

func (controlServer *HttpControlServer) Start(ip string, port int) {
	controlServer.IsRunning = true

	go func() {
		for controlServer.IsRunning {
			cTime := time.Now().Unix()
			for broadcastId, broadcastTimer := range controlServer.BroadcastTimers {
				if broadcastTimer.LifeTimeEnd != 0 && broadcastTimer.LifeTimeEnd < cTime {
					controlServer.StopWsBroadcast(broadcastId)
					continue
				}
				if broadcastTimer.ActiveTime != 0 && len(broadcastTimer.Broadcast.Clients) == 0 {
					if cTime-broadcastTimer.LastActiveTime > int64(broadcastTimer.ActiveTime) {
						controlServer.StopWsBroadcast(broadcastId)
					}
				} else {
					broadcastTimer.LastActiveTime = cTime
				}
			}
			time.Sleep(5 * time.Second)
		}
	}()

	controlServer.WsServer.HttpHandler.HandleFunc("/add_proxy", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Access-Control-Allow-Origin", "*")

		contentLen := request.ContentLength
		body := make([]byte, contentLen)
		bodyReader := request.Body
		bytesRead, err := bodyReader.Read(body)
		if contentLen != 0 && bytesRead == 0 && err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		body = body[:bytesRead]

		proxyJson := &AddProxyJson{}
		err = json.Unmarshal(body, proxyJson)
		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}

		_, proxyExist := controlServer.proxyByRemoteAddress[proxyJson.RemoteAddress]
		if proxyExist {
			writer.WriteHeader(http.StatusAlreadyReported)
			return
		}

		err = controlServer.AddProxy(proxyJson.RemoteAddress, proxyJson.LocalPort)

		if err != nil {
			responseJson := []string{err.Error()}
			jData, _ := json.Marshal(responseJson)
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write(jData)
			return
		}

		writer.WriteHeader(http.StatusOK)
	})

	controlServer.WsServer.HttpHandler.HandleFunc("/stop_proxy", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Access-Control-Allow-Origin", "*")

		contentLen := request.ContentLength
		body := make([]byte, contentLen)
		bodyReader := request.Body
		bytesRead, err := bodyReader.Read(body)
		if contentLen != 0 && bytesRead == 0 && err != nil {
			logger.Error(err.Error())
			return
		}
		body = body[:bytesRead]

		proxyJson := &StopProxyJson{}
		err = json.Unmarshal(body, proxyJson)
		if err != nil {
			logger.Error(err.Error())
			return
		}

		_, proxyExist := controlServer.RtspProxies[proxyJson.Id]
		if !proxyExist {
			writer.WriteHeader(http.StatusNotFound)
			return
		}

		err = controlServer.StopProxy(proxyJson.Id)

		if err != nil {
			responseJson := []string{"Could not stop proxy"}
			jData, _ := json.Marshal(responseJson)
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write(jData)
			return
		}

		writer.WriteHeader(http.StatusOK)
	})

	controlServer.WsServer.HttpHandler.HandleFunc("/proxy_list", func(writer http.ResponseWriter, request *http.Request) {
		responseJson := controlServer.ProxyList()

		jData, _ := json.Marshal(responseJson)
		writer.Header().Set("Content-Type", "application/json")

		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(jData)
	})

	controlServer.WsServer.HttpHandler.HandleFunc("/add_broadcast", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Access-Control-Allow-Origin", "*")

		contentLen := request.ContentLength
		body := make([]byte, contentLen)
		bodyReader := request.Body
		bytesRead, err := bodyReader.Read(body)
		if contentLen != 0 && bytesRead == 0 && err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		body = body[:bytesRead]

		broadcastJson := &AddWsBroadcastJson{}
		err = json.Unmarshal(body, broadcastJson)
		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}

		_, broadcastExist := controlServer.broadcastByRemoteAddress[broadcastJson.RemoteAddress]
		if broadcastExist {
			writer.WriteHeader(http.StatusAlreadyReported)
			return
		}

		err = controlServer.AddWsBroadcast(broadcastJson.RemoteAddress, broadcastJson.LifeTime, broadcastJson.ActiveTime)

		if err != nil {
			responseJson := []string{err.Error()}
			jData, _ := json.Marshal(responseJson)
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write(jData)
			return
		}

		writer.WriteHeader(http.StatusOK)
	})

	controlServer.WsServer.HttpHandler.HandleFunc("/stop_broadcast", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Access-Control-Allow-Origin", "*")

		contentLen := request.ContentLength
		body := make([]byte, contentLen)
		bodyReader := request.Body
		bytesRead, err := bodyReader.Read(body)
		if contentLen != 0 && bytesRead == 0 && err != nil {
			logger.Error(err.Error())
			return
		}
		body = body[:bytesRead]

		broadcastJson := &StopBroadcastJson{}
		err = json.Unmarshal(body, broadcastJson)
		if err != nil {
			logger.Error(err.Error())
			return
		}

		_, broadcastExist := controlServer.WsBroadcasts[broadcastJson.Id]
		if !broadcastExist {
			writer.WriteHeader(http.StatusNotFound)
			return
		}

		err = controlServer.StopWsBroadcast(broadcastJson.Id)

		if err != nil {
			responseJson := []string{"Could not stop broadcast"}
			jData, _ := json.Marshal(responseJson)
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write(jData)
			return
		}

		writer.WriteHeader(http.StatusOK)
	})

	controlServer.WsServer.HttpHandler.HandleFunc("/broadcast_list", func(writer http.ResponseWriter, request *http.Request) {
		responseJson := controlServer.WsBroadcastList()

		jData, _ := json.Marshal(responseJson)
		writer.Header().Set("Content-Type", "application/json")

		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(jData)
	})

	err := controlServer.WsServer.Start("", ip, port)
	if err != nil {
		logger.Error(err.Error())
	}
}

func (controlServer *HttpControlServer) Stop() {
	controlServer.Signals <- os.Interrupt
}

func (controlServer *HttpControlServer) AddProxy(remoteRtspAddress string, port int) error {
	rtspProxy := rtsp_proxy.Create()
	err := rtspProxy.ProxyFromAddress(remoteRtspAddress, port, rtsp_client.RtspTransportTcp)
	if err != nil {
		return err
	}

	controlServer.RtspProxies[rtspProxy.SessionId] = rtspProxy
	controlServer.proxyByRemoteAddress[remoteRtspAddress] = rtspProxy

	rtspProxy.OnStopListeners[0] = func(proxy *rtsp_proxy.RtspProxy) {
		delete(controlServer.RtspProxies, proxy.SessionId)
		delete(controlServer.proxyByRemoteAddress, proxy.RtspClient.RemoteAddress)
	}

	return err
}

func (controlServer *HttpControlServer) StopProxy(proxyId int32) error {
	proxy, exist := controlServer.RtspProxies[proxyId]
	if !exist {
		return errors.New("proxy server with does not exist")
	}
	err := proxy.Stop()
	delete(controlServer.RtspProxies, proxyId)
	delete(controlServer.proxyByRemoteAddress, proxy.RtspClient.RemoteAddress)

	return err
}

func (controlServer *HttpControlServer) ProxyList() []*ProxyJson {
	proxies := []*ProxyJson{}

	for _, proxy := range controlServer.RtspProxies {
		proxyJson := &ProxyJson{}
		proxyJson.Id = proxy.SessionId
		proxyJson.Clients = len(proxy.RtspServer.Clients)
		proxyJson.RemoteAddress = proxy.RtspClient.RemoteAddress
		proxyJson.LocalAddress = proxy.RtspServer.RtspAddress

		proxies = append(proxies, proxyJson)
	}

	return proxies
}

func (controlServer *HttpControlServer) AddWsBroadcast(remoteRtspAddress string, lifeTime int, activeTime int) error {
	broadcast := rtsp_to_ws.NewBroadcast()

	var rtspClient *rtsp_client.RtspClient
	proxy, proxyExist := controlServer.proxyByRemoteAddress[remoteRtspAddress]
	if proxyExist {
		rtspClient = proxy.RtspClient
		proxy.OnStopListeners[broadcast.SessionId] = func(proxy *rtsp_proxy.RtspProxy) {
			broadcast.Stop()
		}
	} else {
		rtspClient = rtsp_client.Create()
		err := rtspClient.ConnectAndPlay(remoteRtspAddress, rtsp_client.RtspTransportTcp)

		if err != nil {
			return err
		}
	}

	broadcast.BroadcastRtspClientToWebsockets(fmt.Sprintf("/ws_stream/%d", broadcast.SessionId), rtspClient, controlServer.WsServer)

	var lifeTimeEnd int64 = 0
	if lifeTime > 0 {
		lifeTimeEnd = time.Now().Add(time.Duration(lifeTime) * time.Second).Unix()
	}

	var lastActiveTime int64 = 0
	if activeTime > 0 {
		lastActiveTime = time.Now().Add(time.Duration(activeTime) * time.Second).Unix()
	}

	broadcastTimer := &BroadcastTimer{
		Broadcast:      broadcast,
		ActiveTime:     activeTime,
		LastActiveTime: lastActiveTime,
		LifeTimeEnd:    lifeTimeEnd,
	}

	controlServer.Lock()
	controlServer.WsBroadcasts[broadcast.SessionId] = broadcast
	controlServer.broadcastByRemoteAddress[remoteRtspAddress] = broadcast
	controlServer.BroadcastTimers[broadcast.SessionId] = broadcastTimer
	controlServer.Unlock()

	broadcast.OnStopListeners[0] = func(broadcast *rtsp_to_ws.Broadcast) {
		controlServer.Lock()
		delete(controlServer.WsBroadcasts, broadcast.SessionId)
		delete(controlServer.broadcastByRemoteAddress, remoteRtspAddress)
		delete(controlServer.BroadcastTimers, broadcast.SessionId)
		controlServer.Unlock()
	}

	return nil
}

func (controlServer *HttpControlServer) WsBroadcastList() []*BroadcastJson {
	broadcasts := []*BroadcastJson{}

	for _, broadcast := range controlServer.WsBroadcasts {
		broadcastJson := &BroadcastJson{}
		broadcastJson.Id = broadcast.SessionId
		broadcastJson.RemoteAddress = broadcast.RtspClient.RemoteAddress
		broadcastJson.LocalAddress = broadcast.Path
		broadcastJson.Clients = len(broadcast.Clients)

		broadcasts = append(broadcasts, broadcastJson)
	}

	return broadcasts
}

func (controlServer *HttpControlServer) StopWsBroadcast(broadcastId int32) error {
	broadcast, exist := controlServer.WsBroadcasts[broadcastId]
	if !exist {
		return errors.New("broadcast does not exist")
	}
	broadcast.Stop()

	controlServer.Lock()
	delete(controlServer.WsBroadcasts, broadcastId)
	delete(controlServer.proxyByRemoteAddress, broadcast.RtspClient.RemoteAddress)
	delete(controlServer.BroadcastTimers, broadcastId)
	controlServer.Unlock()

	return nil
}
