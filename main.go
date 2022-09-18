package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"time"
	"vrt/logger"
	"vrt/rtsp/rtsp_proxy"
	"vrt/ws/ws_client"
	"vrt/ws/ws_server"
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

	wsServer := ws_server.Create()
	err = wsServer.Start("", 6060)
	if err != nil {
		logger.Error(err.Error())
	}

	pathExp := regexp.MustCompile("^\\/.+\\/")
	fileDir := pathExp.FindString(os.Args[0])
	filePath := fileDir + "a.mp4"

	os.Create(filePath)
	f, err := os.OpenFile(filePath, os.O_WRONLY, os.ModePerm)
	if err != nil {
		logger.Error(err.Error())
	}

	wsServer.Callback = func(client *ws_client.WSClient) {
		client.SetCallback(func(client *ws_client.WSClient) {
			rtspProxy.UnsubscribeFromRtpBuff(client.SessionId)
		})
		rtspProxy.SubscribeToRtpBuff(client.SessionId, func(bytes []byte) {
			p := rtp.Packet{}
			err := p.Unmarshal(bytes)
			if err != nil {
				logger.Error(err.Error())
			}

			h := codecs.H264Packet{}
			hRaw, err := h.Unmarshal(p.Payload)

			_, err = f.Write(p.Payload)
			if err != nil {
				logger.Error(err.Error())
			}
			client.Send(websocket.BinaryMessage, hRaw)
		})
	}

	//TODO Убрать это решение из продакшен кода, использовать только для локальной разработки
	select {}
}
