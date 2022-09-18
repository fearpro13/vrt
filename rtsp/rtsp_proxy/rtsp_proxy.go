package rtsp_proxy

import (
	"time"
	"vrt/logger"
	"vrt/rtsp/rtsp_client"
	"vrt/rtsp/rtsp_server"
)

type RtspProxy struct {
	RtspClient *rtsp_client.RtspClient
	RtspServer *rtsp_server.RtspServer
	IsRunning  bool
}

func Create() RtspProxy {
	client := rtsp_client.Create()
	server := rtsp_server.Create()

	proxy := RtspProxy{RtspClient: &client, RtspServer: &server}

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

	err = client.Connect(remoteRtspAddress)
	if err != nil {
		return err
	}

	err = client.Describe()
	if err != nil {
		return err
	}

	err = client.Options()
	if err != nil {
		return err
	}

	//TODO Использование задержки недопустимо. Необходимо сделать так, чтобы после выполнение options запускался setup метод
	time.Sleep(1 * time.Second)
	err = client.Setup()
	if err != nil {
		return err
	}

	err = client.Play()
	if err != nil {
		return err
	}

	return err
}

func (proxy *RtspProxy) Stop() error {
	client := proxy.RtspClient

	proxy.IsRunning = false

	err := client.TearDown()
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

	var recvRtpBuff []byte

	for proxy.IsRunning {
		if rtspClient.RtpServer.IsRunning {
			recvRtpBuff = <-rtspClient.RtpServer.RecvBuff

			lockedBuff := make([]byte, 2048)
			lockedBytes := copy(lockedBuff, recvRtpBuff)
			lockedBuff = lockedBuff[:lockedBytes]

			for _, client := range rtspServer.Clients {
				if len(lockedBuff) > 0 && client.RtpClient.IsConnected {
					cpyBuff := make([]byte, 2048)
					bytesCopied := copy(cpyBuff, lockedBuff)
					cpyBuff = cpyBuff[:bytesCopied]

					err := client.RtpClient.Send(cpyBuff)
					if err != nil {
						logger.Error(err.Error())
						err = client.Disconnect()
						if err != nil {
							logger.Error(err.Error())
						}
					}

					recvRtpBuff = recvRtpBuff[:0]
				}
			}

			lockedBuff = lockedBuff[:0]
		}
		time.Sleep(time.Millisecond * 1)
	}
}
