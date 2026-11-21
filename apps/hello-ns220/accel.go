package main

// G-сенсор, компас и ориентация на NS220/Aurora.
//
// Почему через helper, а не напрямую:
//   * D-Bus API sensorfw (local.AccelerometerSensor.dataAvailable) данные не отдаёт —
//     это управляющий интерфейс; данные идут по внутреннему сокету клиентской
//     библиотеке sensorfw;
//   * прямой Android sensors HAL — binderized (vtable NULL, см. bugs B037);
//   * штатный путь — QtSensors → sensorfw, поэтому собираем маленький helper
//     `sensor-reader` (helper/sensor-reader.cpp) и читаем его строки:
//         ACC x y z ts      — ускорение, м/с²
//         MAG azimuth level 0 ts — компас (азимут, градусы)
//         ORT n ts          — ориентация (QOrientationReading)
//
// Проверено 2026-11-21: ACC ≈ (1.1, -3.2, 9.07), MAG ≈ 234°, ORT 5 (экраном вверх).
//
// Важно: sensorfwd должен быть запущен ПОСЛЕ Android sensors HAL; при старте
// системы он иногда ловит «Poll failed with status 2» и не восстанавливается —
// лечится `systemctl restart sensorfwd` (см. drop-in в packaging/sensorfwd/).

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type accelState struct {
	mu      sync.Mutex
	x, y, z float64
	azimuth float64
	orient  int
	ok      bool
	t       time.Time
	err     string
}

var accel = &accelState{orient: -1}

// helperCandidates — где искать sensor-reader (сначала установленный пакетом).
func helperCandidates() []string {
	var out []string
	if p := os.Getenv("SENSOR_READER"); p != "" {
		out = append(out, p)
	}
	out = append(out, "/usr/libexec/aurora-hello-ns220/sensor-reader")
	if h, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(h, "hello-ns220/helper/sensor-reader"))
	}
	out = append(out, "/home/user/hello-ns220/helper/sensor-reader")
	return out
}

func findSensorReader() string {
	for _, p := range helperCandidates() {
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

// StartAccel запускает helper и читает его вывод. Если за 5 с не пришло ни
// одного ACC (sensorfw/HAL иногда «залипает» после предыдущего клиента),
// helper перезапускается; после трёх неудач — сброс sensorfwd через
// setuid-хелпер `aurora-sensor-reset` (если установлен).
func StartAccel() {
	go func() {
		fails := 0
		for {
			path := findSensorReader()
			if path == "" {
				accel.setErr("нет sensor-reader (QtSensors helper не собран/не установлен)")
				time.Sleep(3 * time.Second)
				continue
			}
			cmd := exec.Command(path)
			cmd.Env = append(os.Environ(), "QT_LOGGING_RULES=qt.*=false")
			// helper не должен переживать приложение: иначе он держит сессию
			// сенсора, и следующий запуск остаётся без ACC (B043).
			cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				accel.setErr("helper: " + err.Error())
				time.Sleep(2 * time.Second)
				continue
			}
			cmd.Stderr = nil
			if err := cmd.Start(); err != nil {
				accel.setErr("helper: " + err.Error())
				time.Sleep(2 * time.Second)
				continue
			}
			lg.Printf("ACCEL: запущен %s (pid %d)", path, cmd.Process.Pid)

			dbgLines := 0
			gotACC := make(chan struct{}, 1)
			done := make(chan struct{})
			go func() {
				sc := bufio.NewScanner(stdout)
				for sc.Scan() {
					line := sc.Text()
					if strings.HasPrefix(line, "ACC ") {
						select {
						case gotACC <- struct{}{}:
						default:
						}
					}
					if !strings.HasPrefix(line, "ACC ") && !strings.HasPrefix(line, "MAG ") && !strings.HasPrefix(line, "ORT ") {
						if dbgLines < 40 {
							lg.Printf("ACCEL helper: %s", line)
							dbgLines++
						}
					}
					accel.parseLine(line)
				}
				close(done)
			}()

			ok := false
			select {
			case <-gotACC:
				ok = true
			case <-done:
			case <-time.After(5 * time.Second):
			}
			if ok {
				fails = 0
				<-done
			} else {
				fails++
				lg.Printf("ACCEL: нет данных за 5 с (попытка %d) — перезапуск helper", fails)
				_ = cmd.Process.Kill()
				<-done
				if fails >= 3 {
					sensorReset()
					fails = 0
				}
			}
			_ = cmd.Wait()
			time.Sleep(1 * time.Second)
		}
	}()
}

// sensorReset перезапускает sensorfwd через setuid-хелпер (если он есть).
func sensorReset() {
	for _, p := range []string{"/usr/bin/aurora-sensor-reset", "/usr/local/bin/aurora-sensor-reset", "/home/user/sensor-reset"} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			lg.Printf("ACCEL: сброс sensorfwd через %s", p)
			_ = exec.Command(p).Run()
			time.Sleep(2 * time.Second)
			return
		}
	}
}

