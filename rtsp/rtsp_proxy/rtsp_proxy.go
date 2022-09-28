package rtsp_proxy

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/deepch/vdk/codec/h264parser"
	"math/rand"
	"vrt/logger"
	"vrt/rtsp/rtsp_client"
	"vrt/rtsp/rtsp_server"
)

type RtspProxy struct {
	SessionId          int64
	RtspClient         *rtsp_client.RtspClient
	RtspServer         *rtsp_server.RtspServer
	IsRunning          bool
	PreBufferedClients map[int64]bool
	RTPPreBuff         []*[]byte
}

type RtpFragmentationInfo struct {
	fragmentationStarted bool
	fragmentationBuffer  bytes.Buffer
}

func Create() RtspProxy {
	server := rtsp_server.Create()

	proxy := RtspProxy{
		SessionId:          rand.Int63(),
		RtspServer:         server,
		RTPPreBuff:         []*[]byte{},
		PreBufferedClients: map[int64]bool{},
	}

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

	//rtspServer.OnClientConnect = func(client *rtsp_client.RtspClient) {
	//	proxy.PreBufferedClients[client.SessionId] = false
	//
	//	f := &RtpFragmentationInfo{
	//		fragmentationStarted: false,
	//		fragmentationBuffer:  bytes.Buffer{},
	//	}
	//
	//	client.OnStartPlaying = func(client *rtsp_client.RtspClient) {
	//		for _, preBuff := range proxy.RTPPreBuff {
	//			a := ContainsH264KeyFrame(f, preBuff)
	//			if a {
	//				logger.Info("KEY FRAME")
	//			}
	//			logger.Info("Send prebuff")
	//			sendToRtspClient(client, *preBuff)
	//		}
	//		proxy.PreBufferedClients[client.SessionId] = true
	//	}
	//}

	//fragmentationInfo := &RtpFragmentationInfo{
	//	fragmentationStarted: false,
	//	fragmentationBuffer:  bytes.Buffer{},
	//}

	rtspClient.SubscribeToRtpBuff(proxy.SessionId, func(bytesPtr *[]byte, num int) {
		rtpBytes := *bytesPtr

		if len(rtpBytes) == 0 {
			return
		}

		//h264KeyFrame := ContainsH264KeyFrame(fragmentationInfo, bytesPtr)
		//if h264KeyFrame {
		//	proxy.RTPPreBuff = []*[]byte{}
		//}
		//proxy.RTPPreBuff = append(proxy.RTPPreBuff, bytesPtr)

		for _, client := range rtspServer.Clients {
			if !client.IsConnected || !client.IsPlaying {
				continue
			}

			//preBuffered, exists := proxy.PreBufferedClients[client.SessionId]
			//
			//if exists {
			//	if preBuffered {
			//		delete(proxy.PreBufferedClients, client.SessionId)
			//	} else {
			//		continue
			//	}
			//}

			sendToRtspClient(client, *bytesPtr)
		}
	})
}

func sendToRtspClient(client *rtsp_client.RtspClient, payload []byte) {
	cpyBuff := make([]byte, len(payload))
	copy(cpyBuff, payload)

	if client.Transport == rtsp_client.RtspTransportTcp {
		if !client.TcpClient.IsConnected {
			return
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

func ContainsH264KeyFrame(fragmentationInfo *RtpFragmentationInfo, payloadRAW *[]byte) bool {
	content := *payloadRAW
	firstByte := content[4]
	padding := (firstByte>>5)&1 == 1
	extension := (firstByte>>4)&1 == 1
	CSRCCnt := int(firstByte & 0x0f)

	offset := 12

	end := len(content)
	if end-offset >= 4*CSRCCnt {
		offset += 4 * CSRCCnt
	}
	if extension && len(content) < 4+offset+2+2 {
		return false
	}
	if extension && end-offset >= 4 {
		extLen := 4 * int(binary.BigEndian.Uint16(content[4+offset+2:]))
		offset += 4
		if end-offset >= extLen {
			offset += extLen
		}
	}
	if padding && end-offset > 0 {
		paddingLen := int(content[end-1])
		if end-offset >= paddingLen {
			end -= paddingLen
		}
	}
	offset += 4
	//videoId := 96
	secondByte := 0 //int(content[1]) //226
	switch secondByte {
	case 0:
		nalRaw, _ := h264parser.SplitNALUs(content[offset:end])
		if len(nalRaw) == 0 || len(nalRaw[0]) == 0 {
			return false
		}
		for _, nal := range nalRaw {
			naluType := nal[0] & 0x1f
			switch {
			case naluType >= 1 && naluType <= 5:
				return naluType == 5
			case naluType == 24:
				packet := nal[1:]
				for len(packet) >= 2 {
					size := int(packet[0])<<8 | int(packet[1])
					if size+2 > len(packet) {
						break
					}
					naluTypefs := packet[2] & 0x1f
					switch {
					case naluTypefs >= 1 && naluTypefs <= 5:
						return naluType == 5
					}
					packet = packet[size+2:]
				}
			case naluType == 28:
				fuIndicator := content[offset]
				fuHeader := content[offset+1]
				isStart := fuHeader&0x80 != 0
				isEnd := fuHeader&0x40 != 0
				if isStart {
					fragmentationInfo.fragmentationStarted = true
					fragmentationInfo.fragmentationBuffer.Truncate(0)
					fragmentationInfo.fragmentationBuffer.Reset()
					fragmentationInfo.fragmentationBuffer.Write([]byte{fuIndicator&0xe0 | fuHeader&0x1f})
				}
				if fragmentationInfo.fragmentationStarted {
					fragmentationInfo.fragmentationBuffer.Write(content[offset+2 : end])
					if isEnd {
						fragmentationInfo.fragmentationStarted = false
						naluTypef := fragmentationInfo.fragmentationBuffer.Bytes()[0] & 0x1f
						if naluTypef == 7 || naluTypef == 9 {
							bufered, _ := h264parser.SplitNALUs(append([]byte{0, 0, 0, 1}, fragmentationInfo.fragmentationBuffer.Bytes()...))
							for _, v := range bufered {
								naluTypefs := v[0] & 0x1f
								if naluTypefs == 5 {
									fragmentationInfo.fragmentationBuffer.Reset()
									fragmentationInfo.fragmentationBuffer.Write(v)
									return true
								}
							}
						}
						return naluTypef == 5
					}
				}
			default:
				return false
			}
		}
	default:
		return false
	}

	return false
}
