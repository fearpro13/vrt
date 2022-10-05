package rtsp_client

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec"
	"github.com/deepch/vdk/codec/aacparser"
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
	Transport string
	TcpClient *tcp_client.TcpClient

	RtpVideoServer *udp_server.UdpServer
	RtpAudioServer *udp_server.UdpServer

	RtpVideoClient *udp_client.UdpClient
	RtpAudioClient *udp_client.UdpClient

	SessionId     int32
	CSeq          int
	Auth          *digest.Auth
	RemoteAddress string
	Login         string
	Password      string

	VideoStreamAddress string //address for video track, not yet used
	AudioStreamAddress string //address for audio track, not yet used

	IsConnected                   bool
	IsPlaying                     bool
	StartVideoTimestamp           int64
	PreVideoTimestamp             int64
	RTPPacketFragmentationStarted bool
	RTPFragmentationBuffer        bytes.Buffer //used for NALU FU buffer
	Sdp                           []sdp.Media
	SdpRaw                        string
	Codecs                        []av.CodecData
	VideoCodec                    av.CodecType
	AudioCodec                    av.CodecType

	AudioTimeScale int64
	AudioTimeLine  time.Duration

	VideoId int
	AudioId int

	VideoIDX int8
	AudioIDX int8

	RtpSubscribers          map[int32]RtpSubscriber
	RTPVideoChan            chan []byte
	RTPAudioChan            chan []byte
	OnDisconnectListeners   map[int32]OnDisconnectCallback
	OnStartPlayingListeners map[int32]OnStartPlayingCallback
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
		RtpVideoServer:          udpServer,
		RtpVideoClient:          udpClient,
		RtpSubscribers:          map[int32]RtpSubscriber{},
		RTPVideoChan:            make(chan []byte, 1024),
		RTPAudioChan:            make(chan []byte, 1024),
		OnDisconnectListeners:   map[int32]OnDisconnectCallback{},
		OnStartPlayingListeners: map[int32]OnStartPlayingCallback{},
		VideoId:                 0,
		AudioId:                 2,
		AudioTimeScale:          8000,
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
		_, err = rtspClient.Describe()
		if err != nil {
			return err
		}

		_, err = rtspClient.Options()
		if err != nil {
			return err
		}

		_, err = rtspClient.Setup()
		if err != nil {
			return err
		}

		_, err = rtspClient.Play()
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
		if rtspClient.RtpVideoClient.IsConnected {
			err := rtspClient.RtpVideoClient.Disconnect()
			if err != nil {
				return err
			}
		}

		if rtspClient.RtpVideoServer.IsRunning {
			err := rtspClient.RtpVideoServer.Stop()
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

func (rtspClient *RtspClient) Describe() (response *rtsp.Response, err error) {
	request := rtsp.NewRequest(rtsp.DESCRIBE, rtspClient.RemoteAddress)
	request.AddHeader(rtsp.CSEQ, strconv.Itoa(rtspClient.CSeq))
	request.AddHeader("Accept", "application/sdp")

	rtspClient.CSeq++

	response, err = rtspClient.SendMessage(request)
	if err != nil {
		return nil, err
	}

	err = parseSdp(rtspClient, &response.Body)
	if err != nil {
		return nil, err
	}

	rtspClient.SdpRaw = response.Body

	return response, err
}

func (rtspClient *RtspClient) Options() (response *rtsp.Response, err error) {
	request := rtsp.NewRequest(rtsp.OPTIONS, rtspClient.RemoteAddress)
	request.AddHeader(rtsp.CSEQ, strconv.Itoa(rtspClient.CSeq))
	request.AddHeader("Accept", "application/sdp")

	rtspClient.CSeq++

	return rtspClient.SendMessage(request)
}

func (rtspClient *RtspClient) Setup() (response *rtsp.Response, err error) {
	if rtspClient.Transport == RtspTransportTcp {
		for _, media := range rtspClient.Sdp {
			var interleaveStart int
			var request *rtsp.Request
			switch media.AVType {
			case VIDEO:
				interleaveStart = rtspClient.VideoId
				request = rtsp.NewRequest(rtsp.SETUP, rtspClient.VideoStreamAddress)
			case AUDIO:
				interleaveStart = rtspClient.AudioId
				request = rtsp.NewRequest(rtsp.SETUP, rtspClient.AudioStreamAddress)
			default:
				return nil, errors.New(fmt.Sprintf("Unsupported media type %s", media.AVType))
			}

			request.AddHeader(rtsp.CSEQ, strconv.Itoa(rtspClient.CSeq))
			request.AddHeader("Accept", "application/media")

			request.AddHeader("Transport", fmt.Sprintf("RTP/AVP/TCP;unicast;interleaved=%d-%d", interleaveStart, interleaveStart+1))
			rtspClient.CSeq++

			response, err = rtspClient.SendMessage(request)

			sessionHeader, exist := response.GetHeader("Session")
			if exist {
				sessionExp := regexp.MustCompile("^(\\d+)")
				if sessionExp.MatchString(sessionHeader) {
					sessionIdString := sessionExp.FindStringSubmatch(sessionHeader)[1]
					sessionIdInt, err := strconv.Atoi(sessionIdString)
					if err != nil {
						return nil, err
					}
					rtspClient.SessionId = int32(sessionIdInt)
				}
			}
		}
	}

	if rtspClient.Transport == RtspTransportUdp {
		for _, media := range rtspClient.Sdp {
			request := rtsp.NewRequest(rtsp.SETUP, rtspClient.VideoStreamAddress)
			request.AddHeader(rtsp.CSEQ, strconv.Itoa(rtspClient.CSeq))
			request.AddHeader("Accept", "application/sdp")
			randPortInt := rand.Intn(64000) + 1024

			rtpServer := udp_server.Create()
			rtpServer.Port = randPortInt

			switch media.AVType {
			case VIDEO:
				rtspClient.RtpVideoServer = rtpServer
			case AUDIO:
				rtspClient.RtpAudioServer = rtpServer
			default:
				return nil, errors.New(fmt.Sprintf("Unsupported media type %s", media.AVType))
			}

			request.AddHeader("Transport", fmt.Sprintf("RTP/AVP/UDP;unicast;client_port=%d-%d", randPortInt, randPortInt+1))

			rtspClient.CSeq++

			response, err = rtspClient.SendMessage(request)

			var remoteRtpServerPort int
			transportHeader, exist := response.GetHeader("Transport")
			if exist {
				serverPortExp := regexp.MustCompile("server_port=(\\d+)-(\\d+)")
				if serverPortExp.MatchString(transportHeader) {
					serverPort := serverPortExp.FindStringSubmatch(transportHeader)[1]
					remoteRtpServerPort, err = strconv.Atoi(serverPort)
					if err != nil {
						return nil, err
					}
				} else {
					return nil, errors.New("remote server did not send server_port attribute")
				}
			}

			sessionHeader, exist := response.GetHeader("Session")
			if exist {
				sessionExp := regexp.MustCompile("^(\\d+)")
				if sessionExp.MatchString(sessionHeader) {
					sessionIdString := sessionExp.FindStringSubmatch(sessionHeader)[1]
					sessionIdInt, err := strconv.Atoi(sessionIdString)
					if err != nil {
						return nil, err
					}
					rtspClient.SessionId = int32(sessionIdInt)
				}
			}

			///////hair pinning/////////////////////
			udpClient := udp_client.Create()

			switch media.AVType {
			case VIDEO:
				udpClient.LocalPort = rtspClient.RtpVideoServer.Port
			case AUDIO:
				udpClient.LocalPort = rtspClient.RtpAudioServer.Port
			default:
				return nil, errors.New(fmt.Sprintf("Unsupported media type %s", media.AVType))
			}

			err = udpClient.Connect(rtspClient.TcpClient.Ip, remoteRtpServerPort)
			if err != nil {
				return nil, err
			}

			err = udpClient.SendMessage("HAIR PINNING;IGNORE THIS MESSAGE")
			if err != nil {
				return nil, err
			}

			err = udpClient.Disconnect()
			if err != nil {
				return nil, err
			}
			//////////////////////////////////////////

			err := rtpServer.Start("", udpClient.LocalPort)
			if err != nil {
				return nil, err
			}
		}
	}

	return response, err
}

func (rtspClient *RtspClient) Play() (response *rtsp.Response, err error) {
	if rtspClient.SessionId == 0 {
		return nil, errors.New("для начала воспроизведения необходимо получить sessionId от сервера")
	}

	request := rtsp.NewRequest(rtsp.PLAY, rtspClient.RemoteAddress)
	request.AddHeader(rtsp.CSEQ, strconv.Itoa(rtspClient.CSeq))
	request.AddHeader("Accept", "application/sdp")
	request.AddHeader("Session", strconv.Itoa(int(rtspClient.SessionId)))

	rtspClient.CSeq++

	response, err = rtspClient.SendMessage(request)

	if err != nil {
		return nil, err
	}

	rtpInfoHeader, exist := response.GetHeader("Rtp-Info")
	if exist {
		rtpTimeExp := regexp.MustCompile("rtptime=(\\d+)")
		if rtpTimeExp.MatchString(rtpInfoHeader) {
			rtpTimeString := rtpTimeExp.FindStringSubmatch(rtpInfoHeader)[1]
			rtpTimeInt, err := strconv.ParseInt(rtpTimeString, 10, 64)
			if err != nil {
				return nil, err
			}
			rtspClient.StartVideoTimestamp = rtpTimeInt
		}
	}

	rtspClient.IsPlaying = true

	for _, listener := range rtspClient.OnStartPlayingListeners {
		go listener(rtspClient)
	}

	go rtspClient.run()
	go rtspClient.broadcastRTP()

	return response, err
}

func (rtspClient *RtspClient) Pause() (response *rtsp.Response, err error) {
	if rtspClient.SessionId == 0 {
		return nil, errors.New("для начала воспроизведения необходимо получить sessionId от сервера")
	}

	request := rtsp.NewRequest(rtsp.PAUSE, rtspClient.RemoteAddress)
	request.AddHeader(rtsp.CSEQ, strconv.Itoa(rtspClient.CSeq))
	request.AddHeader("Accept", "application/sdp")
	request.AddHeader("Session", strconv.Itoa(int(rtspClient.SessionId)))
	request.AddHeader("Accept", "application/sdp")

	return rtspClient.SendMessage(request)
}

func (rtspClient *RtspClient) TearDown() (response *rtsp.Response, err error) {
	rtspClient.IsPlaying = false

	request := rtsp.NewRequest(rtsp.TEARDOWN, rtspClient.RemoteAddress)
	request.AddHeader(rtsp.CSEQ, strconv.Itoa(rtspClient.CSeq))
	request.AddHeader("Accept", "application/sdp")
	request.AddHeader("Session", strconv.Itoa(int(rtspClient.SessionId)))
	request.AddHeader("Accept", "application/sdp")

	return rtspClient.SendMessage(request)
}

const VIDEO = "video"
const AUDIO = "audio"

func (rtspClient *RtspClient) SendMessage(request *rtsp.Request) (response *rtsp.Response, err error) {
	if rtspClient.Auth != nil {
		request.AddHeader("Authorization", rtspClient.Auth.GetString())
	}

	message := request.ToString()
	_, err = rtspClient.TcpClient.SendString(message)
	if err != nil {
		return nil, err
	}

	response, err = rtspClient.ReadResponse()
	if err != nil {
		return nil, err
	}

	if response.Status == 401 && rtspClient.Auth != nil && rtspClient.Auth.Tried {
		return nil, errors.New("authorization failed")
	}

	if response.Status == 401 && rtspClient.Auth == nil {
		authHeader, exist := response.GetHeader("Www-authenticate")
		if exist {
			authExp := regexp.MustCompile("^(\\w+)\\s+realm=\"([^\"]+)\"\\s*,\\s*nonce=\"([^\"]+)\"\\s*.*$")
			if authExp.MatchString(authHeader) {
				authInfo := authExp.FindStringSubmatch(authHeader)
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
		} else {
			return nil, errors.New("authorization failed")
		}
	}

	if response.Status == 401 && rtspClient.Auth != nil && !rtspClient.Auth.Tried {
		rtspClient.Auth.Tried = true

		return rtspClient.SendMessage(request)
	}

	return response, err
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

func (rtspClient *RtspClient) ReadResponse() (response *rtsp.Response, err error) {
	contentLenExp := regexp.MustCompile("^[cC]ontent-[lL]ength:\\s*(\\d+)$")
	endOfResponse := false
	hasContent := false
	var contentLen int

	statusLineExp := regexp.MustCompile("^[rRtTsSpP]+/(\\d+.\\d+)\\s+(\\d+)\\s+(.*)$")

	firstResponseLine, err := rtspClient.TcpClient.ReadLine()
	if err != nil {
		err = rtspClient.Disconnect()
		if err != nil {
			return nil, err
		}
		return nil, err
	}

	if !statusLineExp.MatchString(firstResponseLine) {
		return nil, errors.New(fmt.Sprintf("RTSP client #%d: response does not have status line", rtspClient.SessionId))
	}

	message := ""
	message += firstResponseLine + "\r\n"

	for !endOfResponse {
		responseLine, err := rtspClient.TcpClient.ReadLine()
		if err != nil {
			return nil, err
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

	return rtsp.ParseResponse(message)
}

func parseSdp(client *RtspClient, message *string) (err error) {
	_, client.Sdp = sdp.Parse(*message)

	for _, mediaType := range client.Sdp {
		switch mediaType.AVType {
		case VIDEO:
			client.VideoStreamAddress = mediaType.Control
		case AUDIO:
			client.AudioStreamAddress = mediaType.Control
		default:
			return errors.New(fmt.Sprintf("Unsupported mediatype %s", mediaType.Type))
		}
	}

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
			client.VideoIDX = int8(len(client.Codecs) - 1)
		}
		if i2.AVType == AUDIO {
			var CodecData av.AudioCodecData
			switch i2.Type {
			case av.AAC:
				CodecData, err = aacparser.NewCodecDataFromMPEG4AudioConfigBytes(i2.Config)
				if err == nil {
					return errors.New("audio AAC bad config")
				}
			case av.OPUS:
				var cl av.ChannelLayout
				switch i2.ChannelCount {
				case 1:
					cl = av.CH_MONO
				case 2:
					cl = av.CH_STEREO
				default:
					cl = av.CH_MONO
				}
				CodecData = codec.NewOpusCodecData(i2.TimeScale, cl)
			case av.PCM_MULAW:
				CodecData = codec.NewPCMMulawCodecData()
			case av.PCM_ALAW:
				CodecData = codec.NewPCMAlawCodecData()
			case av.PCM:
				CodecData = codec.NewPCMCodecData()
			default:
				return errors.New(fmt.Sprintf("Audio Codec %s not supported", i2.Type))
			}
			if CodecData != nil {
				client.Codecs = append(client.Codecs, CodecData)
				client.AudioIDX = int8(len(client.Codecs) - 1)
				client.AudioCodec = CodecData.Type()
				if i2.TimeScale != 0 {
					//client.AudioTimeScale = int64(i2.TimeScale)
				}
			}
		}
	}

	return nil
}

func (rtspClient *RtspClient) SubscribeToRtpBuff(uid int32, subscriber RtpSubscriber) {
	rtspClient.RtpSubscribers[uid] = subscriber
}

func (rtspClient *RtspClient) UnsubscribeFromRtpBuff(uid int32) {
	delete(rtspClient.RtpSubscribers, uid)
}

func (rtspClient *RtspClient) broadcastRTP() {
	for rtspClient.IsConnected {
		select {
		case recvRtpVideoBuff := <-rtspClient.RTPVideoChan:
			for _, subscriber := range rtspClient.RtpSubscribers {
				subscriber(&recvRtpVideoBuff, rtspClient.VideoId)
			}
		case recvRtpAudioBuff := <-rtspClient.RTPAudioChan:
			for _, subscriber := range rtspClient.RtpSubscribers {
				subscriber(&recvRtpAudioBuff, rtspClient.AudioId)
			}
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
					switch int(rtpPacket[1]) {
					case rtspClient.VideoId:
						rtspClient.RTPVideoChan <- rtpPacket
					case rtspClient.AudioId:
						rtspClient.RTPAudioChan <- rtpPacket
					}
				} else {
					logger.Warning(fmt.Sprintf("RTSP Interleaved frame error, header: %d %d %d %d", header[0], header[1], header[2], header[3]))
				}
			}

			if rtspClient.Transport == RtspTransportUdp {
				select {
				case buff := <-rtspClient.RtpVideoServer.RecvBuff:
					rtspClient.RTPVideoChan <- buff
				case buff := <-rtspClient.RtpAudioServer.RecvBuff:
					rtspClient.RTPAudioChan <- buff
				}
			}
		}

		if errorCount > maxErrorCount {
			logger.Warning(fmt.Sprintf("RTSP rtspClient #%d: got %d errors while reading remote RTSP stream, rtspClient is disconnected", rtspClient.SessionId, errorCount))
			_ = rtspClient.Disconnect()
		}
	}
}
