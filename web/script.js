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
        flushingTime: 10000,
        fps: 30,
        debug: true
    });

    let port = 6060
    let ip = "0.0.0.0"
    let wsUrl = `ws://${ip}:${port}`

    let ws = new WebSocket(wsUrl)
    //ws.binaryType = 'arraybuffer';
    ws.binaryType = 'blob'

    ws.onopen = () => {
        log("Connected")
    }

    ws.onmessage = (event) => {
        var wsMsg = event.data;
        var arrayBuffer;
        var fileReader = new FileReader();
        fileReader.onload = function(event) {
            arrayBuffer = event.target.result;
            var data = new Uint8Array(arrayBuffer);
            if (!streamingStarted) {
                appendToBuffer(arrayBuffer);
                streamingStarted=true;
                return;
            }
            queue.push(arrayBuffer); // add to the end
        };
        fileReader.readAsArrayBuffer(wsMsg);
    }

    ws.onclose = () => {
        log("Closed")
    }

    ws.onerror = (error) => {
        log("Error")
    }

    var queue = [];
    var video = null;
    var sourceBuffer = null;
    var streamingStarted = false;

// Init the Media Source and add event listener
    function initMediaSource() {
        video = document.getElementById('sample');
        video.onerror = elementError;
        video.loop = false;
        video.addEventListener('canplay', (event) => {
            console.log('Video can start, but not sure it will play through.');
            video.play();
        });
        video.addEventListener('paused', (event) => {
            console.log('Video paused for buffering...');
            setTimeout(function () {
                video.play();
            }, 2000);
        });

        /* NOTE: Chrome will not play the video if we define audio here
        * and the stream does not include audio */
        var mimeCodec = 'video/mp4; codecs="avc1.4D0033, mp4a.40.2"';
        //var mimeCodec = 'video/mp4; codecs=avc1.42E01E,mp4a.40.2'; baseline
        //var mimeCodec = 'video/mp4; codecs=avc1.4d002a,mp4a.40.2'; main
        //var mimeCodec = 'video/mp4; codecs="avc1.64001E, mp4a.40.2"'; high

        if (!window.MediaSource) {
            console.error("No Media Source API available");
            return;
        }

        if (!MediaSource.isTypeSupported(mimeCodec)) {
            console.error("Unsupported MIME type or codec: " + mimeCodec);
            return;
        }

        var ms = new MediaSource();
        video.src = window.URL.createObjectURL(ms);
        ms.addEventListener('sourceopen', onMediaSourceOpen);

        function onMediaSourceOpen() {
            sourceBuffer = ms.addSourceBuffer(mimeCodec);
            sourceBuffer.addEventListener("updateend", loadPacket);
            sourceBuffer.addEventListener("onerror", sourceError);
        }

        function loadPacket() { // called when sourceBuffer is ready for more
            if (!sourceBuffer.updating) {
                if (queue.length > 0) {
                    data = queue.shift(); // pop from the beginning
                    appendToBuffer(data);
                } else { // the queue runs empty, so we must force-feed the next packet
                    streamingStarted = false;
                }
            } else {
            }
        }

        function sourceError(event) {
            console.log("Media source error");
        }

        function elementError(event) {
            console.log("Media element error");
            console.log(event.target.error.message)
        }
    }

// Append AV data to source buffer
    function appendToBuffer(videoChunk) {
        if (videoChunk) {
            sourceBuffer.appendBuffer(videoChunk);
        }
    }

    initMediaSource();
})

