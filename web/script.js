window.addEventListener('load',()=>{
    var video = document.getElementById('sample')

    var mediaSource = new MediaSource()
    var mimeType = 'video/mp4; codecs="mp4a.40.2,avc1.64001f'
    var sourceBuffer = null

    var mp4 = new muxjs.mp4.Transmuxer()

    video.src = URL.createObjectURL(mediaSource)

    let appendNextSegment = () => {
        mp4.off('data')
        mp4.on('data', segment => {
            let data = new Uint8Array(segment.initSegment.byteLength + segment.data.byteLength);

            // Add the segment.initSegment (ftyp/moov) starting at position 0
            data.set(segment.initSegment, 0);

            // Add the segment.data (moof/mdat) starting after the initSegment
            data.set(segment.data, segment.initSegment.byteLength);

            console.log(muxjs.mp4.tools.inspect(data));

            // Add your brand new fMP4 segment to your MSE Source Buffer
            sourceBuffer.appendBuffer(data);
        })
    }

    let appendFirstFrame = () => {
        URL.revokeObjectURL(video.src)
        sourceBuffer = mediaSource.addSourceBuffer(mimeType)
        sourceBuffer.addEventListener('updateend',appendNextSegment)

        mp4.on('data', segment => {
            let data = new Uint8Array(segment.initSegment.byteLength + segment.data.byteLength);

            // Add the segment.initSegment (ftyp/moov) starting at position 0
            data.set(segment.initSegment, 0);

            // Add the segment.data (moof/mdat) starting after the initSegment
            data.set(segment.data, segment.initSegment.byteLength);

            console.log(muxjs.mp4.tools.inspect(data));

            // Add your brand new fMP4 segment to your MSE Source Buffer
            sourceBuffer.appendBuffer(data);
            ws.send(segment.data)
        })
    }

    mediaSource.addEventListener('sourceopen',appendFirstFrame)

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
    let ip = "0.0.0.0"
    let wsUrl = `ws://${ip}:${port}`

    let ws = new WebSocket(wsUrl)
    ws.binaryType = 'arraybuffer';
    //ws.binaryType = 'blob'

    ws.onopen = () => {
        log("Connected")
    }

    let packagesCollected = 0

    ws.onmessage = (event) => {
        let data = new Uint8Array(event.data)
        mp4.push(data)

        if (packagesCollected++ > 15){
            mp4.flush()
            packagesCollected=0
        }

    }

    ws.onclose = () => {
        log("Closed")
    }

    ws.onerror = (error) => {
        log("Error")
    }

})

