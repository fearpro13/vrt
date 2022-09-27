package rtsp_proxy

import (
	"encoding/binary"
	"errors"
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
	RTPPreBuff chan *[]byte
}

func Create() RtspProxy {
	server := rtsp_server.Create()

	proxy := RtspProxy{SessionId: rand.Int63(), RtspServer: server}

	return proxy
}

func (proxy *RtspProxy) ProxyFromRtspClient(client *rtsp_client.RtspClient, localRtspPort int) error {
	err := proxy.RtspServer.Start("", localRtspPort, 0)
	if err != nil {
		return err
	}

	proxy.IsRunning = true

	go proxy.run()

	if err != nil {
		return err
	}

	if !client.IsConnected {
		return errors.New("rtsp proxy #%d: RTSP клиент должен быть подключен перед запуском прокирования")
	}

	if !client.IsPlaying {
		_, err = client.Describe()
		if err != nil {
			return err
		}

		_, err = client.Options()
		if err != nil {
			return err
		}

		_, err = client.Setup()
		if err != nil {
			return err
		}

		_, err = client.Play()
		if err != nil {
			return err
		}
	}

	return err
}

func (proxy *RtspProxy) ProxyFromAddress(remoteRtspAddress string, localRtspPort int, remoteTransport string) error {
	client := rtsp_client.Create()
	proxy.RtspClient = client
	err := client.Connect(remoteRtspAddress, remoteTransport)
	if err != nil {
		return err
	}

	return proxy.ProxyFromRtspClient(client, localRtspPort)
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

	rtspServer.OnClientConnect = func(client *rtsp_client.RtspClient) {

	}

	rtspClient.SubscribeToRtpBuff(proxy.SessionId, func(bytesPtr *[]byte, num int) {
		bytes := *bytesPtr

		if len(bytes) == 0 {
			return
		}

		//logger.Debug(fmt.Sprintf("Receivev RTP packet #%d", num))

		for _, client := range rtspServer.Clients {
			if !client.IsConnected || !client.IsPlaying {
				continue
			}

			cpyBuff := make([]byte, len(bytes))
			copy(cpyBuff, bytes)

			if client.Transport == rtsp_client.RtspTransportTcp {
				if !client.TcpClient.IsConnected {
					continue
				}

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
					buff := make([]byte, len(bytes)+4)
					header := make([]byte, 4)
					header[0] = 0x24
					header[1] = 0
					binary.BigEndian.PutUint16(header[2:4], uint16(len(bytes)))
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
					continue
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
	})
}
