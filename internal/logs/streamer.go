package logs

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/sasha-s/go-deadlock"
	"io"
	"sync"

	"github.com/Vilsol/slox"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Vilsol/klados/internal/cluster"
)

const logBufSize = 1024

// Conn is the subset of a websocket connection the log plane needs. Both
// gofiber/websocket and gorilla/websocket conns satisfy it, keeping this
// package independent of the HTTP stack that upgraded the connection.
type Conn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
}

// RFC 6455 text opcode, matching the constant of both websocket libraries.
const textMessage = 1

type LogOptions struct {
	Follow     bool   `json:"follow"`
	TailLines  *int64 `json:"tailLines"`
	Timestamps bool   `json:"timestamps"`
	Previous   bool   `json:"previous"`
	Container  string `json:"container"`
}

type logStream struct {
	ctxName string
	ns      string
	podName string
	opts    LogOptions
	cancel  context.CancelFunc
	buf     chan []byte
}

type ConnectionProvider interface {
	GetConnection(contextName string) (*cluster.Connection, error)
}

type Streamer struct {
	mu      deadlock.Mutex
	streams map[string]*logStream
	connMgr ConnectionProvider
	ctx     context.Context
}

func NewStreamer(connMgr ConnectionProvider, ctx context.Context) *Streamer {
	return &Streamer{
		streams: make(map[string]*logStream),
		connMgr: connMgr,
		ctx:     ctx,
	}
}

func (s *Streamer) StartStream(ctxName, ns, podName string, opts LogOptions) (string, error) {
	if _, err := s.connMgr.GetConnection(ctxName); err != nil {
		return "", fmt.Errorf("getting connection: %w", err)
	}

	id, err := newID()
	if err != nil {
		return "", err
	}

	streamCtx, cancel := context.WithCancel(context.Background())
	stream := &logStream{
		ctxName: ctxName,
		ns:      ns,
		podName: podName,
		opts:    opts,
		cancel:  cancel,
		buf:     make(chan []byte, logBufSize),
	}

	s.mu.Lock()
	s.streams[id] = stream
	s.mu.Unlock()

	slox.Debug(s.ctx, "log stream started", "id", id, "pod", podName, "container", opts.Container, "ns", ns)

	if opts.Container == "" {
		go s.readAllContainers(streamCtx, stream, id)
	} else {
		go s.readLogs(streamCtx, stream, id)
	}
	return id, nil
}

func (s *Streamer) readLogs(ctx context.Context, stream *logStream, id string) {
	defer close(stream.buf)
	s.readLogsForContainer(ctx, stream, stream.opts, "")
}

func (s *Streamer) readAllContainers(ctx context.Context, stream *logStream, id string) {
	defer close(stream.buf)

	conn, err := s.connMgr.GetConnection(stream.ctxName)
	if err != nil {
		slox.Warn(s.ctx, "log stream lost connection", "id", id, "error", err)
		return
	}

	pod, err := conn.Clientset.CoreV1().Pods(stream.ns).Get(ctx, stream.podName, metav1.GetOptions{})
	if err != nil {
		slox.Warn(s.ctx, "log stream failed to get pod", "id", id, "error", err)
		return
	}

	var wg sync.WaitGroup
	for _, c := range pod.Spec.Containers {
		wg.Add(1)
		containerName := c.Name
		go func() {
			defer wg.Done()
			containerOpts := stream.opts
			containerOpts.Container = containerName
			s.readLogsForContainer(ctx, stream, containerOpts, "["+containerName+"] ")
		}()
	}
	wg.Wait()
}

