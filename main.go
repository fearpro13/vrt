package main

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"
	"vrt/logger"
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

	address := os.Args[1]

	rtspProxy := rtsp_proxy.Create()
	err := rtspProxy.Start(address, 0)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	//TODO Убрать это решение из продакшен кода, использовать только для локальной разработки
	select {}
}
