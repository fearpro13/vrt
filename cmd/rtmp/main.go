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
	"vrt/rtmp/rtmp_server"
	"vrt/rtsp/rtsp_client"
	"vrt/utils/cli"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	command := cli.NewCommand()
	command.RegisterOption("transport", "t", "RTSP transport that is used for remote connection, only TCP and UDP are supported", true, "tcp")
	command.RegisterOption("verbose", "v", "number,verbosity or log level, \n		JUNK-0,DEBUG-1,INFO-2,NOTICE-3,WARN-4,ERR-5,CRITICAL-6,EMERGENCY-7,SILENT-10", true, "2")

	command.RegisterFlag("rtmp", "r", "Enables RTMP receiver server", true, false)
	command.RegisterOption("r:port", "r:p", "RTMP receiver server port", true, "0")

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

	rtmpMode := command.GetFlag("r").Value
	var rtmpServer *rtmp_server.RtmpServer
	if rtmpMode {
		rtmpPortString := command.GetOption("r:p").Value
		rtmpPort, _ := strconv.Atoi(rtmpPortString)

		rtmpServer = rtmp_server.Create()
		rtmpServer.Start("", rtmpPort)
	}

	signals := make(chan os.Signal, 1)

	signal.Notify(signals, syscall.SIGINT)
	signal.Notify(signals, syscall.SIGKILL)

	<-signals

	if rtmpMode {
		_ = rtmpServer.Stop()
	}

	os.Exit(0)
}
