package rtmp_client

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"
	"vrt/logger"
	"vrt/rtmp/AMF0"
	"vrt/tcp/tcp_client"
)

//message types

const SetChunkSize = 1
const AbortMessage = 2
const Acknowledgement = 3
const UserControlMessage = 4
const WindowAcknowledgementSize = 5
const SetPeerBandwidth = 6

//set window acknowledgement limit types

const LimitTypeHard = 0
const LimitTypeSoft = 1
const LimitTypeDynamic = 2

//user control message event types

const StreamBegin = 0
const StreamEOF = 1
const StreamDry = 2
const SetBufferLength = 3
const StreamLsRecorded = 4
const PingRequest = 6
const PingResponse = 7

type OnDisconnectCallback func(client *RtmpClient)
type RtmpReceiver func(*[]byte, int)

type RtmpClient struct {
	SessionId             int32
	TcpClient             *tcp_client.TcpClient
	OnDisconnectListeners map[int32]OnDisconnectCallback
	IsConnected           bool
	sync.Mutex
	RtmpBuff      chan []byte
	RtmpReceivers map[int32]RtmpReceiver
	windowSize    int
}

func Create() *RtmpClient {
	rtmpClient := &RtmpClient{
		SessionId:             rand.Int31(),
		OnDisconnectListeners: map[int32]OnDisconnectCallback{},
		RtmpBuff:              make(chan []byte, 1024),
		RtmpReceivers:         map[int32]RtmpReceiver{},
	}

	return rtmpClient
}

func CreateFromConnection(tcpClient *tcp_client.TcpClient) *RtmpClient {
	rtmpClient := Create()
	rtmpClient.TcpClient = tcpClient

	tcpClient.OnDisconnectListeners[rtmpClient.SessionId] = func(client *tcp_client.TcpClient) {
		rtmpClient.Disconnect()
	}

	rtmpClient.IsConnected = true

	logger.Info(fmt.Sprintf("RTMP client #%d connected", rtmpClient.SessionId))

	err := rtmpClient.BeginHandshake()
	if err != nil {
		logger.Error(err.Error())
		rtmpClient.Disconnect()
	}

	go rtmpClient.run()
	go rtmpClient.broadcastRtmp()

	return rtmpClient
}

func (rtmpClient *RtmpClient) Disconnect() {
	if !rtmpClient.IsConnected {
		return
	}

	rtmpClient.IsConnected = false

	if rtmpClient.TcpClient.IsConnected {
		_ = rtmpClient.TcpClient.Disconnect()
	}

	for _, listener := range rtmpClient.OnDisconnectListeners {
		go listener(rtmpClient)
	}

	logger.Info(fmt.Sprintf("RTMP client #%d disconnected", rtmpClient.SessionId))
}

func (rtmpClient *RtmpClient) SubscribeToRtmp(uid int32, receiver RtmpReceiver) {
	rtmpClient.Lock()
	rtmpClient.RtmpReceivers[uid] = receiver
	rtmpClient.Unlock()
}

func (rtmpClient *RtmpClient) UnsubscribeFromRtmp(uid int32) {
	rtmpClient.Lock()
	delete(rtmpClient.RtmpReceivers, uid)
	rtmpClient.Unlock()
}

