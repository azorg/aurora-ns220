// Package core — бизнес-логика приложения. Никакого UI.
//
// Принцип гибридной схемы: вся логика живёт здесь и переиспользуется
// любым фронтендом (Fyne, Qt/QML, CLI, тесты).
package core

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Service — пример сервиса с состоянием.
type Service struct {
	mu    sync.Mutex
	count int
	name  string
}

func New() *Service {
	return &Service{name: "мир"}
}

// Hello возвращает приветствие (чистая бизнес-логика).
func (s *Service) Hello(who string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if who != "" {
		s.name = who
	}
	s.count++
	return fmt.Sprintf("Привет, %s! (обращений: %d)", s.name, s.count)
}

// Status — сводка состояния.
func (s *Service) Status() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{"name": s.name, "count": s.count}
}

// Request/Response — минимальный JSON-протокол для межъязыкового IPC.
// Строка JSON на запрос, строка JSON на ответ (JSON-lines).
type Request struct {
	Cmd  string `json:"cmd"`
	Arg  string `json:"arg,omitempty"`
	JSON string `json:"json,omitempty"`
}

type Response struct {
	OK    bool   `json:"ok"`
	Text  string `json:"text,omitempty"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// Handle обрабатывает один запрос.
func (s *Service) Handle(req Request) Response {
	switch strings.ToLower(req.Cmd) {
	case "", "hello":
		return Response{OK: true, Text: s.Hello(req.Arg)}
	case "status":
		return Response{OK: true, Data: s.Status()}
	case "ping":
		return Response{OK: true, Text: "pong"}
	default:
		return Response{OK: false, Error: "unknown cmd: " + req.Cmd}
	}
}

// Serve — цикл JSON-lines по потокам (используется core-daemon для Qt/CLI).
func (s *Service) Serve(r io.Reader, w io.Writer) error {
	in := bufio.NewScanner(r)
	in.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	out := bufio.NewWriter(w)
	defer out.Flush()

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req Request
		var resp Response
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp = Response{OK: false, Error: "bad json: " + err.Error()}
		} else {
			resp = s.Handle(req)
		}
		b, _ := json.Marshal(resp)
		if _, err := out.Write(append(b, '\n')); err != nil {
			return err
		}
		if err := out.Flush(); err != nil {
			return err
		}
	}
	return in.Err()
}
