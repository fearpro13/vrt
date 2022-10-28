package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"vrt/http/http_control"
	"vrt/logger"
	"vrt/rtsp/rtsp_client"
	"vrt/rtsp/rtsp_proxy"
	"vrt/rtsp_to_ws"
	"vrt/utils/cli"
	"vrt/ws/ws_server"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	command := cli.NewCommand()
	command.RegisterArgument("remote rtsp address ex. rtsp://user:password@10.10.20.40:554")
	command.RegisterOption("transport", "t", "RTSP transport that is used for remote connection, only TCP and UDP are supported", true, "tcp")
	command.RegisterOption("verbose", "v", "number,verbosity or log level, \n		JUNK-0,DEBUG-1,INFO-2,NOTICE-3,WARN-4,ERR-5,CRITICAL-6,EMERGENCY-7,SILENT-10", true, "2")

	command.RegisterFlag("proxy", "p", "Enables rtsp proxy mode", true, false)
	command.RegisterOption("p:port", "p:p", "local RTSP port", true, "6060")

	command.RegisterFlag("websocket", "w", "Enables websocket stream mode", true, false)
	command.RegisterOption("w:port", "w:p", "websocket port", true, "0")

	command.RegisterFlag("http", "h", "Enables http control mode", true, false)
	command.RegisterOption("h:port", "h:p", "Http control server port", true, "0")

	command.RegisterOption("cert", "c", "Https certificate path", true, "")
	command.RegisterOption("key", "k", "Https private key path", true, "")

	if len(os.Args) == 2 && os.Args[1] == "--help" {
		fmt.Println(command.PrintHelp())
		os.Exit(0)
	}

	args := os.Args[1:]
	err := command.Init(&args)

	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	loglevel := command.GetOption("v")
	logLevelInt, err := strconv.Atoi(loglevel.Value)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	logger.SetLogLevel(logLevelInt)
	logger.Info(fmt.Sprintf("Selected log level %d - %s", logLevelInt, logger.LogLevelToName[logLevelInt]))

	transport := command.GetOption("t").Value
	transport = strings.ToLower(transport)
	if transport != rtsp_client.RtspTransportTcp && transport != rtsp_client.RtspTransportUdp {
		logger.Error("Only TCP and UDP transports are supported")
		os.Exit(1)
	}
	logger.Info(fmt.Sprintf("Selected remote RTSP transport: %s", transport))

	remoteRtspAddress := command.GetArgument(0).Value

	proxyMode := command.GetFlag("proxy").Value
	var rtspProxy *rtsp_proxy.RtspProxy
	if proxyMode {
		logger.Info("Program uses proxy mode")

		localRtspPortString := command.GetOption("p:p").Value
		localRtspPort, err := strconv.Atoi(localRtspPortString)
		if err != nil {
			logger.Error(err.Error())
			os.Exit(1)
		}
		logger.Info(fmt.Sprintf("Selected local RTSP port: %d", localRtspPort))

		rtspProxy = rtsp_proxy.Create()
		err = rtspProxy.ProxyFromAddress(remoteRtspAddress, localRtspPort, transport)
		if err != nil {
			logger.Error(err.Error())
			os.Exit(1)
		}
	}

	websocketMode := command.GetFlag("websocket").Value

	httpsCertPath := command.GetOption("cert").Value
	httpsPrivateKeyPath := command.GetOption("key").Value

	var wsBroadcast *rtsp_to_ws.Broadcast
	if websocketMode {
		logger.Info("Program uses websocket mode")

		wsServerPortString := command.GetOption("w:p").Value
		wsServerPortInt, err := strconv.Atoi(wsServerPortString)
		if err != nil {
			logger.Error(err.Error())
			os.Exit(1)
		}

		wsServer := ws_server.Create()
		err = wsServer.Start("/", "", wsServerPortInt, httpsCertPath, httpsPrivateKeyPath)
		if err != nil {
			logger.Error(err.Error())
			os.Exit(1)
		}

		wsBroadcast = rtsp_to_ws.NewBroadcast()
		if proxyMode {
			wsBroadcast.BroadcastRtspClientToWebsockets("/", rtspProxy.RtspClient, wsServer)
		} else {
			rtspClient := rtsp_client.Create()
			err := rtspClient.ConnectAndPlay(command.GetArgument(0).Value, command.GetOption("t").Value)
			if err != nil {
				logger.Error(err.Error())
				os.Exit(1)
			}
			wsBroadcast.BroadcastRtspClientToWebsockets("/", rtspClient, wsServer)
		}
	}

	httpControlMode := command.GetFlag("h").Value
	if httpControlMode {
		logger.Info("Program uses http control mode")

		controlServerPortString := command.GetOption("h:p").Value
		controlServerPort, err := strconv.Atoi(controlServerPortString)
		if err != nil {
			logger.Error(err.Error())
			os.Exit(1)
		}
		controlServer := http_control.NewHttpControlServer()
		err = controlServer.Start("", controlServerPort, httpsCertPath, httpsPrivateKeyPath)
		if err != nil {
			logger.Error(err.Error())
			os.Exit(1)
		}
	}

	if !websocketMode && !proxyMode && !httpControlMode {
		logger.Info("Program stopped because there is no modes enabled")
		os.Exit(0)
	}

	signals := make(chan os.Signal, 1)

	signal.Notify(signals, syscall.SIGINT)
	signal.Notify(signals, syscall.SIGKILL)

	<-signals

	if websocketMode {
		wsBroadcast.Stop()
	}

	if proxyMode {
		err = rtspProxy.Stop()
		if err != nil {
			logger.Error(err.Error())
		}
	}

	os.Exit(0)
}
