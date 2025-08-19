package main

import (
	"TcpConnection/utils"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
)

type Config struct {
	TCPAddr     string
	HTTPAddr    string
	MaxClients  int
	OutboxSize  int
	ReadTimeout time.Duration
}

func LoadConfig() Config {
	return Config{
		TCPAddr:     getEnv("TCP_ADDR", ":5000"),
		HTTPAddr:    getEnv("HTTP_ADDR", ":9000"),
		MaxClients:  getEnvInt("MAX_CLIENTS", 200),
		OutboxSize:  getEnvInt("OUTBOX_SIZE", 64),
		ReadTimeout: getEnvDuration("READ_TIMEOUT", 60*time.Second),
	}
}

type Client struct {
	id       int64
	conn     net.Conn
	outbox   chan string
	addr     string
	since    time.Time
	bytesIn  int64
	bytesOut int64
}

type Hub struct {
	mu       sync.Mutex
	clients  map[*Client]bool
	nextID   int64
	started  time.Time
	bytesIn  int64
	bytesOut int64
	msgsIn   int64
	msgsOut  int64
	drops    int64
	config   Config
}

func NewHub(cfg Config) *Hub {
	return &Hub{
		clients: make(map[*Client]bool),
		started: time.Now(),
		config:  cfg,
	}
}

func (h *Hub) Add(pClientDetail *Client, logger *utils.LoggerId) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.clients) >= h.config.MaxClients {
		return false
	}
	h.clients[pClientDetail] = true
	logger.Log("Client connected:", pClientDetail.addr, "ID:", pClientDetail.id)
	return true
}

func (h *Hub) Remove(pClientDetail *Client, logger *utils.LoggerId) {
	h.mu.Lock()
	delete(h.clients, pClientDetail)
	h.mu.Unlock()
	pClientDetail.conn.Close()
	logger.Log("Client disconnected:", pClientDetail.addr, "ID:", pClientDetail.id)
}

func (h *Hub) Broadcast(pMsg string, logger *utils.LoggerId) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c.outbox <- pMsg:
			h.msgsOut++
			h.bytesOut += int64(len(pMsg) + 1)
		default:
			h.drops++
			logger.Log("Drop (backpressure) for client:", c.id)
		}
	}
	logger.Log("Broadcast:", pMsg)
}

func handleClient(pCtx context.Context, pHubData *Hub, pConn net.Conn) {
	log := &utils.LoggerId{}
	log.SetSid()

	pHubData.mu.Lock()
	pHubData.nextID++
	id := pHubData.nextID
	pHubData.mu.Unlock()

	lClient := &Client{
		id:     id,
		conn:   pConn,
		outbox: make(chan string, pHubData.config.OutboxSize),
		addr:   pConn.RemoteAddr().String(),
		since:  time.Now(),
	}

	if !pHubData.Add(lClient, log) {
		fmt.Fprintln(pConn, "ERR server full")
		pConn.Close()
		return
	}

	// Writer
	go func() {
		// for lMsg := range lClient.outbox {
		// 	lBytesCount, lErr := fmt.Fprintln(lClient.conn, lMsg)
		// 	if lErr != nil {
		// 		break
		// 	}
		// 	lClient.bytesOut += int64(lBytesCount)
		// }

		defer pConn.Close()
		for {
			select {
			case <-pCtx.Done(): // shutdown signal
				log.Log("Client Connection Closed:", lClient.addr, "ID:", lClient.id)
				return
			case lMsg, ok := <-lClient.outbox:
				if !ok {
					return
				}
				lBytesCount, lErr := fmt.Fprintln(lClient.conn, lMsg)
				if lErr != nil {
					break
				}
				lClient.bytesOut += int64(lBytesCount)
			}
		}
	}()

	// Reader
	reader := bufio.NewScanner(pConn)
	for {
		pConn.SetReadDeadline(time.Now().Add(pHubData.config.ReadTimeout))
		select {
		case <-pCtx.Done(): // shutdown signal
			return
		default:
		}

		if !reader.Scan() {
			break
		}

		line := strings.TrimSpace(reader.Text())

		pHubData.mu.Lock()
		pHubData.msgsIn++
		pHubData.bytesIn += int64(len(line) + 1)
		pHubData.mu.Unlock()

		log.NewRequestSid()
		switch {
		case line == "PING":
			log.Log("PING received")
			lClient.outbox <- "PONG"
		case strings.HasPrefix(line, "ECHO "):
			msg := strings.TrimPrefix(line, "ECHO ")
			log.Log("ECHO:", msg)
			lClient.outbox <- msg
		case line == "TIME":
			log.Log("TIME request")
			lClient.outbox <- time.Now().Format(time.RFC3339)
		case strings.HasPrefix(line, "BCAST "):
			msg := strings.TrimPrefix(line, "BCAST ")
			log.Log("BCAST:", msg)
			pHubData.Broadcast(msg, log)
		default:
			log.Log("Unknown command:", line)
			lClient.outbox <- "ERR unknown"
		}
	}

	pHubData.Remove(lClient, log)
	close(lClient.outbox)
}

