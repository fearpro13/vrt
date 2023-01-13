# vrt

Video re-transmitter

RTSP proxy + Rtsp to WebSocket + Http control mode

# Cборка
#### go build < buildOutputDir >

# Запуск
## Программа запускается с 1-м аргументом

## 1 - адрес удалённого rtsp сервера
    например rtsp://username:password@127.0.0.1:554/?trackID=1

# Допустимые параметры для запуска:

## Программу возможно запустить в одном из 3-х режимов:
-p - RTSP прокси

-w - трансляция RTSP через websocket(fMP4)

-h - комбинированный режим работы, с управлением через HTTP API

В случае запуска в комбинированном режиме, используемые параметры для запуска программы могут игнорироваться, т.к
добавление новых RTSP ресурсов осуществляется уже через HTTP API

## У каждого из режимов доступен дополнительный параметр - порт. Для выбора порта системой(автоматически) необходимо указать 0
 -p:p

 -w:p

 -h:p

## -v - уровень логирования
    JUNK 0

    DEBUG 1

    INFO 2

    NOTICE 3

    WARN 4

    ERR 5

    CRITICAL 6

    EMERGENCY 7

## -t - протокол для подключения к удалённому rtsp ресурсу
    tcp

    udp

## Для комбинированного режима доступно следующее HTTP API

### Добавление RTSP прокси
POST, JSON /add_proxy

{

	'remote_address':<remote_rtsp_address>,

	'local_port':<local_port>

}

local_port - локальный порт, для RTSP прокси. 0 - порт выбирается системой

### Остановка RTSP прокси
POST,JSON /stop_proxy

{

'id':<proxy_id>

}


### Получение списка активных RTSP прокси
GET /proxy_list


### Добавление websocket трансляции
POST, JSON /add_broadcast`

{

	'remote_address':<remote_rtsp_address>,

	'lifetime':<websocket_stream_lifetime>,

	'active_time':<websocket_stream_active_time>

}

websocket_stream_lifetime - общее время длительности трансляции в секундах, после окончания этого времени - websocket трансляция завершается. 0 - без ограничений

websocket_stream_active_time - время длительности трансляции в секундах, по истечению которого трансляция завершается, если она совсем не используется(нет ни одного подключенного пользователя). 0 - без ограничений

### Остановка websocket трансляции
POST, JSON /stop_broadcast

{

'id':<websocket_broadcast_id>

}

### Получение списка websocket трансляций
GET /broadcast_list



#### Пример запуска RTSP прокси с уровнем логирования INFO на порту 6060, подключение к удалённому ресурсу TCP
#### vrt rtsp://username:password@127.0.0.1:554/?trackID=1 -p:p 6060 -v 2 -t tcp

#### Пример запуска трансляции через websocket с уровнем логирования INFO на порту 6060, подключение к удалённому ресурсу UDP
#### vrt rtsp://username:password@127.0.0.1:554/?trackID=1 -w:p 6060 -v 2 -t udp

#### Пример запуска комбинированного режима с уровнем логирования INFO на порту 6060
#### vrt rtsp://username:password@127.0.0.1:554/?trackID=1 -h:p 6060 -v 2





