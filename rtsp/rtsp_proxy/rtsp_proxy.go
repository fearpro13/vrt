package rtsp_proxy

import (
	"math/rand"
	"vrt/logger"
	"vrt/rtsp/rtsp_client"
	"vrt/rtsp/rtsp_server"
)

type RtspProxy struct {
	SessionId  int64
	RtspClient *rtsp_client.RtspClient
	RtspServer *rtsp_server.RtspServer
	IsRunning  bool
}

func Create() RtspProxy {
	client := rtsp_client.Create()
	server := rtsp_server.Create()

	proxy := RtspProxy{SessionId: rand.Int63(), RtspClient: client, RtspServer: server}

	return proxy
}

func (proxy *RtspProxy) Start(remoteRtspAddress string, localRtspPort int) error {
	client := proxy.RtspClient
	server := proxy.RtspServer

	err := server.Start("", localRtspPort, 0)
	if err != nil {
		return err
	}

	proxy.IsRunning = true

	go proxy.run()

	err = client.Connect(remoteRtspAddress, rtsp_client.RtspTransportTcp)
	if err != nil {
		return err
	}

	_, err = client.Describe()
	if err != nil {
		return err
	}

	_, err = client.Options()
	if err != nil {
		return err
	}

	//time.Sleep(1 * time.Second)

	//TODO Использование задержки недопустимо. Необходимо сделать так, чтобы после выполнение options запускался setup метод
	_, err = client.Setup()
	if err != nil {
		return err
	}

	//time.Sleep(1 * time.Second)

	_, err = client.Play()
	if err != nil {
		return err
	}

	return err
}

func (proxy *RtspProxy) Stop() error {
	client := proxy.RtspClient

	client.UnsubscribeFromRtpBuff(proxy.SessionId)

	proxy.IsRunning = false

	_, err := client.TearDown()
	if err != nil {
		return err
	}

	err = client.Disconnect()
	if err != nil {
		return err
	}

	return err
}

func (proxy *RtspProxy) run() {
	rtspServer := proxy.RtspServer
	rtspClient := proxy.RtspClient

	rtspClient.SubscribeToRtpBuff(proxy.SessionId, func(bytes []byte) {
		if len(bytes) == 0 {
			return
		}

		for _, client := range rtspServer.Clients {
			if client.RtpClient.IsConnected {
				cpyBuff := make([]byte, 2048)
				bytesCopied := copy(cpyBuff, bytes)
				cpyBuff = cpyBuff[:bytesCopied]

				err := client.RtpClient.Send(cpyBuff)
				if err != nil {
					logger.Error(err.Error())
					err = client.Disconnect()
					if err != nil {
						logger.Error(err.Error())
					}
				}
			}
		}
	})

	//for proxy.IsRunning {
	//	if rtspClient.RtpServer.IsRunning {
	//		recvRtpBuff = <-rtspClient.RtpServer.RecvBuff
	//
	//		lockedBuff := make([]byte, 2048)
	//		lockedBytes := copy(lockedBuff, recvRtpBuff)
	//		lockedBuff = lockedBuff[:lockedBytes]
	//
	//		for _, client := range rtspServer.Clients {
	//			if len(lockedBuff) > 0 && client.RtpClient.IsConnected {
	//				cpyBuff := make([]byte, 2048)
	//				bytesCopied := copy(cpyBuff, lockedBuff)
	//				cpyBuff = cpyBuff[:bytesCopied]
	//
	//				err := client.RtpClient.Send(cpyBuff)
	//				if err != nil {
	//					logger.Error(err.Error())
	//					err = client.Disconnect()
	//					if err != nil {
	//						logger.Error(err.Error())
	//					}
	//				}
	//
	//				recvRtpBuff = recvRtpBuff[:0]
	//			}
	//		}
	//
	//		lockedBuff = lockedBuff[:0]
	//	}
	//	time.Sleep(time.Millisecond * 1)
	//}
}
