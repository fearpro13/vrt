package rtsp_client

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/format/rtsp/sdp"
	"io"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"vrt/logger"
	"vrt/tcp/tcp_client"
	"vrt/udp/udp_client"
	"vrt/udp/udp_server"
)

const RtspTransportTcp = "tcp"
const RtspTransportUdp = "udp"

type OnDisconnectCallback func(client *RtspClient)
type OnStartPlayingCallback func(client *RtspClient)

type RtspClient struct {
	Transport                     string
	TcpClient                     *tcp_client.TcpClient
	RtpServer                     *udp_server.UdpServer
	RtpClient                     *udp_client.UdpClient
	SessionId                     int64
	CSeq                          int
	RemoteAddress                 string
	RemoteStreamAddress           string
	LocalRtpServerPort            int
	RemoteRtpServerPort           int //required for udp hair pinning
	IsConnected                   bool
	IsPlaying                     bool
	StartVideoTimestamp           int64
	PreVideoTimestamp             int64
	RTPPacketFragmentationStarted bool
	RTPFragmentationBuffer        bytes.Buffer //used for NALU FU buffer
	Sdp                           []sdp.Media
	Codecs                        []av.CodecData
	VideoCodec                    av.CodecType
	VideoIDX                      int8
	RtpSubscribers                map[int64]RtpSubscriber
	RTPVideoChan                  chan []byte
	RTPAudioChan                  chan []byte
	OnDisconnect                  OnDisconnectCallback
	OnStartPlaying                OnStartPlayingCallback
}

type RtpSubscriber func(*[]byte, int)

func Create() *RtspClient {
	sessionId := rand.Int63()
	tcpClient := tcp_client.Create()
	udpServer := udp_server.Create()
	udpClient := udp_client.Create()

	rtspClient := &RtspClient{
		SessionId:      sessionId,
		CSeq:           0,
		TcpClient:      tcpClient,
		RtpServer:      udpServer,
		RtpClient:      udpClient,
		RtpSubscribers: map[int64]RtpSubscriber{},
		RTPVideoChan:   make(chan []byte, 1024),
		RTPAudioChan:   make(chan []byte, 1024),
	}

	return rtspClient
}

func CreateFromConnection(tcpClient *tcp_client.TcpClient) *RtspClient {
	client := Create()

	client.TcpClient = tcpClient
	client.IsConnected = true
	logger.Info(fmt.Sprintf("RTSP client #%d connected", client.SessionId))

	return client
}

func (client *RtspClient) Connect(address string, transport string) error {
	if transport == "" {
		transport = RtspTransportUdp
	}
	client.Transport = transport

	hasRtsp, _ := regexp.MatchString("^rtsp:/{2}", address)
	hasCreds, _ := regexp.MatchString("\\w+:\\w+", address)

	if !hasRtsp {
		return errors.New("отсутствует префикс rtsp://")
	}

	if !hasCreds {
		return errors.New("отсутсвуют данные для авторизации BasicAuth")
	}

	ipExp, _ := regexp.Compile("\\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\\b")
	ip := ipExp.FindString(address)

	if ip == "" {
		return errors.New("отсутствует IP адрес")
	}

	portExp, _ := regexp.Compile("(:\\d+)")
	portString := portExp.FindString(address)

	var portInt int
	if portString == "" {
		portInt = 554
	} else {
		portString = strings.Replace(portString, ":", "", 1)
		portInt64, _ := strconv.ParseInt(portString, 10, 0)
		portInt = int(portInt64)
	}

	client.TcpClient.OnDisconnect = func(tcpClient *tcp_client.TcpClient) {
		client.Disconnect()
	}

	err := client.TcpClient.Connect(ip, portInt)

	if err != nil {
		return err
	}

	client.RemoteAddress = address
	client.IsConnected = true

	logger.Info(fmt.Sprintf("RTSP client #%d connected, transport: %s", client.SessionId, transport))

	return err
}

