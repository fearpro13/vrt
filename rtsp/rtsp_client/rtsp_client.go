package rtsp_client

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/format/rtsp/sdp"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"
	"vrt/logger"
	"vrt/tcp/tcp_client"
	"vrt/udp/udp_client"
	"vrt/udp/udp_server"
)

type RtspClient struct {
	TcpClient                     *tcp_client.TcpClient
	RtpServer                     *udp_server.UdpServer
	RtpClient                     *udp_client.UdpClient
	SessionId                     int64
	CSeq                          int
	RemoteAddress                 string
	RemoteStreamAddress           string
	IsConnected                   bool
	PreVideoTimestamp             int64
	RTPPacketFragmentationStarted bool
	RTPBuffer                     bytes.Buffer //used for NALU FU buffer
	Sdp                           []sdp.Media
	Codecs                        []av.CodecData
	VideoCodec                    av.CodecType
	VideoIDX                      int8
	RtpSubscribers                map[int64]RtpSubscriber
}

type RtpSubscriber func([]byte)

func Create() RtspClient {
	sessionId := rand.Int63()
	tcpClient := tcp_client.Create()
	udpServer := udp_server.Create()
	udpClient := udp_client.Create()

	return RtspClient{
		SessionId:      sessionId,
		CSeq:           1,
		TcpClient:      &tcpClient,
		RtpServer:      &udpServer,
		RtpClient:      &udpClient,
		RtpSubscribers: map[int64]RtpSubscriber{},
	}
}

func (client *RtspClient) Connect(address string) error {
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

	client.RemoteAddress = address
	client.IsConnected = true

	err := client.TcpClient.Connect(ip, portInt)

	logger.Info(fmt.Sprintf("RTSP client #%d connected", client.SessionId))

	go client.run()
	go client.broadcastRTP()

	return err
}

func (client *RtspClient) Disconnect() error {
	client.IsConnected = false

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

	err := client.TcpClient.Disconnect()

	logger.Info(fmt.Sprintf("RTSP client #%d disconnected", client.SessionId))

	return err
}

func (client *RtspClient) ReadMessage() (message string, err error) {
	message, err = client.TcpClient.ReadMessage()
	parseMessage(client, &message)
	return message, err
}

func (client *RtspClient) Describe() error {
	message := ""
	message += fmt.Sprintf("DESCRIBE %s RTSP/1.0\r\n", client.RemoteAddress)
	message += fmt.Sprintf("CSeq: %d\r\n", client.CSeq)
	message += "Accept: application/sdp, application/rtsl, application/mheg\r\n"
	message += "\r\n"

	client.CSeq++
	_, err := client.TcpClient.Send(message)

	return err
}

func (client *RtspClient) Options() error {
	message := ""
	message += fmt.Sprintf("OPTIONS %s RTSP/1.0\r\n", client.RemoteAddress)
	message += fmt.Sprintf("CSeq: %d\r\n", client.CSeq)
	message += "Accept: application/sdp, application/rtsl, application/mheg\r\n"
	message += "\r\n"

	client.CSeq++
	_, err := client.TcpClient.Send(message)

	return err
}

func (client *RtspClient) Setup() error {
	randPortInt := rand.Intn(64000) + 1024
	err := client.RtpServer.Start("", randPortInt-(randPortInt%2))

	if err != nil {
		return err
	}

	portMin := client.RtpServer.Port
	portMax := portMin + 1

	message := ""
	message += fmt.Sprintf("SETUP %s RTSP/1.0\r\n", client.RemoteStreamAddress)
	message += fmt.Sprintf("CSeq: %d\r\n", client.CSeq)
	message += fmt.Sprintf("Transport: RTP/AVP/UDP;unicast;client_port=%d-%d\r\n", portMin, portMax)
	message += "\r\n"

	client.CSeq++
	_, err = client.TcpClient.Send(message)

	return err
}