func (s *Streamer) readLogsForContainer(ctx context.Context, stream *logStream, opts LogOptions, prefix string) {
	conn, err := s.connMgr.GetConnection(stream.ctxName)
	if err != nil {
		slox.Warn(s.ctx, "log stream lost connection", "container", opts.Container, "error", err)
		return
	}

	logOpts := &corev1.PodLogOptions{
		Container:  opts.Container,
		Follow:     opts.Follow,
		Timestamps: opts.Timestamps,
		Previous:   opts.Previous,
		TailLines:  opts.TailLines,
	}

	rc, err := conn.Clientset.CoreV1().Pods(stream.ns).GetLogs(stream.podName, logOpts).Stream(ctx)
	if err != nil {
		slox.Warn(s.ctx, "log stream open failed", "container", opts.Container, "error", err)
		msg, _ := json.Marshal(map[string]any{"type": "error", "message": err.Error()})
		select {
		case stream.buf <- msg:
		case <-ctx.Done():
		}
		return
	}
	defer rc.Close()

	if prefix == "" {
		buf := make([]byte, 4096)
		for {
			n, err := rc.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				select {
				case stream.buf <- chunk:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					slox.Warn(s.ctx, "log stream read error", "container", opts.Container, "error", err)
				}
				return
			}
		}
	} else {
		scanner := bufio.NewScanner(rc)
		for scanner.Scan() {
			if ctx.Err() != nil {
				return
			}
			line := []byte(prefix + scanner.Text() + "\n")
			select {
			case stream.buf <- line:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (s *Streamer) HandleConn(streamID string, conn Conn) {
	s.mu.Lock()
	stream, ok := s.streams[streamID]
	s.mu.Unlock()

	if !ok {
		_ = conn.WriteMessage(textMessage, []byte(`{"type":"error","message":"stream not found"}`))
		return
	}

	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req struct {
				Type        string `json:"type"`
				Count       int    `json:"count"`
				AlreadyHave int    `json:"alreadyHave"`
			}
			if err := json.Unmarshal(msg, &req); err != nil {
				continue
			}
			if req.Type == "load_history" && req.Count > 0 {
				lines, hasMore, err := s.fetchHistory(stream, req.Count, req.AlreadyHave)
				if err != nil {
					slox.Debug(s.ctx, "log history fetch failed", "id", streamID, "error", err)
					continue
				}
				resp, _ := json.Marshal(map[string]any{
					"type":     "history",
					"lines":    lines,
					"has_more": hasMore,
				})
				_ = conn.WriteMessage(textMessage, resp)
			}
		}
	}()

	for chunk := range stream.buf {
		if err := conn.WriteMessage(textMessage, chunk); err != nil {
			slox.Warn(s.ctx, "log ws write error", "id", streamID, "error", err)
			return
		}
	}
	_ = conn.WriteMessage(textMessage, []byte(`{"type":"eof"}`))

	slox.Debug(s.ctx, "log stream closed", "id", streamID)
	s.mu.Lock()
	delete(s.streams, streamID)
	s.mu.Unlock()
}

func (s *Streamer) fetchHistory(stream *logStream, count, alreadyHave int) ([]string, bool, error) {
	conn, err := s.connMgr.GetConnection(stream.ctxName)
	if err != nil {
		return nil, false, err
	}

	tailLines := int64(alreadyHave + count)
	logOpts := &corev1.PodLogOptions{
		Container:  stream.opts.Container,
		TailLines:  &tailLines,
		Timestamps: true,
	}

	rc, err := conn.Clientset.CoreV1().Pods(stream.ns).GetLogs(stream.podName, logOpts).Stream(context.Background())
	if err != nil {
		return nil, false, err
	}
	defer rc.Close()

	var lines []string
	totalScanned := 0
	scanner := bufio.NewScanner(rc)
	for scanner.Scan() {
		if totalScanned < count {
			lines = append(lines, scanner.Text())
		}
		totalScanned++
	}

	if int64(totalScanned) >= tailLines {
		// Normal case: pod has more history beyond what we fetched
		return lines, true, nil
	}

	// Pod has fewer lines than tailLines — some lines we kept may already be
	// in the client's buffer. Only return the genuinely new (oldest) lines.
	genuinelyNew := totalScanned - alreadyHave
	if genuinelyNew <= 0 {
		return []string{}, false, nil
	}
	if genuinelyNew < len(lines) {
		return lines[:genuinelyNew], false, nil
	}
	return lines, false, nil
}

func (s *Streamer) StopStream(streamID string) {
	s.mu.Lock()
	stream, ok := s.streams[streamID]
	if ok {
		delete(s.streams, streamID)
	}
	s.mu.Unlock()

	if ok {
		stream.cancel()
	}
}

func (s *Streamer) StopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, stream := range s.streams {
		stream.cancel()
		delete(s.streams, id)
	}
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
