
    let addProxy = (address,port) => {
        let uri = `http://localhost:6060/add_proxy`
        let config = {
            method:'POST',
            body:JSON.stringify({
                'remote_address':address,
                'local_port':port
            })
        }
        fetch(uri,config)
    }

    let stopProxy = (id) => {
        let uri = `http://localhost:6060/stop_proxy`
        let config = {
            method:'POST',
            body:JSON.stringify({id})
        }
        fetch(uri,config)
    }

    let proxyList = async () => {
        let uri = `http://localhost:6060/proxy_list`
        let response = await fetch(uri)
        return await response.json()
    }

    let addBroadcast = address => {
        let uri = `http://localhost:6060/add_broadcast`
        let config = {
            method:'POST',
            body:JSON.stringify({
                'remote_address':address
            })
        }
        fetch(uri,config)
    }

    let stopBroadcast = (id) => {
        let uri = `http://localhost:6060/stop_broadcast`
        let config = {
            method:'POST',
            body:JSON.stringify({id})
        }
        fetch(uri,config)
    }

    let broadcastList = async () => {
        let uri = `http://localhost:6060/broadcast_list`
        let response = await fetch(uri)
        return await response.json()
    }

function addNewProxy(){
    let address = document.getElementById('rtsp_address').value
    addProxy(address,0)
}

function addNewBroadcast(){
    let address = document.getElementById('broadcast_address').value
    addBroadcast(address)
}

    function refreshBroadcastList(){
        let list = document.getElementById('broadcast_list')
        list.innerHTML = ""
        broadcastList()
            .then(broadcasts => {
                broadcasts.forEach(broadcast => {
                    let row = document.createElement('tr')
                    list.append(row)

                    let col1 = document.createElement('td')
                    col1.innerHTML = broadcast.id
                    row.append(col1)

                    let col2 = document.createElement('td')
                    col2.innerHTML = broadcast.remote_address
                    row.append(col2)

                    let col3 = document.createElement('td')
                    col3.innerHTML = broadcast.local_address
                    col3.style.cursor='pointer'
                    col3.onclick=()=>{
                        let address = `ws://localhost:6060${broadcast.local_address}`
                        broadcastPreview(address)
                    }
                    row.append(col3)

                    let col4 = document.createElement('td')
                    col4.innerHTML = broadcast.clients
                    row.append(col4)

                    let col5 = document.createElement('td')
                    row.append(col5)

                    let closeButton = document.createElement('input')
                    closeButton.type="button"
                    closeButton.value = "STOP"
                    closeButton.onclick = () => {stopBroadcast(broadcast.id)}
                    col5.append(closeButton)
                })
            })
    }


function refreshProxyList(){
    let list = document.getElementById('proxy_list')
    list.innerHTML = ""
    proxyList()
        .then(proxies => {
            proxies.forEach(proxy => {
                let row = document.createElement('tr')
                list.append(row)

                let col1 = document.createElement('td')
                col1.innerHTML = proxy.id
                row.append(col1)

                let col2 = document.createElement('td')
                col2.innerHTML = proxy.remote_address
                row.append(col2)

                let col3 = document.createElement('td')
                col3.innerHTML = proxy.local_address
                row.append(col3)

                let col4 = document.createElement('td')
                col4.innerHTML = proxy.clients
                row.append(col4)

                let col5 = document.createElement('td')
                row.append(col5)

                let closeButton = document.createElement('input')
                closeButton.type="button"
                closeButton.value = "STOP"
                closeButton.onclick = () => {stopProxy(proxy.id)}
                col5.append(closeButton)
            })
        })
}

let broadcastPreview = (address) => {
    new websocketStream(address,'preview')
}