func (client *RtspClient) Play() error {
	if client.SessionId == 0 {
		return errors.New("для начала воспроизведения необходимо получить sessionId от сервера")
	}

	message := ""
	message += fmt.Sprintf("PLAY %s RTSP/1.0\r\n", client.RemoteAddress)
	message += fmt.Sprintf("CSeq: %d\r\n", client.CSeq)
	message += fmt.Sprintf("Session:%d\r\n", client.SessionId)
	message += "Accept: application/sdp, application/rtsl, application/mheg\r\n"
	message += "\r\n"

	client.CSeq++
	_, err := client.TcpClient.Send(message)

	return err
}

func (client *RtspClient) Pause() error {
	if client.SessionId == 0 {
		return errors.New("для начала воспроизведения необходимо получить sessionId от сервера")
	}

	message := ""
	message += fmt.Sprintf("PAUSE %s RTSP/1.0\r\n", client.RemoteAddress)
	message += fmt.Sprintf("CSeq: %d\r\n", client.CSeq)
	message += fmt.Sprintf("Session:%d", client.SessionId)
	message += "Accept: application/sdp, application/rtsl, application/mheg\r\n"
	message += "\r\n"

	client.CSeq++
	_, err := client.TcpClient.Send(message)

	return err
}

func (client *RtspClient) TearDown() error {
	if client.SessionId == 0 {
		return errors.New("для начала воспроизведения необходимо получить sessionId от сервера")
	}

	message := ""
	message += fmt.Sprintf("TEARDOWN %s RTSP/1.0\r\n", client.RemoteAddress)
	message += fmt.Sprintf("CSeq: %d\r\n", client.CSeq)
	message += fmt.Sprintf("Session:%d", client.SessionId)
	message += "Accept: application/sdp, application/rtsl, application/mheg\r\n"
	message += "\r\n"

	client.CSeq++
	_, err := client.TcpClient.Send(message)

	return err
}

const VIDEO = "video"
const AUDIO = "audio"

func parseMessage(client *RtspClient, message *string) {
	_, client.Sdp = sdp.Parse(*message)

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
		//client.videoID = client.chTMP
	}

	lines := strings.Split(*message, "\r\n")
	for _, line := range lines {
		matches, _ := regexp.MatchString("a=control", line)
		if matches {
			//a=control:rtsp://stream:Tv4m6ag6@10.3.43.140:554/trackID=1
			remoteStreamAddressExp, _ := regexp.Compile(":(.+)")
			remoteStreamAddress := remoteStreamAddressExp.FindStringSubmatch(line)[1]
			client.RemoteStreamAddress = remoteStreamAddress
		}

		matches, _ = regexp.MatchString("Session.*:.*\\d+", line)
		if matches {
			sessionIdExp, _ := regexp.Compile("\\d+")
			sessionIdString := sessionIdExp.FindString(line)
			sessionIdInt64, _ := strconv.ParseInt(sessionIdString, 10, 64)
			client.SessionId = sessionIdInt64
		}
	}
}

func (client *RtspClient) SubscribeToRtpBuff(uid int64, subscriber RtpSubscriber) {
	client.RtpSubscribers[uid] = subscriber
}

func (client *RtspClient) UnsubscribeFromRtpBuff(uid int64) {
	delete(client.RtpSubscribers, uid)
}

func (client *RtspClient) broadcastRTP() {
	for client.IsConnected {
		if client.RtpServer.IsRunning {
			recvRtpBuff := <-client.RtpServer.RecvBuff

			for _, subscriber := range client.RtpSubscribers {
				go subscriber(recvRtpBuff)
			}

			recvRtpBuff = recvRtpBuff[:0]
		} else {
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func (client *RtspClient) run() {
	for client.IsConnected {
		buff := <-client.TcpClient.RecvBuff
		if len(buff) > 0 {
			message := string(buff)
			parseMessage(client, &message)
			logger.Debug(message)
		}
	}
}
