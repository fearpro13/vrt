window.addEventListener('load',()=>{
    log = (message) => {
        let now = new Date()
        let nowFormatted = now.toLocaleString()
        let log = document.getElementById('log')
        let template = `[${nowFormatted}]: ${message}`
        let logLine = document.createElement("div")
        logLine.innerHTML = template
        logLine.className = 'log-line'

        log.appendChild(logLine)
    }

    var jmuxer = new JMuxer({
        node: 'sample',
        mode: 'video',
        flushingTime: 1000,
        fps: 30,
        debug: true
    });

    let port = 6060
    let ip = "0.0.0.0"
    let wsUrl = `ws://${ip}:${port}`

    let ws = new WebSocket(wsUrl)
    ws.binaryType = 'arraybuffer';

    ws.onopen = () => {
        log("Connected")
    }

    ws.onmessage = (event) => {
        let data = new Uint8Array(event.data)
        //log(`Received: ${data.length} bytes`)
        jmuxer.feed({
            video: data
        });
    }

    ws.onclose = () => {
        log("Closed")
    }

    ws.onerror = (error) => {
        log("Error")
    }
})