func (rtmpClient *RtmpClient) BeginHandshake() error {
	tcpClient := rtmpClient.TcpClient
	c0 := make([]byte, 1)
	tcpClient.Socket.SetReadDeadline(time.Now().Add(tcpClient.ReadTimeout))
	tcpClient.IO.Read(c0)
	rtmpVersion := int(c0[0])
	logger.Junk(fmt.Sprintf("RTMP client #%d: Handshake c0 received, version %d ", rtmpClient.SessionId, rtmpVersion))

	tcpClient.Socket.SetWriteDeadline(time.Now().Add(tcpClient.WriteTimeout))
	tcpClient.IO.Write(c0)
	tcpClient.IO.Flush()

	c1 := make([]byte, 1536)
	tcpClient.Socket.SetReadDeadline(time.Now().Add(tcpClient.ReadTimeout))
	tcpClient.IO.Read(c1)
	hTime := binary.BigEndian.Uint32(c1[:4])
	zero := binary.BigEndian.Uint32(c1[4:8])
	logger.Junk(fmt.Sprintf("RTMP client #%d: Handshake c1 received, time %d ", rtmpClient.SessionId, hTime))
	if zero != 0 {
		return errors.New(fmt.Sprintf("RTMP client #%d: Handshake c1 received, zero header is not zero", rtmpClient.SessionId))
	}

	s1 := make([]byte, 1536)
	binary.BigEndian.PutUint32(s1[:4], hTime)
	binary.BigEndian.PutUint32(s1[4:8], 0)
	rand.Read(s1[8:])
	tcpClient.Socket.SetWriteDeadline(time.Now().Add(tcpClient.WriteTimeout))
	tcpClient.IO.Write(s1)
	tcpClient.IO.Flush()

	c2 := make([]byte, 1536)
	tcpClient.Socket.SetReadDeadline(time.Now().Add(tcpClient.ReadTimeout))
	tcpClient.IO.Read(c2)
	hTime1 := binary.BigEndian.Uint32(c2[:4])
	hTime2 := binary.BigEndian.Uint32(c2[4:8])

	logger.Junk(fmt.Sprintf("RTMP client #%d: Handshake c2 received, time1 %d ", rtmpClient.SessionId, hTime1))
	if hTime1 != hTime {
		return errors.New(fmt.Sprintf("RTMP client #%d: Handshake c2 received, but c2 time(%d) is not equal to time in s1(%d)", rtmpClient.SessionId, hTime1, hTime))
	}
	logger.Junk(fmt.Sprintf("RTMP client #%d: Handshake c2 received, time2 %d ", rtmpClient.SessionId, hTime2))
	//if hTime2 != hTime {
	//	return errors.New(fmt.Sprintf("RTMP client #%d: Handshake c2 received, but c2 time2(%d) is not equal to time in s1(%d)", rtmpClient.SessionId, hTime2, hTime))
	//}

	s2 := make([]byte, 1536)
	binary.BigEndian.PutUint32(s2[:4], hTime)
	binary.BigEndian.PutUint32(s2[4:8], hTime)
	copy(s2[8:], s1[8:])
	tcpClient.Socket.SetWriteDeadline(time.Now().Add(tcpClient.WriteTimeout))
	tcpClient.IO.Write(s2)
	tcpClient.IO.Flush()

	return nil
}

func (rtmpClient *RtmpClient) broadcastRtmp() {
	for rtmpClient.IsConnected {
		buff := <-rtmpClient.RtmpBuff
		for _, listener := range rtmpClient.RtmpReceivers {
			listener(&buff, 0)
		}
	}
}

func (rtmpClient *RtmpClient) readChunk() {
}