func main() {
	lErr := os.MkdirAll("log", os.ModePerm)
	if lErr != nil {
		log.Fatalf("Failed to create log directory: %v", lErr)
	}
	logFolderName := "./log/log" + time.Now().Format("02012006.15.04.05") + ".log"
	file, lErr := os.OpenFile(logFolderName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if lErr != nil {
		fmt.Println("Error opening log file:", lErr)
		return
	}
	log.SetOutput(file)
	lConfigData := LoadConfig()
	lHubDetails := NewHub(lConfigData)

	logger := &utils.LoggerId{}
	logger.SetSid()

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// TCP
	lListener, lErr := net.Listen("tcp", lConfigData.TCPAddr)
	if lErr != nil {
		logger.Log("TCP listen error:", lErr)
		os.Exit(1)
	}
	defer lListener.Close()
	logger.Log("TCP server listening on", lConfigData.TCPAddr)

	// Capture Ctrl+C
	go func() {
		lSignal := make(chan os.Signal, 1)
		signal.Notify(lSignal, os.Interrupt)
		<-lSignal
		log.Println("Shutting down server...")
		cancel()
		lListener.Close()
		os.Exit(1)
	}()

	go func() {
		for {
			select {
			case <-ctx.Done(): // shutdown
				logger.Log("TCP listener stopping")
				return
			default:
			}
			lClientConn, lErr := lListener.Accept()
			if lErr != nil {
				logger.Log("Accept error:", lErr)
				continue
			}
			go handleClient(ctx, lHubDetails, lClientConn)
		}
	}()

	// HTTP API
	http.HandleFunc("/broadcast", func(w http.ResponseWriter, r *http.Request) {
		log := &utils.LoggerId{}
		log.SetSid()
		log.NewRequestSid()

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		lBody, lErr := io.ReadAll(r.Body)
		if lErr != nil || len(lBody) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		lMsg := strings.TrimSpace(string(lBody))
		lHubDetails.Broadcast(lMsg, log)
		json.NewEncoder(w).Encode(map[string]string{"broadcasted": lMsg})
	})

	http.HandleFunc("/clients", func(w http.ResponseWriter, r *http.Request) {
		lHubDetails.mu.Lock()
		lClientDetails := make([]map[string]interface{}, 0, len(lHubDetails.clients))
		for lClientData := range lHubDetails.clients {
			lClientDetails = append(lClientDetails, map[string]interface{}{
				"id":              lClientData.id,
				"remote_addr":     lClientData.addr,
				"connected_since": lClientData.since.Format(time.RFC3339),
			})
		}
		lHubDetails.mu.Unlock()
		json.NewEncoder(w).Encode(lClientDetails)
	})

	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		lHubDetails.mu.Lock()
		metrics := map[string]interface{}{
			"uptime_sec":  int64(time.Since(lHubDetails.started).Seconds()),
			"clients":     len(lHubDetails.clients),
			"bytes_in":    lHubDetails.bytesIn,
			"bytes_out":   lHubDetails.bytesOut,
			"msgs_in":     lHubDetails.msgsIn,
			"msgs_out":    lHubDetails.msgsOut,
			"drops":       lHubDetails.drops,
			"max_clients": lHubDetails.config.MaxClients,
		}
		lHubDetails.mu.Unlock()
		json.NewEncoder(w).Encode(metrics)
	})

	logger.Log("HTTP server listening on", lConfigData.HTTPAddr)
	if err := http.ListenAndServe(lConfigData.HTTPAddr, nil); err != nil {
		logger.Log("HTTP listen error:", err)
	}
}

// --- helpers ---
func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var i int
		fmt.Sscanf(v, "%d", &i)
		return i
	}
	return def
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return def
}