func (client *RtspClient) Disconnect() error {
	if client.Transport == RtspTransportUdp {
		/*
			Если rtsp клиент используется для подключения к удалённому серверу - RTP client не создаётся, создаётся только RTP сервер
			Если входящее соединение рассматривается как RTP клиент - RTP client создаётся, но не создаётся RTP сервер
		*/
		if client.RtpClient.IsConnected {
			err := client.RtpClient.Disconnect()
			if err != nil {
				return err
			}
		}

		if client.RtpServer.IsRunning {
			err := client.RtpServer.Stop()
			if err != nil {
				return err
			}
		}
	}

	if client.TcpClient.IsConnected {
		err := client.TcpClient.Disconnect()
		if err != nil {
			return err
		}
	}

	client.IsConnected = false

	if client.OnDisconnect != nil {
		client.OnDisconnect(client)
	}

	logger.Info(fmt.Sprintf("RTSP client #%d disconnected", client.SessionId))

	return nil
}

func (client *RtspClient) Describe() (response string, err error) {
	message := ""
	message += fmt.Sprintf("DESCRIBE %s RTSP/1.0\r\n", client.RemoteAddress)
	message += fmt.Sprintf("CSeq: %d\r\n", client.CSeq)
	message += "Accept: application/sdp\r\n"
	message += "\r\n"

	client.CSeq++
	_, err = client.TcpClient.SendString(message)
	if err != nil {
		return "", nil
	}

	response, err = client.ReadMessage()
	if err != nil {
		return "", err
	}
	parseSdp(client, &response)

	return response, err
}

func (client *RtspClient) Options() (response string, err error) {
	message := ""
	message += fmt.Sprintf("OPTIONS %s RTSP/1.0\r\n", client.RemoteAddress)
	message += fmt.Sprintf("CSeq: %d\r\n", client.CSeq)

	message += "Accept: application/sdp\r\n"
	message += "\r\n"

	client.CSeq++
	_, err = client.TcpClient.SendString(message)

	if err != nil {
		return "", nil
	}

	return client.ReadMessage()
}

func (client *RtspClient) Setup() (response string, err error) {
	message := ""
	message += fmt.Sprintf("SETUP %s RTSP/1.0\r\n", client.RemoteStreamAddress)
	message += fmt.Sprintf("CSeq: %d\r\n", client.CSeq)

	if client.Transport == RtspTransportTcp {
		message += fmt.Sprintf("Transport: RTP/AVP/TCP;unicast;interleaved=0-1\r\n")
	}

	if client.Transport == RtspTransportUdp {
		randPortInt := rand.Intn(64000) + 1024
		client.LocalRtpServerPort = randPortInt

		message += fmt.Sprintf("Transport: RTP/AVP/UDP;unicast;client_port=%d-%d\r\n", randPortInt, randPortInt+1)
	}

	message += "\r\n"

	client.CSeq++
	_, err = client.TcpClient.SendString(message)

	response, err = client.ReadMessage()

	lines := strings.Split(response, "\r\n")
	for _, line := range lines {
		serverPortExp := regexp.MustCompile("server_port=(\\d+)-(\\d+)")
		if serverPortExp.MatchString(line) {
			serverPort := serverPortExp.FindStringSubmatch(line)[1]
			client.RemoteRtpServerPort, err = strconv.Atoi(serverPort)
			if err != nil {
				return "", err
			}
		}

		sessionExp := regexp.MustCompile("^[sS]ession:\\s+(\\d+)")
		if sessionExp.MatchString(line) {
			sessionIdString := sessionExp.FindStringSubmatch(line)[1]
			sessionIdInt, err := strconv.Atoi(sessionIdString)
			if err != nil {
				return "", err
			}
			client.SessionId = int64(sessionIdInt)
		}
	}

	if client.Transport == RtspTransportUdp {
		///////hair pinning/////////////////////
		udpClient := udp_client.Create()
		udpClient.LocalPort = client.LocalRtpServerPort
		err = udpClient.Connect(client.TcpClient.Ip, client.RemoteRtpServerPort)
		if err != nil {
			return "", err
		}

		err = udpClient.SendMessage("HAIR PINNING;IGNORE THIS MESSAGE")
		if err != nil {
			return "", err
		}

		err = udpClient.Disconnect()
		if err != nil {
			return "", err
		}
		//////////////////////////////////////////

		rtpServer := udp_server.Create()
		client.RtpServer = rtpServer

		err := rtpServer.Start("", client.LocalRtpServerPort)
		if err != nil {
			return "", err
		}
	}

	return response, err
}

