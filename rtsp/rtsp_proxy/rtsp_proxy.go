package rtsp_proxy

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/rand"
	"vrt/logger"
	"vrt/rtsp/rtsp_client"
	"vrt/rtsp/rtsp_server"
)

type OnStopCallback func(proxy *RtspProxy)

type RtspProxy struct {
	SessionId          int32
	RtspClient         *rtsp_client.RtspClient
	RtspServer         *rtsp_server.RtspServer
	IsRunning          bool
	PreBufferedClients map[int32]bool
	RTPPreBuff         []*[]byte
	clientSelfHosted   bool // if rtsp client was created within rtsp_proxy, not outside
	OnStopListeners    map[int32]OnStopCallback
}

type RtpFragmentationInfo struct {
	fragmentationStarted bool
	fragmentationBuffer  bytes.Buffer
}

func Create() *RtspProxy {
	server := rtsp_server.Create()

	proxy := &RtspProxy{
		SessionId:          rand.Int31(),
		RtspServer:         server,
		RTPPreBuff:         []*[]byte{},
		PreBufferedClients: map[int32]bool{},
		OnStopListeners:    map[int32]OnStopCallback{},
	}

	return proxy
}

func (proxy *RtspProxy) ProxyFromRtspClient(client *rtsp_client.RtspClient, localRtspPort int) error {
	err := proxy.RtspServer.Start("", localRtspPort, 0)
	if err != nil {
		return err
	}

	if !client.IsConnected {
		return errors.New("rtsp proxy #%d: RTSP клиент должен быть подключен перед запуском прокирования")
	}

	proxy.IsRunning = true
	go proxy.run()

	if !client.IsPlaying {
		_, _, err = client.Describe()
		if err != nil {
			return err
		}

		_, _, err = client.Options()
		if err != nil {
			return err
		}

		_, _, err = client.Setup()
		if err != nil {
			return err
		}

		_, _, err = client.Play()
		if err != nil {
			return err
		}
	}

	return err
}

func (proxy *RtspProxy) ProxyFromAddress(remoteRtspAddress string, localRtspPort int, remoteTransport string) error {
	client := rtsp_client.Create()
	proxy.clientSelfHosted = true
	proxy.RtspClient = client
	err := client.Connect(remoteRtspAddress, remoteTransport)
	if err != nil {
		return err
	}

	return proxy.ProxyFromRtspClient(client, localRtspPort)
}

func (proxy *RtspProxy) Stop() error {
	if !proxy.IsRunning {
		return nil
	}

	proxy.IsRunning = false

	client := proxy.RtspClient
	server := proxy.RtspServer

	client.UnsubscribeFromRtpBuff(proxy.SessionId)

	if proxy.clientSelfHosted {
		if client.IsConnected {
			_, _, err := client.TearDown()
			if err != nil {
				return err
			}

			err = client.Disconnect()
			if err != nil {
				return err
			}
		}
	}

	err := server.Stop()

	for _, listener := range proxy.OnStopListeners {
		go listener(proxy)
	}

	return err
}

func (proxy *RtspProxy) run() {
	rtspServer := proxy.RtspServer
	rtspClient := proxy.RtspClient

	rtspClient.OnDisconnectListeners[proxy.SessionId] = func(client *rtsp_client.RtspClient) {
		_ = proxy.Stop()
	}

	rtspClient.SubscribeToRtpBuff(proxy.SessionId, func(bytesPtr *[]byte, num int) {
		rtpBytes := *bytesPtr

		if len(rtpBytes) == 0 {
			return
		}

		for _, client := range rtspServer.Clients {
			if !client.IsConnected || !client.IsPlaying {
				continue
			}

			sendToRtspClient(client, *bytesPtr)
		}
	})
}

func sendToRtspClient(client *rtsp_client.RtspClient, payload []byte) {
	cpyBuff := make([]byte, len(payload))
	copy(cpyBuff, payload)

	if client.Transport == rtsp_client.RtspTransportTcp {
		if cpyBuff[0] == 0x24 {
			_, err := client.TcpClient.Send(cpyBuff)
			if err != nil {
				logger.Error(err.Error())
				err = client.Disconnect()
				if err != nil {
					logger.Error(err.Error())
				}
			}
		} else {
			buff := make([]byte, len(cpyBuff)+4)
			header := make([]byte, 4)
			header[0] = 0x24
			header[1] = 0
			binary.BigEndian.PutUint16(header[2:4], uint16(len(cpyBuff)))
			copy(buff, header)
			copy(buff[4:], cpyBuff)

			_, err := client.TcpClient.Send(buff)
			if err != nil {
				logger.Error(err.Error())
				err = client.Disconnect()
				if err != nil {
					logger.Error(err.Error())
				}
			}
		}
	}

	if client.Transport == rtsp_client.RtspTransportUdp {
		if !client.RtpClient.IsConnected {
			return
		}

		if cpyBuff[0] == 0x24 {
			err := client.RtpClient.Send(cpyBuff[4:])
			if err != nil {
				logger.Error(err.Error())
				err = client.Disconnect()
				if err != nil {
					logger.Error(err.Error())
				}
			}
		} else {
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
}
