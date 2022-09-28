package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"regexp"
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

	if len(os.Args) == 2 && os.Args[1] == "--help" {
		help := "Usage: \n" +
			"Arguments:\n" +
			"	1 - remote rtsp address ex. rtsp://user:password@10.10.20.40:554\n" +
			"Options: \n" +
			"	-p local RTSP port\n" +
			"	-v number,verbosity or log level, \n" +
			"		JUNK-0,DEBUG-1,INFO-2,NOTICE-3,WARN-4,ERR-5,CRITICAL-6,EMERGENCY-7,SILENT-10\n" +
			"	-t RTSP transport that is used for remote connection, only TCP and UDP are supported. Default: tcp"
		fmt.Println(help)

		os.Exit(0)
	}

	if len(os.Args) < 2 {
		logger.Error("RTSP address argument is not defined")
		os.Exit(1)
	}

	mappedOptions := map[string]string{}
	for i := 2; i < len(os.Args); i++ {
		curArg := os.Args[i]
		optionNameExp := regexp.MustCompile("-(\\w+)")
		if !optionNameExp.MatchString(curArg) {
			logger.Error(fmt.Sprintf("Wrong option %s, most probably you are missing prefix '-'", curArg))
			os.Exit(1)
		}

		nextArg := os.Args[i+1]

		optionName := optionNameExp.FindStringSubmatch(curArg)[1]
		mappedOptions[optionName] = nextArg

		i = i + 1
	}

	loglevel, mapped := mappedOptions["v"]
	if !mapped {
		loglevel = "2"
	}
	logLevelInt, err := strconv.Atoi(loglevel)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	logger.SetLogLevel(logLevelInt)
	logger.Info(fmt.Sprintf("Selected log level %d - %s", logLevelInt, logger.LogLevelToName[logLevelInt]))

	transport, mapped := mappedOptions["t"]
	if !mapped {
		transport = "tcp"
	}
	transport = strings.ToLower(transport)
	if transport != rtsp_client.RtspTransportTcp && transport != rtsp_client.RtspTransportUdp {
		logger.Error("Only TCP and UDP transports are supported")
		os.Exit(1)
	}
	logger.Info(fmt.Sprintf("Selected remote RTSP transport: %s", transport))

	localRtspPortString, mapped := mappedOptions["p"]
	if !mapped {
		localRtspPortString = "554"
	}
	localRtspPort, err := strconv.Atoi(localRtspPortString)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	logger.Info(fmt.Sprintf("Selected local RTSP port: %d", localRtspPort))

	address := os.Args[1]

	rtspProxyTcp := rtsp_proxy.Create()
	err = rtspProxyTcp.ProxyFromAddress(address, localRtspPort, transport)
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
