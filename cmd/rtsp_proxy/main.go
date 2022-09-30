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
	"vrt/utils/cli"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	command := cli.NewCommand()
	command.RegisterArgument("remote rtsp address ex. rtsp://user:password@10.10.20.40:554")
	command.RegisterOption("port", "p", "local RTSP port", false, "6060")
	command.RegisterOption("transport", "t", "RTSP transport that is used for remote connection, only TCP and UDP are supported", true, "tcp")
	command.RegisterOption("verbose", "v", "number,verbosity or log level, \n		JUNK-0,DEBUG-1,INFO-2,NOTICE-3,WARN-4,ERR-5,CRITICAL-6,EMERGENCY-7,SILENT-10", true, "2")

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

	localRtspPortString := command.GetOption("p").Value
	localRtspPort, err := strconv.Atoi(localRtspPortString)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	logger.Info(fmt.Sprintf("Selected local RTSP port: %d", localRtspPort))

	address := os.Args[1]

	rtspProxy := rtsp_proxy.Create()
	err = rtspProxy.ProxyFromAddress(address, localRtspPort, transport)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	signals := make(chan os.Signal, 1)

	signal.Notify(signals, syscall.SIGINT)
	signal.Notify(signals, syscall.SIGKILL)

	<-signals

	rtspProxy.Stop()

	os.Exit(0)
}
