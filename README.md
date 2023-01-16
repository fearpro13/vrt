# vrt

Video re-transmitter

RTSP proxy + Rtsp to WebSocket + Http control mode

# Build App
#### go build < buildOutputDir >

# Launch
## Program requires 1 argument to start(in combined mode this argument is ignored, so if you are going to use combined mode, you can just type any single char to bypass args check)

## 1 - remote rtsp address
    example: rtsp://username:password@127.0.0.1:554/?trackID=1

# Possible options for launch:

## Program could be started in one of three available modes:
-p - RTSP proxy

-w - video translation through websocket(fMP4)

-h - combined mode with control by HTTP API

In case of combined mode, some of program options might be ignored as further adding of rtsp remote sources is done through HTTP API.
Each newly added rtsp remote source in combined mode is connected via TCP(known bug, UDP is implemented, but not wired with HTTP API)

## Each mode has additional option - local port. If you are going to use port choosen by system - set option value to 0.
 -p:p - local RTSP proxy port

 -w:p - local websocket translation port

 -h:p - local HTTP port for combined mode

## -v - Verbosity level (number)
    JUNK 0

    DEBUG 1

    INFO 2

    NOTICE 3

    WARN 4

    ERR 5

    CRITICAL 6

    EMERGENCY 7

## -t - underlying protocol, used for remote RTSP connections(ignored in combined mode)
    tcp

    udp

## Available HTTP API for combined mode:

### Add rtsp proxy
POST, JSON /add_proxy

{

	'remote_address':<remote_rtsp_address>,

	'local_port':<local_port>

}

local_port - local Rtsp proxy port, for system auto-selected port put 0

### Stop Rtsp proxy
POST,JSON /stop_proxy

{

'id':<proxy_id>

}


### Get list of available Rtsp proxies
GET /proxy_list


### Add websocket video translation
POST, JSON /add_broadcast`

{

	'remote_address':<remote_rtsp_address>,

	'lifetime':<websocket_stream_lifetime>,

	'active_time':<websocket_stream_active_time>

}

websocket_stream_lifetime - total duration of websocket translation(seconds). Once this parameter becomes greater than application running time - translation stops. Put 0 to avoid this limitation.

websocket_stream_active_time - If translation does not have any clients connected for this period of time - translation will be stopped. Put 0 to avoid this limitation.

### Stop websocket translation
POST, JSON /stop_broadcast

{

'id':<websocket_broadcast_id>

}

### Get list of available websocket translations
GET /broadcast_list



#### Example - starting in rtsp proxy mode with verbosity level INFO on local port 6060, remote rtsp connections use TCP protocol
#### vrt rtsp://username:password@127.0.0.1:554/?trackID=1 -p:p 6060 -v 2 -t tcp

#### Example - starting in websocket translation mode with verbosity level INFO on local port 6060, remote rtsp connections use UDP protocol
#### vrt rtsp://username:password@127.0.0.1:554/?trackID=1 -w:p 6060 -v 2 -t udp

#### Example - starting in combined mode with verbosity level INFO on local port 6060
#### vrt rtsp://username:password@127.0.0.1:554/?trackID=1 -h:p 6060 -v 2


#Examples of using program in combined mode could be found in /web folder.





