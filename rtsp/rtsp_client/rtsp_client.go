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
	"time"
	"vrt/auth/digest"
	"vrt/logger"
	"vrt/rtsp"
	"vrt/tcp/tcp_client"
	"vrt/udp/udp_client"
	"vrt/udp/udp_server"
)

const RtspTransportTcp = "tcp"
const RtspTransportUdp = "udp"

type OnDisconnectCallback func(client *RtspClient)
type OnStartPlayingCallback func(client *RtspClient)
type RtpSubscriber func(*[]byte, int)

type RtspClient struct {
	Transport                     string
	TcpClient                     *tcp_client.TcpClient
	RtpServer                     *udp_server.UdpServer
	RtpClient                     *udp_client.UdpClient
	SessionId                     int32
	CSeq                          int
	Auth                          *digest.Auth
	RemoteAddress                 string
	Login                         string
	Password                      string
	RemoteStreamAddress           string //contains trackId or e.t.c
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
	RtpSubscribers                map[int32]RtpSubscriber
	RTPVideoChan                  chan []byte
	RTPAudioChan                  chan []byte
	OnDisconnectListeners         map[int32]OnDisconnectCallback
	OnStartPlayingListeners       map[int32]OnStartPlayingCallback
}

func Create() *RtspClient {
	sessionId := rand.Int31()
	tcpClient := tcp_client.Create()
	udpServer := udp_server.Create()
	udpClient := udp_client.Create()

	rtspClient := &RtspClient{
		SessionId:               sessionId,
		CSeq:                    0,
		TcpClient:               tcpClient,
		RtpServer:               udpServer,
		RtpClient:               udpClient,
		RtpSubscribers:          map[int32]RtpSubscriber{},
		RTPVideoChan:            make(chan []byte, 1024),
		RTPAudioChan:            make(chan []byte, 1024),
		OnDisconnectListeners:   map[int32]OnDisconnectCallback{},
		OnStartPlayingListeners: map[int32]OnStartPlayingCallback{},
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

func (rtspClient *RtspClient) Connect(address string, transport string) error {
	if transport == "" {
		transport = RtspTransportUdp
	}
	rtspClient.Transport = transport

	hasRtspExp := regexp.MustCompile("^rtsp:/{2}")
	hasCredsExp := regexp.MustCompile("(\\w+):(\\w+)")

	hasRtsp := hasRtspExp.MatchString(address)
	hasCreds := hasCredsExp.MatchString(address)

	if !hasRtsp {
		return errors.New(fmt.Sprintf("RTSP rtspClient #%d: Отсутствует префикс rtsp://", rtspClient.SessionId))
	}

	if !hasCreds {
		return errors.New(fmt.Sprintf("RTSP rtspClient #%d: отсутсвуют данные для авторизации BasicAuth", rtspClient.SessionId))
	}

	creds := hasCredsExp.FindStringSubmatch(address)
	rtspClient.Login = creds[1]
	rtspClient.Password = creds[2]

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

	rtspClient.TcpClient.OnDisconnectListeners[rtspClient.SessionId] = func(client *tcp_client.TcpClient) {
		_ = rtspClient.Disconnect()
	}

	err := rtspClient.TcpClient.Connect(ip, portInt)

	if err != nil {
		return err
	}

	rtspClient.RemoteAddress = address
	rtspClient.IsConnected = true

	logger.Info(fmt.Sprintf("RTSP rtspClient #%d connected, transport: %s", rtspClient.SessionId, transport))

	return err
}

func (rtspClient *RtspClient) ConnectAndPlay(address string, transport string) error {
	err := rtspClient.Connect(address, transport)
	if err != nil {
		return err
	}

	if !rtspClient.IsPlaying {
		_, _, err = rtspClient.Describe()
		if err != nil {
			return err
		}

		_, _, err = rtspClient.Options()
		if err != nil {
			return err
		}

		_, _, err = rtspClient.Setup()
		if err != nil {
			return err
		}

		_, _, err = rtspClient.Play()
		if err != nil {
			return err
		}
	}

	return err
}

func (rtspClient *RtspClient) Disconnect() error {
	if !rtspClient.IsConnected {
		return nil
	}

	rtspClient.IsConnected = false

	for uid := range rtspClient.RtpSubscribers {
		rtspClient.UnsubscribeFromRtpBuff(uid)
	}

	if rtspClient.Transport == RtspTransportUdp {
		/*
			Если rtsp клиент используется для подключения к удалённому серверу - RTP rtspClient не создаётся, создаётся только RTP сервер
			Если входящее соединение рассматривается как RTP клиент - RTP rtspClient создаётся, но не создаётся RTP сервер
		*/
		if rtspClient.RtpClient.IsConnected {
			err := rtspClient.RtpClient.Disconnect()
			if err != nil {
				return err
			}
		}

		if rtspClient.RtpServer.IsRunning {
			err := rtspClient.RtpServer.Stop()
			if err != nil {
				return err
			}
		}
	}

	if rtspClient.TcpClient.IsConnected {
		err := rtspClient.TcpClient.Disconnect()
		if err != nil {
			return err
		}
	}

	for _, listener := range rtspClient.OnDisconnectListeners {
		go listener(rtspClient)
	}

	logger.Info(fmt.Sprintf("RTSP rtspClient #%d disconnected", rtspClient.SessionId))

	return nil
}

func (rtspClient *RtspClient) Describe() (status int, response string, err error) {
	request := rtsp.NewRequest(rtsp.DESCRIBE, rtspClient.RemoteAddress)
	request.AddHeader(rtsp.CSeq, strconv.Itoa(rtspClient.CSeq))
	request.AddHeader("Accept", "application/sdp")

	rtspClient.CSeq++

	status, response, err = rtspClient.SendMessage(request)

	parseSdp(rtspClient, &response)

	return status, response, err
}

func (rtspClient *RtspClient) Options() (status int, response string, err error) {
	request := rtsp.NewRequest(rtsp.OPTIONS, rtspClient.RemoteAddress)
	request.AddHeader(rtsp.CSeq, strconv.Itoa(rtspClient.CSeq))
	request.AddHeader("Accept", "application/sdp")

	rtspClient.CSeq++

	status, response, err = rtspClient.SendMessage(request)

	return status, response, err
}

func (rtspClient *RtspClient) Setup() (status int, response string, err error) {
	request := rtsp.NewRequest(rtsp.SETUP, rtspClient.RemoteStreamAddress)
	request.AddHeader(rtsp.CSeq, strconv.Itoa(rtspClient.CSeq))
	request.AddHeader("Accept", "application/sdp")

	rtspClient.CSeq++

	if rtspClient.Transport == RtspTransportTcp {
		request.AddHeader("Transport", "RTP/AVP/TCP;unicast;interleaved=0-1")
	}

	if rtspClient.Transport == RtspTransportUdp {
		randPortInt := rand.Intn(64000) + 1024
		rtspClient.LocalRtpServerPort = randPortInt

		request.AddHeader("Transport", fmt.Sprintf("RTP/AVP/UDP;unicast;client_port=%d-%d", randPortInt, randPortInt+1))
	}

	status, response, err = rtspClient.SendMessage(request)

	lines := strings.Split(response, "\r\n")
	for _, line := range lines {
		serverPortExp := regexp.MustCompile("server_port=(\\d+)-(\\d+)")
		if serverPortExp.MatchString(line) {
			serverPort := serverPortExp.FindStringSubmatch(line)[1]
			rtspClient.RemoteRtpServerPort, err = strconv.Atoi(serverPort)
			if err != nil {
				return status, "", err
			}
		}

		sessionExp := regexp.MustCompile("^[sS]ession:\\s+(\\d+)")
		if sessionExp.MatchString(line) {
			sessionIdString := sessionExp.FindStringSubmatch(line)[1]
			sessionIdInt, err := strconv.Atoi(sessionIdString)
			if err != nil {
				return status, "", err
			}
			rtspClient.SessionId = int32(sessionIdInt)
		}
	}

	if rtspClient.Transport == RtspTransportUdp {
		///////hair pinning/////////////////////
		udpClient := udp_client.Create()
		udpClient.LocalPort = rtspClient.LocalRtpServerPort
		err = udpClient.Connect(rtspClient.TcpClient.Ip, rtspClient.RemoteRtpServerPort)
		if err != nil {
			return status, "", err
		}

		err = udpClient.SendMessage("HAIR PINNING;IGNORE THIS MESSAGE")
		if err != nil {
			return status, "", err
		}

		err = udpClient.Disconnect()
		if err != nil {
			return status, "", err
		}
		//////////////////////////////////////////

		rtpServer := udp_server.Create()
		rtspClient.RtpServer = rtpServer

		err := rtpServer.Start("", rtspClient.LocalRtpServerPort)
		if err != nil {
			return status, "", err
		}
	}

	return status, response, err
}

func (rtspClient *RtspClient) Play() (status int, response string, err error) {
	if rtspClient.SessionId == 0 {
		return 0, "", errors.New("для начала воспроизведения необходимо получить sessionId от сервера")
	}

	request := rtsp.NewRequest(rtsp.PLAY, rtspClient.RemoteAddress)
	request.AddHeader(rtsp.CSeq, strconv.Itoa(rtspClient.CSeq))
	request.AddHeader("Accept", "application/sdp")
	request.AddHeader("Session", strconv.Itoa(int(rtspClient.SessionId)))
	request.AddHeader("Accept", "application/sdp")

	rtspClient.CSeq++

	status, response, err = rtspClient.SendMessage(request)

	if err != nil {
		return 0, "", err
	}

	rtpTimeExp := regexp.MustCompile("rtptime=(\\d+)")
	responseLines := strings.Split(response, "\r\n")
	for _, responseLine := range responseLines {
		if rtpTimeExp.MatchString(responseLine) {
			rtpTimeString := rtpTimeExp.FindStringSubmatch(responseLine)[1]
			rtpTimeInt, err := strconv.ParseInt(rtpTimeString, 10, 64)
			if err != nil {
				return 0, "", nil
			}
			rtspClient.StartVideoTimestamp = rtpTimeInt
			break
		}
	}

	rtspClient.IsPlaying = true

	for _, listener := range rtspClient.OnStartPlayingListeners {
		go listener(rtspClient)
	}

	go rtspClient.run()
	go rtspClient.broadcastRTP()

	return status, response, err
}

func (rtspClient *RtspClient) Pause() (status int, response string, err error) {
	if rtspClient.SessionId == 0 {
		return 0, "", errors.New("для начала воспроизведения необходимо получить sessionId от сервера")
	}

	request := rtsp.NewRequest(rtsp.PAUSE, rtspClient.RemoteAddress)
	request.AddHeader(rtsp.CSeq, strconv.Itoa(rtspClient.CSeq))
	request.AddHeader("Accept", "application/sdp")
	request.AddHeader("Session", strconv.Itoa(int(rtspClient.SessionId)))
	request.AddHeader("Accept", "application/sdp")

	return rtspClient.SendMessage(request)
}

func (rtspClient *RtspClient) TearDown() (status int, response string, err error) {
	rtspClient.IsPlaying = false

	request := rtsp.NewRequest(rtsp.TEARDOWN, rtspClient.RemoteAddress)
	request.AddHeader(rtsp.CSeq, strconv.Itoa(rtspClient.CSeq))
	request.AddHeader("Accept", "application/sdp")
	request.AddHeader("Session", strconv.Itoa(int(rtspClient.SessionId)))
	request.AddHeader("Accept", "application/sdp")

	return rtspClient.SendMessage(request)
}

const VIDEO = "video"
const AUDIO = "audio"

func (rtspClient *RtspClient) SendMessage(request *rtsp.Request) (status int, response string, err error) {
	if rtspClient.Auth != nil {
		request.AddHeader("Authorization", rtspClient.Auth.GetString())
	}

	message := request.ToString()
	_, err = rtspClient.TcpClient.SendString(message)
	if err != nil {
		return 0, "", err
	}

	status, response, err = rtspClient.ReadResponse()
	if err != nil {
		return status, "", err
	}

	if status == 401 && rtspClient.Auth != nil && rtspClient.Auth.Tried {
		return status, response, errors.New("authorization failed")
	}

	if status == 401 && rtspClient.Auth == nil {
		authExp := regexp.MustCompile("^[wW]{3}-[aA]uthenticate:\\s+(\\w+)\\s+realm=\"([^\"]+)\"\\s*,\\s*nonce=\"([^\"]+)\"\\s*.*$")

		messageLines := strings.Split(response, "\r\n")

		for _, line := range messageLines {
			if authExp.MatchString(line) {
				authInfo := authExp.FindStringSubmatch(line)
				//authType := authInfo[1]
				authRealm := authInfo[2]
				authNonce := authInfo[3]
				if rtspClient.Auth == nil {
					rtspClient.Auth = &digest.Auth{
						Username: rtspClient.Login,
						Realm:    authRealm,
						Password: rtspClient.Password,
						Method:   request.Method,
						Uri:      rtspClient.RemoteAddress,
						Nonce:    authNonce,
					}
				} else {
					rtspClient.Auth.Nonce = authNonce
				}
			}
		}
	}

	if status == 401 && rtspClient.Auth != nil && !rtspClient.Auth.Tried {
		//request.AddHeader("Authorization", rtspClient.Auth.GetString())
		rtspClient.Auth.Tried = true

		return rtspClient.SendMessage(request)
	}

	return status, response, err
}

func (rtspClient *RtspClient) ReadRequest() (message string, err error) {
	contentLenExp := regexp.MustCompile("^[cC]ontent-[lL]ength:\\s*(\\d+)$")
	endOfResponse := false
	hasContent := false
	var contentLen int

	for !endOfResponse {
		responseLine, err := rtspClient.TcpClient.ReadLine()
		if err != nil {
			return "", nil
		}

		if responseLine == "" {
			message += "\r\n"
			if hasContent {
				sdpContent, _, _ := rtspClient.TcpClient.ReadBytes(contentLen)
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

	logger.Junk(fmt.Sprintf("Rtsp rtspClient #%d: received message:", rtspClient.SessionId))
	logger.Junk(message)

	return message, err
}

func (rtspClient *RtspClient) ReadResponse() (status int, message string, err error) {
	contentLenExp := regexp.MustCompile("^[cC]ontent-[lL]ength:\\s*(\\d+)$")
	endOfResponse := false
	hasContent := false
	var contentLen int

	statusLineExp := regexp.MustCompile("^[rR][tT][sS][pP]/(\\d+.\\d+)\\s+(\\d+)\\s+(\\w+)$")

	firstResponseLine, err := rtspClient.TcpClient.ReadLine()
	if err != nil {
		err = rtspClient.Disconnect()
		if err != nil {
			return 0, "", err
		}
		return 0, "", err
	}

	if !statusLineExp.MatchString(firstResponseLine) {
		return 0, "", errors.New(fmt.Sprintf("RTSP client #%d: response does not have status line", rtspClient.SessionId))
	}

	statusLine := statusLineExp.FindStringSubmatch(firstResponseLine)
	//rtspVersion := statusLine[1]
	statusCode, _ := strconv.Atoi(statusLine[2])
	//reason := statusLine[3]

	message += firstResponseLine + "\r\n"

	for !endOfResponse {
		responseLine, err := rtspClient.TcpClient.ReadLine()
		if err != nil {
			return statusCode, "", nil
		}

		if responseLine == "" {
			message += "\r\n"
			if hasContent {
				sdpContent, _, _ := rtspClient.TcpClient.ReadBytes(contentLen)
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

	logger.Junk(fmt.Sprintf("Rtsp rtspClient #%d: received message:", rtspClient.SessionId))
	logger.Junk(message)

	return statusCode, message, err
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

func (rtspClient *RtspClient) SubscribeToRtpBuff(uid int32, subscriber RtpSubscriber) {
	rtspClient.RtpSubscribers[uid] = subscriber
}

func (rtspClient *RtspClient) UnsubscribeFromRtpBuff(uid int32) {
	delete(rtspClient.RtpSubscribers, uid)
}

func (rtspClient *RtspClient) broadcastRTP() {
	//var i int
	for rtspClient.IsConnected {

		recvRtpBuff := <-rtspClient.RTPVideoChan

		//i++
		for _, subscriber := range rtspClient.RtpSubscribers {
			subscriber(&recvRtpBuff, 0)
		}
	}
}

func (rtspClient *RtspClient) run() {
	errorCount := 0
	maxErrorCount := 5
	for rtspClient.IsConnected && rtspClient.IsPlaying {
		if rtspClient.Transport == RtspTransportTcp {
			header := make([]byte, 4)
			err := rtspClient.TcpClient.Socket.SetReadDeadline(time.Now().Add(rtspClient.TcpClient.ReadTimeout))
			if err != nil {
				logger.Error(err.Error())
				errorCount++
			}

			_, err = io.ReadFull(rtspClient.TcpClient.IO, header)
			if err != nil {
				logger.Error(err.Error())
				errorCount++
			} else {
				interleaved := header[0] == 0x24
				if interleaved {
					contentLen := int32(binary.BigEndian.Uint16(header[2:]))
					rtpPacket := make([]byte, contentLen+4)
					rtpPacket[0] = header[0]
					rtpPacket[1] = header[1]
					rtpPacket[2] = header[2]
					rtpPacket[3] = header[3]

					err := rtspClient.TcpClient.Socket.SetReadDeadline(time.Now().Add(rtspClient.TcpClient.ReadTimeout))
					if err != nil {
						logger.Error(err.Error())
						errorCount++
					}

					_, err = io.ReadFull(rtspClient.TcpClient.IO, rtpPacket[4:contentLen+4])
					if err != nil {
						logger.Error(err.Error())
						errorCount++
					}

					/////VERY IMPORTANT NOTICE!!!
					///VIDEO AND AUDIO TRACK CHANNEL IDENTIFIERS ARE NOT CONSTANT!!!!
					////IDENTIFIERS COME FROM SDP ON SETUP STEP
					/////NORMALLY THEY ARE THE FIRST AVAILABLE NUMBERS 0 - video, 1 - audio
					////BUT UNDER ANY CIRCUMSTANCES THAT MAY CHANGE
					videoChannelId := byte(0)
					audioChannelId := byte(1)
					if rtpPacket[1] == videoChannelId {
						rtspClient.RTPVideoChan <- rtpPacket
					}
					if rtpPacket[1] == audioChannelId {
						rtspClient.RTPAudioChan <- rtpPacket
					}

				} else {
					logger.Warning(fmt.Sprintf("RTSP Interleaved frame error, header: %d %d %d %d", header[0], header[1], header[2], header[3]))
				}
			}
			if rtspClient.Transport == RtspTransportUdp {
				rtspClient.RTPVideoChan <- <-rtspClient.RtpServer.RecvBuff
			}
		}

		if errorCount > maxErrorCount {
			logger.Warning(fmt.Sprintf("RTSP rtspClient #%d: got %d errors while reading remote RTSP stream, rtspClient is disconnected", rtspClient.SessionId, errorCount))
			_ = rtspClient.Disconnect()
		}
	}
}
