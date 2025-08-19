# TCP_Line_Server


### clone URL
```sh
https://github.com/SagarVijaya/Video_Analytics.git
```
### **1 Install Dependencies**

```sh
go mod tidy
```

### **2 Run the app**
```sh
go run main.go

```
### **3 Env File Data**
```
TCP_ADDR=:5000       # TCP server address
HTTP_ADDR=:9000      # HTTP server address
MAX_CLIENTS=200      # Maximum concurrent TCP clients
OUTBOX_SIZE=64       # Per-client outbound queue size
READ_TIMEOUT=30s     # Per-client read timeout


```

### **4 Open a TCP connection using telnet:

```sh
telnet localhost 5000

Example 1 → PING - PONG

Example 2 → ECHO Hello World - Hello World

Example 3 → TIME - 2025-08-19T03:10:00+05:30

Example 4 → BCAST Server is live! - Server is live!(ALL CLIENTS) 

```

## sample Request

```sh
curl --location --request GET 'http://localhost:9000/metrics' \
--header 'Content-Type: text/plain' \
--data 'BCAST Hello from HTTP
'

curl --location --request GET 'http://localhost:9000/clients' \
--header 'Content-Type: text/plain' \
--data 'BCAST'

curl --location 'http://localhost:9000/broadcast' \
--header 'Content-Type: text/plain' \
--data 'SERVER IS LIVE'


```

```sh

http://localhost:9000

```


### sample Request 1

- Endpoint :  /broadcast
- Method :  POST

```sh

SERVER IS LIVE

```Text

## sample Response 1

```json

{"broadcasted":"SERVER IS LIVE"}

```

### sample Request 2
- Endpoint :  /clients
- Method :  GET

### sample Response 2
```json
[
    {
        "connected_since": "2025-08-19T02:46:51+05:30",
        "id": 1,
        "remote_addr": "[::1]:55412"
    }
]
```

### sample Response 3
- Endpoint :  /metrics
- Method :  GET

```json
{
    "bytes_in": 19,
    "bytes_out": 0,
    "clients": 0,
    "drops": 0,
    "max_clients": 200,
    "msgs_in": 3,
    "msgs_out": 0,
    "uptime_sec": 678
}


```




