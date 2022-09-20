window.addEventListener('load', () => {
    var video = document.getElementById('video_ws')

    var mediaSource = new MediaSource()
    var sourceBuffer = null
    var streamingStarted = false;
    var queue = [];

    video.src = URL.createObjectURL(mediaSource)

    let log = (message) => {
        let now = new Date()
        let nowFormatted = now.toLocaleString()
        let log = document.getElementById('log')
        let template = `[${nowFormatted}]: ${message}`
        let logLine = document.createElement("div")
        logLine.innerHTML = template
        logLine.className = 'log-line'

        log.appendChild(logLine)
    }

    let port = 6060
    //let ip = "0.0.0.0"
    let ip = "10.0.43.19"
    let wsUrl = `ws://${ip}:${port}`

    let ws = new WebSocket(wsUrl)
    ws.binaryType = 'arraybuffer';
    ws.onopen = () => {
        log("Connected")
    }
    ws.onclose = () => {
        log("Closed")
    }
    ws.onerror = (error) => {
        log("Error")
    }

    ws.onmessage = function (event) {
        var data = new Uint8Array(event.data);
        if (data[0] === 9) {
            decoded_arr = data.slice(1);
            if (window.TextDecoder) {
                mimeCodec = new TextDecoder("utf-8").decode(decoded_arr);
            }

            log('Initialized codec: ' + mimeCodec);

            sourceBuffer = mediaSource.addSourceBuffer('video/mp4; codecs="' + mimeCodec + '"');
            sourceBuffer.mode = "segments"
            sourceBuffer.addEventListener("updateend", loadPacket);
        } else {
            pushPacket(event.data);
        }
    };

    function pushPacket(packet) {
        if (!streamingStarted) {
            sourceBuffer.appendBuffer(packet);
            streamingStarted = true;
            return;
        }
        queue.push(packet);
        if (!sourceBuffer.updating) {
            loadPacket();
        }
    }

    function loadPacket() {
        if (!sourceBuffer.updating) {
            if (queue.length > 0) {
                let latestPacket = queue.shift();
                sourceBuffer.appendBuffer(latestPacket);
            } else {
                streamingStarted = false;
            }
        }
    }
})

