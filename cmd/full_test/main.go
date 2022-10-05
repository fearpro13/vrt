package main

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"
	"vrt/logger"
	"vrt/rtsp/rtsp_client"
	"vrt/rtsp/rtsp_proxy"
	"vrt/rtsp_to_ws"
	"vrt/ws/ws_server"
)

// rtsp://stream:Tv4m6ag6@10.1.60.25
// rtsp://stream:Tv4m6ag6@10.3.43.140:554
// rtsp://stream:Tv4m6ag6@10.0.229.148 //sound+auth
func main() {
	rand.Seed(time.Now().UnixNano())

	if len(os.Args) < 2 {
		fmt.Println("RTSP address argument is not defined")
		os.Exit(1)
	}

	if len(os.Args) > 2 {
		logLevelString := os.Args[2]
		logLevelInt4, err := strconv.ParseInt(logLevelString, 10, 4)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
		logLevelInt := int(logLevelInt4)
		logger.SetLogLevel(logLevelInt)
	}

	address := os.Args[1]

	//////////////////////////////////////////////////////
	rtspProxyTcp := rtsp_proxy.Create()
	err := rtspProxyTcp.ProxyFromAddress(address, 0, rtsp_client.RtspTransportTcp)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	wsServer1 := ws_server.Create()
	err = wsServer1.Start("/", "", 6061)
	if err != nil {
		logger.Error(err.Error())
	}

	wsBroadcast1 := rtsp_to_ws.NewBroadcast()
	wsBroadcast1.BroadcastRtspClientToWebsockets("/", rtspProxyTcp.RtspClient, wsServer1)

	//////////////////////////////////////////////////////////
	rtspProxyUdp := rtsp_proxy.Create()
	err = rtspProxyUdp.ProxyFromAddress(address, 0, rtsp_client.RtspTransportUdp)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	wsServer2 := ws_server.Create()
	err = wsServer2.Start("/", "", 6062)
	if err != nil {
		logger.Error(err.Error())
	}

	wsBroadcast2 := rtsp_to_ws.NewBroadcast()
	wsBroadcast2.BroadcastRtspClientToWebsockets("/", rtspProxyUdp.RtspClient, wsServer2)

	////////////////////////////////////////////////////////////
	rtspClientTcp := rtsp_client.Create()
	rtspClientTcp.Connect(rtspProxyTcp.RtspServer.RtspAddress, rtsp_client.RtspTransportTcp)
	rtspClientTcp.Describe()
	rtspClientTcp.Options()
	rtspClientTcp.Setup()
	rtspClientTcp.Play()

	wsServer3 := ws_server.Create()
	err = wsServer3.Start("/", "", 6063)
	if err != nil {
		logger.Error(err.Error())
	}

	wsBroadcast3 := rtsp_to_ws.NewBroadcast()
	wsBroadcast3.BroadcastRtspClientToWebsockets("/", rtspClientTcp, wsServer3)

	////////////////////////////////////////////////////////////
	rtspClientUdp := rtsp_client.Create()
	rtspClientUdp.Connect(rtspProxyUdp.RtspServer.RtspAddress, rtsp_client.RtspTransportUdp)
	rtspClientUdp.Describe()
	rtspClientUdp.Options()
	rtspClientUdp.Setup()
	rtspClientUdp.Play()

	wsServer4 := ws_server.Create()
	err = wsServer4.Start("/", "", 6064)
	if err != nil {
		logger.Error(err.Error())
	}

	wsBroadcast4 := rtsp_to_ws.NewBroadcast()
	wsBroadcast4.BroadcastRtspClientToWebsockets("/", rtspClientUdp, wsServer4)

	//////////////////////////////////////////////////////////

	//TODO Убрать это решение из продакшен кода, использовать только для локальной разработки
	select {}
}
