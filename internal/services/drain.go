package services

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/sasha-s/go-deadlock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubectl/pkg/drain"
)

type DrainSession struct {
	contextName string
	nodeName    string
	cancel      context.CancelFunc
}

type DrainService struct {
	appService *AppService
	sessions   map[string]*DrainSession
	mu         deadlock.Mutex
	ctx        context.Context
}

func NewDrainService(appSvc *AppService) *DrainService {
	return &DrainService{
		appService: appSvc,
		sessions:   make(map[string]*DrainSession),
	}
}

func (s *DrainService) Startup(ctx context.Context) error {
	s.ctx = ctx
	return nil
}

func (s *DrainService) Shutdown() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sess := range s.sessions {
		sess.cancel()
	}
	return nil
}

func sessionKey(contextName, nodeName string) string {
	return contextName + ":" + nodeName
}

func (s *DrainService) emit(name string, data any) {
	s.appService.Emit(name, data)
}

func (s *DrainService) StartDrain(contextName, nodeName string) error {
	s.mu.Lock()
	key := sessionKey(contextName, nodeName)
	if _, exists := s.sessions[key]; exists {
		s.mu.Unlock()
		return fmt.Errorf("drain already active for node %s", nodeName)
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.sessions[key] = &DrainSession{
		contextName: contextName,
		nodeName:    nodeName,
		cancel:      cancel,
	}
	s.mu.Unlock()

	s.emit(fmt.Sprintf("drain:%s:updated", contextName), nil)

	go func() {
		eventName := fmt.Sprintf("drain:%s:%s", contextName, nodeName)
		defer func() {
			s.mu.Lock()
			delete(s.sessions, key)
			s.mu.Unlock()
			s.emit(fmt.Sprintf("drain:%s:updated", contextName), nil)
		}()

		conn, err := s.appService.ClusterManager().GetConnection(contextName)
		if err != nil {
			s.emit(eventName, map[string]string{"type": "error", "message": err.Error()})
			s.emit(eventName, map[string]string{"type": "complete"})
			return
		}

		node, err := conn.Clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			s.emit(eventName, map[string]string{"type": "error", "message": err.Error()})
			s.emit(eventName, map[string]string{"type": "complete"})
			return
		}

		writer := &drainWriter{emit: func(msg string) {
			s.emit(eventName, map[string]string{"type": "log", "message": msg})
		}}

		helper := &drain.Helper{
			Ctx:                 ctx,
			Client:              conn.Clientset,
			IgnoreAllDaemonSets: true,
			DeleteEmptyDirData:  true,
			Timeout:             5 * time.Minute,
			Out:                 writer,
			ErrOut:              writer,
		}

		if err := drain.RunCordonOrUncordon(helper, node, true); err != nil {
			if ctx.Err() != nil {
				s.emit(eventName, map[string]string{"type": "cancelled"})
				return
			}
			s.emit(eventName, map[string]string{"type": "error", "message": err.Error()})
			s.emit(eventName, map[string]string{"type": "complete"})
			return
		}

		if err := drain.RunNodeDrain(helper, nodeName); err != nil {
			if ctx.Err() != nil {
				s.emit(eventName, map[string]string{"type": "cancelled"})
				return
			}
			s.emit(eventName, map[string]string{"type": "error", "message": err.Error()})
		}

		s.emit(eventName, map[string]string{"type": "complete"})
	}()

	return nil
}

func (s *DrainService) CancelDrain(contextName, nodeName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sessionKey(contextName, nodeName)
	sess, exists := s.sessions[key]
	if !exists {
		return fmt.Errorf("no active drain for node %s", nodeName)
	}
	sess.cancel()
	return nil
}

func (s *DrainService) IsActive(contextName, nodeName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.sessions[sessionKey(contextName, nodeName)]
	return exists
}

func (s *DrainService) ListActive(contextName string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []string
	for key, sess := range s.sessions {
		if sess.contextName == contextName {
			_ = key
			result = append(result, sess.nodeName)
		}
	}
	return result
}

type drainWriter struct {
	emit func(string)
	buf  bytes.Buffer
}

func (w *drainWriter) Write(p []byte) (n int, err error) {
	w.buf.Write(p)
	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			if line != "" {
				w.buf.Reset()
				w.buf.WriteString(line)
			}
			break
		}
		msg := line[:len(line)-1]
		if msg != "" {
			w.emit(msg)
		}
	}
	return len(p), nil
}

// Cordon marks the node as unschedulable.
func (s *DrainService) CordonNode(contextName, nodeName string) error {
	conn, err := s.appService.ClusterManager().GetConnection(contextName)
	if err != nil {
		return err
	}
	node, err := conn.Clientset.CoreV1().Nodes().Get(s.ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	helper := &drain.Helper{Ctx: s.ctx, Client: conn.Clientset, Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}}
	return drain.RunCordonOrUncordon(helper, node, true)
}

// Uncordon marks the node as schedulable.
func (s *DrainService) UncordonNode(contextName, nodeName string) error {
	conn, err := s.appService.ClusterManager().GetConnection(contextName)
	if err != nil {
		return err
	}
	node, err := conn.Clientset.CoreV1().Nodes().Get(s.ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	helper := &drain.Helper{Ctx: s.ctx, Client: conn.Clientset, Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}}
	return drain.RunCordonOrUncordon(helper, node, false)
}