func (client *RtspClient) Play() (response string, err error) {
	if client.SessionId == 0 {
		return "", errors.New("для начала воспроизведения необходимо получить sessionId от сервера")
	}

	message := ""
	message += fmt.Sprintf("PLAY %s RTSP/1.0\r\n", client.RemoteAddress)
	message += fmt.Sprintf("CSeq: %d\r\n", client.CSeq)
	message += fmt.Sprintf("Session:%d\r\n", client.SessionId)
	message += "Accept: application/sdp, application/rtsl, application/mheg\r\n"
	message += "\r\n"

	client.CSeq++
	_, err = client.TcpClient.SendString(message)

	if err != nil {
		return "", err
	}

	response, err = client.ReadMessage()
	if err != nil {
		return "", err
	}

	rtpTimeExp := regexp.MustCompile("rtptime=(\\d+)")
	responseLines := strings.Split(response, "\r\n")
	for _, responseLine := range responseLines {
		if rtpTimeExp.MatchString(responseLine) {
			rtpTimeString := rtpTimeExp.FindStringSubmatch(responseLine)[1]
			rtpTimeInt, err := strconv.ParseInt(rtpTimeString, 10, 64)
			if err != nil {
				return "", nil
			}
			client.StartVideoTimestamp = rtpTimeInt
			break
		}
	}

	client.IsPlaying = true

	if client.OnStartPlaying != nil {
		client.OnStartPlaying(client)
	}

	go client.run()
	go client.broadcastRTP()

	return response, err
}

func (client *RtspClient) Pause() (response string, err error) {
	if client.SessionId == 0 {
		return "", errors.New("для начала воспроизведения необходимо получить sessionId от сервера")
	}

	message := ""
	message += fmt.Sprintf("PAUSE %s RTSP/1.0\r\n", client.RemoteAddress)
	message += fmt.Sprintf("CSeq: %d\r\n", client.CSeq)
	message += fmt.Sprintf("Session:%d", client.SessionId)
	message += "Accept: application/sdp, application/rtsl, application/mheg\r\n"
	message += "\r\n"

	client.CSeq++
	_, err = client.TcpClient.SendString(message)
	if err != nil {
		return "", err
	}

	return client.ReadMessage()
}

func (client *RtspClient) TearDown() (response string, err error) {
	client.IsPlaying = false

	message := ""
	message += fmt.Sprintf("TEARDOWN %s RTSP/1.0\r\n", client.RemoteAddress)
	message += fmt.Sprintf("CSeq: %d\r\n", client.CSeq)
	message += fmt.Sprintf("Session:%d", client.SessionId)
	message += "Accept: application/sdp\r\n"
	message += "\r\n"

	client.CSeq++
	_, err = client.TcpClient.SendString(message)
	if err != nil {
		return "", err
	}

	response, err = client.ReadMessage()
	if err != nil {
		return "", err
	}

	err = client.Disconnect()
	if err != nil {
		return "", err
	}

	return response, err
}

const VIDEO = "video"
const AUDIO = "audio"

func (client *RtspClient) ReadMessage() (message string, err error) {
	contentLenExp := regexp.MustCompile("^[cC]ontent-[lL]ength:\\s*(\\d+)$")
	endOfResponse := false
	hasContent := false
	var contentLen int

	firstResponseLine, err := client.TcpClient.ReadLine()
	if err != nil {
		err = client.Disconnect()
		if err != nil {
			return "", err
		}
		return "", err
	}

	message += firstResponseLine + "\r\n"

	for !endOfResponse {
		responseLine, err := client.TcpClient.ReadLine()
		if err != nil {
			return "", nil
		}

		if responseLine == "" {
			message += "\r\n"
			if hasContent {
				sdpContent, _, _ := client.TcpClient.ReadBytes(contentLen)
				message += string(sdpContent)
			}
			endOfResponse = true
		} else if contentLenExp.MatchString(responseLine) {
			hasContent = true
			contentLenString := contentLenExp.FindStringSubmatch(responseLine)[1]
			contentLen, _ = strconv.Atoi(contentLenString)
		}

		message += responseLine + "\r\n"
	}

	logger.Junk(fmt.Sprintf("Rtsp client #%d: received message:", client.SessionId))
	logger.Junk(message)

	return message, err
}

