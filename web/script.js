class websocketStream {
    constructor(address,playerId) {
        this.connect(address)
        this.init(playerId)

        this.address = address
        this.playerId = playerId
    }

    connect = (address) => {
        let log = this.log

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
                this.sourceBuffer.addEventListener('error',()=>{
                    this.disconnect()
                    this.destroy()
                    this.connect(address)
                    this.init(this.playerId)
                })
            } else {
                this.pushPacket(event.data);
            }
        };
    }

    disconnect = () => {
        this.ws.close(1000,"Disconnected manually")
    }

    destroy = () => {
        delete this.mediaSource
        delete this.sourceBuffer
        delete this.streamingStarted
        delete this.queue
        delete this.video
    }

    init = (playerId) => {
        let video = document.getElementById(playerId)

        if (video === undefined || video === null) {
            this.log(`DOM element with id ${playerId} not found`)
            return
        }

        this.mediaSource = new MediaSource()
        this.sourceBuffer = null
        this.streamingStarted = false;
        this.queue = [];

        video.src = URL.createObjectURL(this.mediaSource)
        this.video = video
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

    log = (message) => {
        let now = new Date()
        let nowFormatted = now.toLocaleString()
        let template = `[${nowFormatted}]: ${message}`

        let log = document.getElementById('log')

        if(log === null) {
            console.log(template)
            return
        }

        let logLine = document.createElement("div")
        logLine.innerHTML = template
        logLine.className = 'log-line'

        log.appendChild(logLine)
    }
}