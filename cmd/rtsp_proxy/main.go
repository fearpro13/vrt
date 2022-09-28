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
	"vrt/logger"
	"vrt/rtsp/rtsp_client"
	"vrt/rtsp/rtsp_proxy"
)

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

	var transport string
	if len(os.Args) > 3 {
		transport = strings.ToLower(os.Args[3])
		if transport != rtsp_client.RtspTransportTcp && transport != rtsp_client.RtspTransportUdp {
			fmt.Println("Only TCP and UDP transports are supported")
			os.Exit(1)
		}
	}

	address := os.Args[1]

	rtspProxyTcp := rtsp_proxy.Create()
	err := rtspProxyTcp.ProxyFromAddress(address, 0, transport)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	signals := make(chan os.Signal, 1)

	signal.Notify(signals, syscall.SIGINT)
	signal.Notify(signals, syscall.SIGKILL)

	<-signals

	rtspProxyTcp.Stop()

	os.Exit(0)
}