func parseSdp(client *RtspClient, message *string) {
	_, client.Sdp = sdp.Parse(*message)
	client.RemoteStreamAddress = client.Sdp[0].Control

	for _, i2 := range client.Sdp {
		if i2.AVType != VIDEO && i2.AVType != AUDIO {
			continue
		}
		if i2.AVType == VIDEO {
			if i2.Type == av.H264 {
				if len(i2.SpropParameterSets) > 1 {
					if codecData, err := h264parser.NewCodecDataFromSPSAndPPS(i2.SpropParameterSets[0], i2.SpropParameterSets[1]); err == nil {
						//client.sps = i2.SpropParameterSets[0]
						//client.pps = i2.SpropParameterSets[1]
						client.Codecs = append(client.Codecs, codecData)
					}
				} else {
					client.Codecs = append(client.Codecs, h264parser.CodecData{})
					//client.WaitCodec = true
				}
				//client.FPS = i2.FPS
				client.VideoCodec = av.H264
			} else {
				logger.Error(fmt.Sprintf("SDP Video Codec Type Not Supported %s", i2.Type))
			}
		}
		client.VideoIDX = int8(len(client.Codecs) - 1)
	}

}

func (client *RtspClient) SubscribeToRtpBuff(uid int64, subscriber RtpSubscriber) {
	client.RtpSubscribers[uid] = subscriber
}

func (client *RtspClient) UnsubscribeFromRtpBuff(uid int64) {
	delete(client.RtpSubscribers, uid)
}

func (client *RtspClient) broadcastRTP() {
	var i int
	for client.IsConnected {

		recvRtpBuff := <-client.RTPVideoChan

		i++
		for _, subscriber := range client.RtpSubscribers {
			subscriber(&recvRtpBuff, i)
		}
	}
}

func (client *RtspClient) run() {
	for client.IsConnected && client.IsPlaying {
		if client.Transport == RtspTransportTcp {
			header := make([]byte, 4)
			_, err := io.ReadFull(client.TcpClient.IO, header)
			if err != nil {
				logger.Error(err.Error())
			}
			interleaved := header[0] == 0x24
			if interleaved {
				contentLen := int32(binary.BigEndian.Uint16(header[2:]))
				rtpPacket := make([]byte, contentLen+4)
				rtpPacket[0] = header[0]
				rtpPacket[1] = header[1]
				rtpPacket[2] = header[2]
				rtpPacket[3] = header[3]

				_, err := io.ReadFull(client.TcpClient.IO, rtpPacket[4:contentLen+4])
				if err != nil {
					logger.Error(err.Error())
				}

				/////VERY IMPORTANT NOTICE!!!
				///VIDEO AND AUDIO TRACK CHANNEL IDENTIFIERS ARE NOT CONSTANT!!!!
				////IDENTIFIERS COME FROM SDP ON SETUP STEP
				/////NORMALLY THEY ARE THE FIRST AVAILABLE NUMBERS 0 - video, 1 - audio
				////BUT UNDER ANY CIRCUMSTANCES THAT MAY CHANGE
				videoChannelId := byte(0)
				audioChannelId := byte(1)
				if rtpPacket[1] == videoChannelId {
					client.RTPVideoChan <- rtpPacket
				}
				if rtpPacket[1] == audioChannelId {
					client.RTPAudioChan <- rtpPacket
				}

			} else {
				logger.Warning(fmt.Sprintf("RTSP Interleaved frame error, header: %d %d %d %d", header[0], header[1], header[2], header[3]))
			}
		}
		if client.Transport == RtspTransportUdp {
			client.RTPVideoChan <- <-client.RtpServer.RecvBuff
		}
	}
}
