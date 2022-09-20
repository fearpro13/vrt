package rtsp_to_ws

import (
	"encoding/binary"
	"fmt"
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/format/mp4f"
	"github.com/gorilla/websocket"
	"log"
	"math"
	"time"
	"vrt/logger"
	"vrt/rtsp/rtsp_client"
	"vrt/ws/ws_client"
	"vrt/ws/ws_server"
)

type Broadcast struct {
	IsRunning  bool
	rtspClient *rtsp_client.RtspClient
	wsServer   *ws_server.WSServer
	clients    map[int64]*ws_client.WSClient
}

func (broadcast *Broadcast) Stop() {
	for _, wsClient := range broadcast.wsServer.Clients {
		broadcast.rtspClient.UnsubscribeFromRtpBuff(wsClient.SessionId)
		wsClient.SetCallback(nil)
	}
	broadcast.wsServer.Callback = nil
	broadcast.IsRunning = false

	logger.Info(fmt.Sprintf("RTSP client #%d broadcast to #%d Websocket server stopped", broadcast.rtspClient.SessionId, broadcast.wsServer.SessionId))
}

func BroadcastRtspClientToWebsockets(rtspClient *rtsp_client.RtspClient, wsServer *ws_server.WSServer) *Broadcast {
	broadcast := &Broadcast{
		IsRunning:  true,
		rtspClient: rtspClient,
		wsServer:   wsServer,
		clients:    map[int64]*ws_client.WSClient{},
	}

	wsServer.Callback = func(client *ws_client.WSClient) {
		broadcast.clients[client.SessionId] = client
		client.SetCallback(func(client *ws_client.WSClient) {
			delete(broadcast.clients, client.SessionId)
			rtspClient.UnsubscribeFromRtpBuff(client.SessionId)
		})

		muxer := mp4f.NewMuxer(nil)
		codecs := rtspClient.Codecs
		err := muxer.WriteHeader(codecs)
		if err != nil {
			log.Println("muxer.WriteHeader", err)
			return
		}
		meta, init := muxer.GetInit(codecs)

		client.Send(websocket.BinaryMessage, append([]byte{9}, meta...))
		client.Send(websocket.BinaryMessage, init)

		var start = false

		rtspClient.SubscribeToRtpBuff(client.SessionId, func(bytes []byte) {
			interleavedFakeFrame := make([]byte, 4)
			interleavedFakeFrame[0] = 36
			interleavedFakeFrame[1] = bytes[1] //96 = videoID RTP format from SDP
			payloadSizeBytes := int16ToBytes(len(bytes))
			interleavedFakeFrame[2] = payloadSizeBytes[0]
			interleavedFakeFrame[3] = payloadSizeBytes[1]

			bytes = append(interleavedFakeFrame, bytes...)

			packets, _ := rtpDemux(rtspClient, &bytes)

			for _, packet := range packets {
				if packet.IsKeyFrame {
					start = true
				}
				if !start {
					continue
				}

				_, hRaw, _ := muxer.WritePacket(*packet, false)

				if len(hRaw) > 0 {
					client.Send(websocket.BinaryMessage, hRaw)
				}

			}
		})
	}

	logger.Info(fmt.Sprintf("RTSP client #%d broadcast to #%d Websocket server started", rtspClient.SessionId, wsServer.SessionId))

	return broadcast
}

func BroadcastRtspToWebsockets(rtspAddress string, wsServer *ws_server.WSServer) {

}

func extractRTPFromInterleavedFrame(header []byte) []byte {
	h0 := header[0]
	switch h0 {
	case 0x24:
		length := int32(binary.BigEndian.Uint16(header[2:]))
		if length > 65535 || length < 12 {
			logger.Error("RTSP Client RTP Incorrect Packet Size")
			return []byte{}
		}
		content := make([]byte, length+4)
		content[0] = header[0]
		content[1] = header[1]
		content[2] = header[2]
		content[3] = header[3]
		content = append(content, header[4:]...)

		return content
	default:
		logger.Error("RTSP Client RTP Read DeSync")
		return []byte{}
	}
}