// parseLine разбирает строки helper-а.
func (a *accelState) parseLine(line string) {
	f := strings.Fields(line)
	if len(f) < 2 {
		return
	}
	switch f[0] {
	case "ACC":
		if len(f) < 4 {
			return
		}
		x, err1 := parseNum(f[1])
		y, err2 := parseNum(f[2])
		z, err3 := parseNum(f[3])
		if err1 != nil || err2 != nil || err3 != nil {
			return
		}
		a.mu.Lock()
		a.x, a.y, a.z = x, y, z
		a.t = time.Now()
		a.ok = true
		a.err = ""
		a.mu.Unlock()
	case "MAG":
		if v, err := parseNum(f[1]); err == nil {
			a.mu.Lock()
			a.azimuth = v
			a.mu.Unlock()
		}
	case "ORT":
		if v, err := strconv.Atoi(f[1]); err == nil {
			a.mu.Lock()
			a.orient = v
			a.mu.Unlock()
		}
	}
}

// parseNum принимает и точку, и запятую (локаль ru_RU в helper-е).
func parseNum(s string) (float64, error) {
	return strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
}

func (a *accelState) setErr(s string) {
	a.mu.Lock()
	a.err = s
	a.ok = false
	a.mu.Unlock()
}

// AccelState — снимок: ускорение, модуль, азимут, ориентация, свежесть, ошибка.
func AccelState() (x, y, z, g, azimuth float64, orient int, ok bool, age time.Duration, err string) {
	accel.mu.Lock()
	defer accel.mu.Unlock()
	if !accel.t.IsZero() {
		age = time.Since(accel.t)
	}
	if accel.ok {
		g = math.Sqrt(accel.x*accel.x + accel.y*accel.y + accel.z*accel.z)
	}
	return accel.x, accel.y, accel.z, g, accel.azimuth, accel.orient, accel.ok, age, accel.err
}

// OrientationText — человекочитаемое положение (QOrientationReading).
func OrientationText(n int) string {
	switch n {
	case 0:
		return "не определено"
	case 1:
		return "портрет"
	case 2:
		return "ландшафт"
	case 3:
		return "портрет перевёрнут"
	case 4:
		return "ландшафт перевёрнут"
	case 5:
		return "экраном вверх"
	case 6:
		return "экраном вниз"
	}
	return fmt.Sprintf("код %d", n)
}

// TiltText — наклон из ускорения: тангаж (вперёд/назад) и крен (влево/вправо).
func TiltText(x, y, z float64) string {
	if x == 0 && y == 0 && z == 0 {
		return "—"
	}
	pitch := math.Atan2(-x, math.Sqrt(y*y+z*z)) * 180 / math.Pi
	roll := math.Atan2(y, z) * 180 / math.Pi
	return fmt.Sprintf("тангаж %+.0f° крен %+.0f°", pitch, roll)
}