func (rtmpClient *RtmpClient) run() {
	for rtmpClient.IsConnected {

		//+--------------+----------------+--------------------+--------------+
		//| Basic Header | Message Header | Extended Timestamp |  Chunk Data  |
		//	+--------------+----------------+--------------------+--------------+
		//|                                                    |
		//|<------------------- Chunk Header ----------------->|

		bHeader := make([]byte, 1)
		rtmpClient.TcpClient.Socket.SetReadDeadline(time.Now().Add(rtmpClient.TcpClient.ReadTimeout))
		_, err := rtmpClient.TcpClient.IO.Read(bHeader[0:1])
		if err != nil {
			logger.Error(err.Error())
			rtmpClient.Disconnect()
		}
		//logger.Junk(pB(bHeader[0]))

		// 0 1 2 3 4 5 6 7
		//+-+-+-+-+-+-+-+-+
		//|fmt|   cs id   |
		//+-+-+-+-+-+-+-+-+

		chunkMessageTypeId := int((bHeader[0] >> 6) & 3) //1 2 3
		chunkStreamId := int(bHeader[0] & 63)            //bh1 - chunkStreamId(2-63), bh2 - chunkStreamId(64-319),bh3 - chunkStreamId(64-65599)
		switch chunkStreamId {
		case 64: //chunk basic header 3
			// 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3
			//+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
			//|fmt|0 0 0 0 0 1|          cs id - 64           |
			//+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
			bHeader := make([]byte, 3)
			rtmpClient.TcpClient.Socket.SetReadDeadline(time.Now().Add(rtmpClient.TcpClient.ReadTimeout))
			_, err := rtmpClient.TcpClient.IO.Read(bHeader[1:])
			if err != nil {
				logger.Error(err.Error())
				rtmpClient.Disconnect()
			}
			chunkStreamId = Uint16(bHeader[1:])
			logger.Junk(fmt.Sprintf("RTMP client #%d: Chunk basic header type 3(3 byte)", rtmpClient.SessionId))
		case 0: //chunk basic header 2
			// 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5
			//+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
			//|fmt|0 0 0 0 0 0|   cs id - 64  |
			//+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

			bHeader := make([]byte, 2)
			rtmpClient.TcpClient.Socket.SetReadDeadline(time.Now().Add(rtmpClient.TcpClient.ReadTimeout))
			_, err := rtmpClient.TcpClient.IO.Read(bHeader[1:])
			if err != nil {
				logger.Error(err.Error())
				rtmpClient.Disconnect()
			}
			chunkStreamId = int(bHeader[1])
			logger.Junk(fmt.Sprintf("RTMP client #%d: Chunk basic header type 2(2 byte)", rtmpClient.SessionId))
		default: //chunk basic header 1
			//already mapped
			logger.Junk(fmt.Sprintf("RTMP client #%d: Chunk basic header type 1(1 byte)", rtmpClient.SessionId))
		}

		logger.Junk(fmt.Sprintf("RTMP client #%d: Chunk basic header - chunk message type - %d, chunk stream id - %d",
			rtmpClient.SessionId, chunkMessageTypeId, chunkStreamId))

		////////////////////////////////////////////////////

		var messageLen int
		var messageTypeId int
		var messageStreamId int
		switch chunkMessageTypeId {
		case 0:
			//	0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
			//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
			//	|                 timestamp                     |message length |
			//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
			//  |      message length (cont)    |message type id| msg stream id |
			//  +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
			//  |            message stream id (cont)           |
			//  +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

			mHeader := make([]byte, 11)
			rtmpClient.TcpClient.Socket.SetReadDeadline(time.Now().Add(rtmpClient.TcpClient.ReadTimeout))
			_, err := rtmpClient.TcpClient.IO.Read(mHeader)
			if err != nil {
				logger.Error(err.Error())
				rtmpClient.Disconnect()
			}
			timestamp := Uint24(mHeader[0:3])
			messageLen = Uint24(mHeader[3:6])
			messageTypeId = int(mHeader[6])
			messageStreamId = int(binary.BigEndian.Uint32(mHeader[7:11]))
			logger.Junk(fmt.Sprintf("RTMP client #%d: Chunk message header - timestamp - %d,  message length - %d, message type id - %d, msg stream id - %d",
				rtmpClient.SessionId, timestamp, messageLen, messageTypeId, messageStreamId))
		case 1:
			//	0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
			//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
			//	|                timestamp delta                |message length |
			//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
			//  |     message length (cont)     |message type id|
			//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

			mHeader := make([]byte, 7)
			rtmpClient.TcpClient.Socket.SetReadDeadline(time.Now().Add(rtmpClient.TcpClient.ReadTimeout))
			_, err := rtmpClient.TcpClient.IO.Read(mHeader)
			if err != nil {
				logger.Error(err.Error())
				rtmpClient.Disconnect()
			}
			timestampDelta := Uint24(mHeader[0:3])
			messageLen = Uint24(mHeader[3:6])
			messageTypeId = int(mHeader[6])
			logger.Junk(fmt.Sprintf("RTMP client #%d: Chunk message header - type %d, timestamp delta - %d, message length - %d, message type id - %d",
				rtmpClient.SessionId, 1, timestampDelta, messageLen, messageTypeId))
		case 2:
			//0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3
			//+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
			//|               timestamp delta                 |
			//+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

			mHeader := make([]byte, 3)
			rtmpClient.TcpClient.Socket.SetReadDeadline(time.Now().Add(rtmpClient.TcpClient.ReadTimeout))
			_, err := rtmpClient.TcpClient.IO.Read(mHeader)
			if err != nil {
				logger.Error(err.Error())
				rtmpClient.Disconnect()
			}
			timestampDelta := Uint24(mHeader[:])
			logger.Junk(fmt.Sprintf("RTMP client #%d: Chunk message header - type %d, timestamp delta - %d",
				rtmpClient.SessionId, 1, timestampDelta))
		case 3:
		}

		var messageBody []byte
		if messageLen > 0 {
			messageBody = make([]byte, messageLen)
			rtmpClient.TcpClient.Socket.SetReadDeadline(time.Now().Add(rtmpClient.TcpClient.ReadTimeout))
			_, err = rtmpClient.TcpClient.IO.Read(messageBody)
			if err != nil {
				logger.Error(err.Error())
				rtmpClient.Disconnect()
			}
			logger.Junk(fmt.Sprintf("RTMP client #%d: Read %d bytes message",
				rtmpClient.SessionId, messageLen))
		}

		// chunkStreamId=2 - protocol control messages(chunk stream id =2 AND message stream id = 0)
		if messageStreamId == 0 {
			switch messageTypeId {
			case SetChunkSize: //set chunk size, default 128 bytes
				chunkSize := binary.BigEndian.Uint32(messageBody[0:4])
				rtmpClient.windowSize = int(chunkSize)
				logger.Junk(fmt.Sprintf("RTMP client #%d: Message: Set chunk size - %d",
					rtmpClient.SessionId, chunkSize))

				buff := make([]byte, 4)
				binary.BigEndian.PutUint32(buff[0:4], uint32(250000))
				rtmpClient.SendMessage(chunkStreamId, 0, 0, WindowAcknowledgementSize, messageStreamId, buff)

				buff = make([]byte, 5)
				binary.BigEndian.PutUint32(buff[0:4], uint32(250000))
				buff[4] = LimitTypeDynamic
				rtmpClient.SendMessage(chunkStreamId, 0, 0, SetPeerBandwidth, messageStreamId, buff)

				buff = make([]byte, 6)
				binary.BigEndian.PutUint16(buff[0:2], StreamBegin)
				rtmpClient.SendMessage(chunkStreamId, 0, 0, UserControlMessage, messageStreamId, buff)

				buff = make([]byte, 4)
				binary.BigEndian.PutUint32(buff[0:4], uint32(4096))
				rtmpClient.SendMessage(chunkStreamId, 0, 0, SetChunkSize, messageStreamId, buff)
			case AbortMessage: //abort message
				abortChunkStreamId := binary.BigEndian.Uint32(messageBody[0:4])
				logger.Junk(fmt.Sprintf("RTMP client #%d: Message: Abort message - chunk stream id - %d",
					rtmpClient.SessionId, abortChunkStreamId))
			case Acknowledgement: //acknowledgement
				sequenceNumber := binary.BigEndian.Uint32(messageBody[0:4])
				logger.Junk(fmt.Sprintf("RTMP client #%d: Message: Acknowledgement - sequence number - %d",
					rtmpClient.SessionId, sequenceNumber))
			case WindowAcknowledgementSize: //window acknowledgement size
				ackWindowSize := binary.BigEndian.Uint32(messageBody[0:4])
				logger.Junk(fmt.Sprintf("RTMP client #%d: Message: Window acknowledgement size - %d",
					rtmpClient.SessionId, ackWindowSize))
			case SetPeerBandwidth: //set peer bandwidth
				ackWindowSize := binary.BigEndian.Uint32(messageBody[0:4])
				limitType := int(messageBody[4]) // 0 - hard, 1- soft, 2- dynamic
				limitTypeString := ""
				switch limitType {
				case LimitTypeHard:
					limitTypeString = "Hard"
				case LimitTypeSoft:
					limitTypeString = "Soft"
				case LimitTypeDynamic:
					limitTypeString = "Dynamic"
				}
				logger.Junk(fmt.Sprintf("RTMP client #%d: Message: Set peer bandwidth : Window acknowledgement size - %d, limitType - %d(%s) ",
					rtmpClient.SessionId, ackWindowSize, limitType, limitTypeString))
			case 20, 17: //command message
				logger.Junk(fmt.Sprintf("RTMP client #%d: Received message type %d(%s)",
					rtmpClient.SessionId, messageTypeId, "Command message"))

				parsedMessage, err := AMF0.Parse(messageBody)
				if err != nil {
					logger.Error(err.Error())
					logger.Junk(fmt.Sprintf("AMF0: parsed %d entries", len(parsedMessage.Objects)))
				}

				buff := make([]byte, 4)
				binary.BigEndian.PutUint32(buff[0:4], uint32(250000))
				rtmpClient.SendMessage(chunkStreamId, 0, 0, 14, messageStreamId, buff)

			case 18, 15: //data message
				logger.Junk(fmt.Sprintf("RTMP client #%d: Received message type %d(%s)",
					rtmpClient.SessionId, messageTypeId, "Data message"))
			case 19, 16: //shared object message
				logger.Junk(fmt.Sprintf("RTMP client #%d: Received message type %d(%s)",
					rtmpClient.SessionId, messageTypeId, "Shared object message"))
			case 8: //audio message
				logger.Junk(fmt.Sprintf("RTMP client #%d: Received message type %d(%s)",
					rtmpClient.SessionId, messageTypeId, "Audio message"))
			case 9: //video message
				logger.Junk(fmt.Sprintf("RTMP client #%d: Received message type %d(%s)",
					rtmpClient.SessionId, messageTypeId, "Video message"))
			case 22: //aggregate message
				logger.Junk(fmt.Sprintf("RTMP client #%d: Received message type %d(%s)",
					rtmpClient.SessionId, messageTypeId, "Aggregate message"))
			case UserControlMessage:
				eventType := binary.BigEndian.Uint16(messageBody[0:2])
				switch eventType {
				case StreamBegin: //stream begin
				case StreamEOF: //stream eof
				case StreamDry: //stream dry
				case SetBufferLength: //set buffer length
				case StreamLsRecorded: //streamls recorded
				case PingRequest: //ping request
				case PingResponse: //ping response
				default:
					logger.Junk(fmt.Sprintf("RTMP client #%d: Unsupported User control message, Event type : type %d",
						rtmpClient.SessionId, eventType))
				}
			default:
				logger.Junk(fmt.Sprintf("RTMP client #%d: Received unsupported message type id %d",
					rtmpClient.SessionId, messageTypeId))
			}
		}

		//if payloadLen > 0 {
		//	message := make([]byte, payloadLen)
		//	rtmpClient.TcpClient.Socket.SetReadDeadline(time.Now().Add(rtmpClient.TcpClient.ReadTimeout))
		//	_, err := rtmpClient.TcpClient.IO.Read(message)
		//	if err != nil {
		//		logger.Error(err.Error())
		//		rtmpClient.Disconnect()
		//	}
		//	logger.Junk(fmt.Sprintf("RTMP client #%d: Read %d bytes message", rtmpClient.SessionId, payloadLen))
		//}

		//rtmpClient.RtmpBuff <- mHeader
		//logger.Debug(fmt.Sprintf("Received %d %d %d %d", header[0], header[1], header[2], header[3]))
	}
}

