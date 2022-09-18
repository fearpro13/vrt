# vrt

Video re-transmitter

RTSP proxy + Rtsp to WebSocket

# Cборка
#### go build < buildDir >

# Запуск
#### Пример запуска с уровнем логирования INFO
#### vrt rtsp://rtsp://username:password@127.0.0.1:554/?trackID=1 2
## Программа запускается с 2-мя аргументами

## 1 - адрес удалённого rtsp сервера
    например rtsp://username:password@127.0.0.1:554/?trackID=1
## 2 - уровень логирования
    JUNK 0
    DEBUG 1
    INFO 2
    NOTICE 3
    WARN 4
    ERR 5
    CRITICAL 6
    EMERGENCY 7
