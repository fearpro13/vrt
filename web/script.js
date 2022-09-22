class websocketStream {
    constructor(address,playerId,log) {
        let video = document.getElementById(playerId)

        if (video === undefined || video === null) {
            log(`DOM element with id ${playerId} not found`)
            return
        }

        this.mediaSource = new MediaSource()
        this.sourceBuffer = null
        this.streamingStarted = false;
        this.queue = [];

        video.src = URL.createObjectURL(this.mediaSource)

        this.ws = new WebSocket(address)
        this.ws.binaryType = 'arraybuffer';
        this.ws.onopen = () => {
            log("Connected")
        }
        this.ws.onclose = () => {
            log("Closed")
        }
        this.ws.onerror = (error) => {
            log("Error")
        }

        this.ws.onmessage = event => {
            let data = new Uint8Array(event.data);
            let decoded_arr;
            let mimeCodec;
            if (data[0] === 9) {
                decoded_arr = data.slice(1);
                if (window.TextDecoder) {
                    mimeCodec = new TextDecoder("utf-8").decode(decoded_arr);
                }

                log('Initialized codec: ' + mimeCodec);

                this.sourceBuffer = this.mediaSource.addSourceBuffer('video/mp4; codecs="' + mimeCodec + '"');
                this.sourceBuffer.mode = "segments"
                this.sourceBuffer.addEventListener("updateend", this.loadPacket);
            } else {
                this.pushPacket(event.data);
            }
        };
    }

    pushPacket = (packet) => {
        if (!this.streamingStarted) {
            this.sourceBuffer.appendBuffer(packet);
            this.streamingStarted = true;
            return;
        }
        this.queue.push(packet);
        if (!this.sourceBuffer.updating) {
            this.loadPacket();
        }
    }

    loadPacket = () => {
        if (!this.sourceBuffer.updating) {
            if (this.queue.length > 0) {
                let latestPacket = this.queue.shift();
                this.sourceBuffer.appendBuffer(latestPacket);
            } else {
                this.streamingStarted = false;
            }
        }
    }
}

window.addEventListener('load', () => {
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

    new websocketStream("ws://0.0.0.0:6061",'video_tcp',log)
    //new websocketStream("ws://0.0.0.0:6062",'video_udp',log)
    // websocketStream("ws://0.0.0.0:6063",'video_proxy_tcp',log)
    // websocketStream("ws://0.0.0.0:6064",'video_proxy_udp',log)
})