func (rtmpClient *RtmpClient) SendMessage(chunkStreamId int, chunkMessageType int, timestamp int, messageTypeId int, messageStreamId int, data []byte) {
	//+--------------+----------------+--------------------+--------------+
	//| Basic Header | Message Header | Extended Timestamp |  Chunk Data  |
	//	+--------------+----------------+--------------------+--------------+
	//|                                                    |
	//|<------------------- Chunk Header ----------------->|

	// 0 1 2 3 4 5 6 7
	//+-+-+-+-+-+-+-+-+
	//|fmt|   cs id   |
	//+-+-+-+-+-+-+-+-+

	bHeader := make([]byte, 1)
	bHeader[0] = byte((chunkStreamId) | (chunkMessageType << 6))

	//	0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	//	|                 timestamp                     |message length |
	//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	//  |      message length (cont)    |message type id| msg stream id |
	//  +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	//  |            message stream id (cont)           |
	//  +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

	mHeader := make([]byte, 11)
	PutUint24(mHeader[0:3], timestamp)
	PutUint24(mHeader[3:6], len(data))
	mHeader[6] = byte(messageTypeId)
	binary.BigEndian.PutUint32(mHeader[7:], uint32(messageStreamId))

	message := make([]byte, len(bHeader)+len(mHeader)+len(data))
	copy(message[0:len(bHeader)], bHeader)
	copy(message[len(bHeader):len(bHeader)+len(mHeader)], mHeader)
	copy(message[len(bHeader)+len(mHeader):], data)

	rtmpClient.TcpClient.Socket.SetWriteDeadline(time.Now().Add(rtmpClient.TcpClient.WriteTimeout))
	_, err := rtmpClient.TcpClient.IO.Write(message)
	if err != nil {
		logger.Error(err.Error())
		rtmpClient.Disconnect()
	}
	err = rtmpClient.TcpClient.IO.Flush()
	if err != nil {
		logger.Error(err.Error())
		rtmpClient.Disconnect()
	}
}

func Uint24(bytes []byte) int {
	if len(bytes) != 3 {
		panic("byte count should be equal to 3")
	}

	buff := make([]byte, 4)
	copy(buff[1:], bytes)

	return int(binary.BigEndian.Uint32(buff))
}
func Uint16(bytes []byte) int {
	if len(bytes) != 2 {
		panic("byte count should be equal to 2")
	}

	return Uint16(bytes)
}

func PutUint24(bytes []byte, int24 int) {
	if len(bytes) != 3 {
		panic("byte count should be equal to 3")
	}

	bytes[0] = byte(int24 >> 16)
	bytes[1] = byte(int24 >> 8)
	bytes[2] = byte(int24)
}

func pB(b byte) string {
	return fmt.Sprintf("%d%d%d%d%d%d%d%d", b&128/128, b&64/64, b&32/32, b&16/16, b&8/8, b&4/4, b&2/2, b&1)
}
