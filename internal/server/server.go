package server

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/henankongming/ichuansu/internal/transfer"
)

const (
	HealthPath   = "/v1/health"
	DownloadPath = "/v1/download"
	UploadPath   = "/v1/upload"
)

type Handler struct {
	programDir string
	token      []byte
}

func NewHandler(programDir string, token []byte) http.Handler {
	return &Handler{programDir: programDir, token: append([]byte(nil), token...)}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.URL.Path {
	case HealthPath:
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "ichuansu"})
	case DownloadPath:
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.download(w, r)
	case UploadPath:
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.upload(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) authorized(r *http.Request) bool {
	authorization := r.Header.Get("Authorization")
	if len(h.token) != 32 || !strings.HasPrefix(authorization, "Bearer ") {
		return false
	}
	provided := strings.TrimPrefix(authorization, "Bearer ")
	if len(provided) != hex.EncodedLen(len(h.token)) {
		return false
	}
	decoded := make([]byte, hex.DecodedLen(len(provided)))
	if _, err := hex.Decode(decoded, []byte(provided)); err != nil {
		return false
	}
	return len(decoded) == len(h.token) && subtle.ConstantTimeCompare(decoded, h.token) == 1
}

func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	sourcePath := r.URL.Query().Get("path")
	if strings.TrimSpace(sourcePath) == "" {
		http.Error(w, "source path is required", http.StatusBadRequest)
		return
	}
	prepared, err := transfer.PrepareSource(sourcePath)
	if err != nil {
		if errors.Is(err, transfer.ErrSourceMissing) {
			http.Error(w, "source not found", http.StatusNotFound)
		} else if errors.Is(err, transfer.ErrSourceUnavailable) {
			http.Error(w, "source unavailable", http.StatusForbidden)
		} else {
			http.Error(w, "source unavailable or unsupported", http.StatusBadRequest)
		}
		return
	}
	defer prepared.Cleanup()
	file, err := prepared.Open()
	if err != nil {
		http.Error(w, "source unavailable", http.StatusNotFound)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(prepared.Size, 10))
	w.Header().Set("X-Ichuansu-Name", prepared.Name)
	w.Header().Set("X-Ichuansu-SHA256", prepared.SHA256)
	w.Header().Set("X-Ichuansu-Size", strconv.FormatInt(prepared.Size, 10))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, file)
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	name, err := transfer.SafeFilename(r.URL.Query().Get("name"))
	if err != nil {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}
	if r.ContentLength < 0 {
		http.Error(w, "content length is required", http.StatusLengthRequired)
		return
	}
	expectedHash := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Ichuansu-SHA256")))
	if len(expectedHash) != 64 {
		http.Error(w, "invalid SHA-256 header", http.StatusBadRequest)
		return
	}
	for _, char := range expectedHash {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			http.Error(w, "invalid SHA-256 header", http.StatusBadRequest)
			return
		}
	}
	path, err := transfer.SaveAtomic(h.programDir, name, r.Body, r.ContentLength, expectedHash, nil)
	if err != nil {
		if err == transfer.ErrConflict {
			http.Error(w, "file already exists", http.StatusConflict)
			return
		}
		if errors.Is(err, transfer.ErrChecksum) || errors.Is(err, transfer.ErrLength) {
			http.Error(w, "file integrity verification failed", http.StatusUnprocessableEntity)
			return
		}
		http.Error(w, "file receive failed", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "saved", "name": filepath.Base(path)})
}

// ListenAndServe starts the HTTP API on the supplied, already-validated
// listener address. The CLI deliberately does not invent that address.
func ListenAndServe(address, programDir string, token []byte) error {
	if strings.TrimSpace(address) == "" {
		return fmt.Errorf("服务监听地址尚未由现有 Xray 部署关系唯一确定")
	}
	host, port, splitErr := net.SplitHostPort(address)
	if splitErr != nil || port == "" {
		return fmt.Errorf("服务监听地址必须是明确的 IP:port")
	}
	portNumber, portErr := strconv.Atoi(port)
	if portErr != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("服务端口必须在 1 到 65535 之间")
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() {
		return fmt.Errorf("拒绝绑定非明确的本机单播 IP；不允许 0.0.0.0、:: 或 loopback")
	}
	assigned := false
	interfaces, interfaceErr := net.Interfaces()
	if interfaceErr == nil {
		for _, iface := range interfaces {
			addresses, _ := iface.Addrs()
			for _, address := range addresses {
				var candidate net.IP
				switch value := address.(type) {
				case *net.IPNet:
					candidate = value.IP
				case *net.IPAddr:
					candidate = value.IP
				}
				if candidate != nil && candidate.Equal(ip) {
					assigned = true
				}
			}
		}
	}
	if !assigned {
		return fmt.Errorf("监听 IP 不是本机网卡上的地址")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("无法监听文件服务地址：%w", err)
	}
	return ServeListener(listener, programDir, token)
}

func ServeListener(listener net.Listener, programDir string, token []byte) error {
	server := &http.Server{
		Handler:            NewHandler(programDir, token),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:        60 * time.Second,
		MaxHeaderBytes:     1 << 20,
	}
	return server.Serve(&idleListener{Listener: listener, timeout: 2 * time.Minute})
}

type idleListener struct {
	net.Listener
	timeout time.Duration
}

func (listener *idleListener) Accept() (net.Conn, error) {
	conn, err := listener.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &idleServerConn{Conn: conn, timeout: listener.timeout}, nil
}

type idleServerConn struct {
	net.Conn
	timeout time.Duration
}

func (conn *idleServerConn) Read(buffer []byte) (int, error) {
	_ = conn.Conn.SetReadDeadline(time.Now().Add(conn.timeout))
	return conn.Conn.Read(buffer)
}

func (conn *idleServerConn) Write(buffer []byte) (int, error) {
	_ = conn.Conn.SetWriteDeadline(time.Now().Add(conn.timeout))
	return conn.Conn.Write(buffer)
}