func rtpDemux(rtspClient *rtsp_client.RtspClient, payloadRAW *[]byte) ([]*av.Packet, bool) {
	content := *payloadRAW
	firstByte := content[4]
	padding := (firstByte>>5)&1 == 1
	extension := (firstByte>>4)&1 == 1
	CSRCCnt := int(firstByte & 0x0f)
	//SequenceNumber := int(binary.BigEndian.Uint16(content[6:8]))
	timestamp := int64(binary.BigEndian.Uint32(content[8:16]))

	offset := 12

	end := len(content)
	if end-offset >= 4*CSRCCnt {
		offset += 4 * CSRCCnt
	}
	if extension && len(content) < 4+offset+2+2 {
		return nil, false
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
		if rtspClient.PreVideoTimestamp == 0 {
			rtspClient.PreVideoTimestamp = timestamp
		}
		if timestamp-rtspClient.PreVideoTimestamp < 0 {
			if math.MaxUint32-rtspClient.PreVideoTimestamp < 90*100 { //100 ms
				rtspClient.PreVideoTimestamp = 0
				rtspClient.PreVideoTimestamp -= math.MaxUint32 - rtspClient.PreVideoTimestamp
			} else {
				rtspClient.PreVideoTimestamp = 0
			}
		}
		//if client.PreSequenceNumber != 0 && SequenceNumber-client.PreSequenceNumber != 1 {
		//	client.Println("drop packet", SequenceNumber-1)
		//}
		//client.PreSequenceNumber = SequenceNumber
		if rtspClient.RTPBuffer.Len() > 4048576 {
			logger.Notice("Big Buffer Flush")
			rtspClient.RTPBuffer.Truncate(0)
			rtspClient.RTPBuffer.Reset()
		}
		nalRaw, _ := h264parser.SplitNALUs(content[offset:end])
		if len(nalRaw) == 0 || len(nalRaw[0]) == 0 {
			return nil, false
		}
		var retmap []*av.Packet
		for _, nal := range nalRaw {
			if true {
				naluType := nal[0] & 0x1f
				switch {
				case naluType >= 1 && naluType <= 5:
					retmap = append(retmap, &av.Packet{
						Data:            append(int32ToBytes(len(nal)), nal...),
						CompositionTime: time.Duration(1) * time.Millisecond,
						Idx:             rtspClient.VideoIDX,
						IsKeyFrame:      naluType == 5,
						Duration:        time.Duration(float32(timestamp-rtspClient.PreVideoTimestamp)/90) * time.Millisecond,
						Time:            time.Duration(timestamp/90) * time.Millisecond,
					})
				case naluType == 7:
					//client.CodecUpdateSPS(nal)
				case naluType == 8:
					//client.CodecUpdatePPS(nal)
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
							retmap = append(retmap, &av.Packet{
								Data:            append(int32ToBytes(len(packet[2:size+2])), packet[2:size+2]...),
								CompositionTime: time.Duration(1) * time.Millisecond,
								Idx:             rtspClient.VideoIDX,
								IsKeyFrame:      naluType == 5,
								Duration:        time.Duration(float32(timestamp-rtspClient.PreVideoTimestamp)/90) * time.Millisecond,
								Time:            time.Duration(timestamp/90) * time.Millisecond,
							})
						case naluTypefs == 7:
							//client.CodecUpdateSPS(packet[2 : size+2])
						case naluTypefs == 8:
							//client.CodecUpdatePPS(packet[2 : size+2])
						}
						packet = packet[size+2:]
					}
				case naluType == 28:
					fuIndicator := content[offset]
					fuHeader := content[offset+1]
					isStart := fuHeader&0x80 != 0
					isEnd := fuHeader&0x40 != 0
					if isStart {
						rtspClient.RTPPacketFragmentationStarted = true
						rtspClient.RTPBuffer.Truncate(0)
						rtspClient.RTPBuffer.Reset()
						rtspClient.RTPBuffer.Write([]byte{fuIndicator&0xe0 | fuHeader&0x1f})
					}
					if rtspClient.RTPPacketFragmentationStarted {
						rtspClient.RTPBuffer.Write(content[offset+2 : end])
						if isEnd {
							rtspClient.RTPPacketFragmentationStarted = false
							naluTypef := rtspClient.RTPBuffer.Bytes()[0] & 0x1f
							if naluTypef == 7 || naluTypef == 9 {
								bufered, _ := h264parser.SplitNALUs(append([]byte{0, 0, 0, 1}, rtspClient.RTPBuffer.Bytes()...))
								for _, v := range bufered {
									naluTypefs := v[0] & 0x1f
									switch {
									case naluTypefs == 5:
										rtspClient.RTPBuffer.Reset()
										rtspClient.RTPBuffer.Write(v)
										naluTypef = 5
									case naluTypefs == 7:
										//client.CodecUpdateSPS(v)
									case naluTypefs == 8:
										//client.CodecUpdatePPS(v)
									}
								}
							}
							retmap = append(retmap, &av.Packet{
								Data:            append(int32ToBytes(rtspClient.RTPBuffer.Len()), rtspClient.RTPBuffer.Bytes()...),
								CompositionTime: time.Duration(1) * time.Millisecond,
								Duration:        time.Duration(float32(timestamp-rtspClient.PreVideoTimestamp)/90) * time.Millisecond,
								Idx:             rtspClient.VideoIDX,
								IsKeyFrame:      naluTypef == 5,
								Time:            time.Duration(timestamp/90) * time.Millisecond,
							})
						}
					}
				default:
					logger.Error(fmt.Sprintf("Unsupported NAL Type %d", naluType))
				}
			}
		}

		if len(retmap) > 0 {
			rtspClient.PreVideoTimestamp = timestamp
			return retmap, true
		}
	default:
		//client.Println("Unsuported Intervaled data packet", int(content[1]), content[offset:end])
	}
	return nil, false
}

func int16ToBytes(val int) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, uint16(val))
	return buf
}

func int32ToBytes(val int) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(val))
	return buf
}
