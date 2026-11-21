package main

// Один экземпляр приложения + «поднять окно».
//
// Зачем: lipstick для не-booster приложений (Exec без invoker) при «закрытии»
// убирает окно с рабочего стола, но процесс не завершает — в итоге остаётся
// невидимый экземпляр. Повторный запуск из меню (invoker --single-instance)
// тоже просто стартует второй процесс, и внешне «приложение не открывается».
//
// Решение: при старте, если сокет уже занят — просим первый экземпляр показать
// окно и выходим; иначе слушаем сокет и по команде «show» поднимаем окно.
// Сокет живёт в $XDG_RUNTIME_DIR (удаляется при выходе).

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
)

type singleInstance struct {
	path string
	ln   net.Listener
}

func sockPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = "/tmp"
	}
	return filepath.Join(dir, "aurora-hello-ns220.sock")
}

// acquireSingleInstance возвращает (nil, true), если уже есть работающий
// экземпляр (тогда вызывающий должен выйти), либо живой объект для этого процесса.
func acquireSingleInstance(w fyne.Window) (*singleInstance, bool) {
	path := sockPath()
	if c, err := net.Dial("unix", path); err == nil {
		_, _ = c.Write([]byte("show\n"))
		_ = c.Close()
		lg.Printf("SINGLE: уже запущено, попросили показать окно (%s)", path)
		return nil, true
	}
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		lg.Printf("SINGLE: сокет недоступен (%v) — работаем без него", err)
		return nil, false
	}
	si := &singleInstance{path: path, ln: ln}
	go si.serve(w)
	return si, false
}

func (s *singleInstance) serve(w fyne.Window) {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		cmd, _ := bufio.NewReader(c).ReadString('\n')
		_ = c.Close()
		switch strings.TrimSpace(cmd) {
		case "show":
			lg.Printf("SINGLE: получена команда show")
			fyne.Do(func() {
				w.Show()
				w.RequestFocus()
			})
		case "exit":
			lg.Printf("SINGLE: получена команда exit")
			fyne.Do(func() { w.Close() })
		}
	}
}

func (s *singleInstance) close() {
	if s == nil {
		return
	}
	_ = s.ln.Close()
	_ = os.Remove(s.path)
}
