package main

// Кнопки, подсветка, сон экрана и модем/SIM — то, что доступно без SIM-карты.
//
// Проверено 2026-11-21 на NS220:
//   * кнопки: /dev/input/event0 (ACCDET), event1 (mtk-kpd), event2 (elink_keys);
//     пользователь в группе input → evdev читается напрямую;
//   * подсветка: /sys/class/leds/lcd-backlight/brightness (system:system) —
//     только через setuid-хелпер `aurora-brightness` (0..255, сейчас 96);
//   * сон/пробуждение: com.nokia.mce (системная шина), методы
//     req_display_state_off / req_display_state_on / dim / lpm — доступны
//     пользователю (проверено);
//   * модем: ofono, /ril_0, online; SIM **отсутствует** (`Present=false`);
//     NetworkRegistration в этой прошивке нет (нет SIM) → звонки, включая 112,
//     недоступны; IMEI (Serial) = 356399112625294, ревизия модема MOLY.LR9.W1444.

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

// ---------- кнопки (evdev) ----------

type buttonsState struct {
	mu    sync.Mutex
	last  string
	count int
	t     time.Time
	devOK bool
	err   string
}

var buttons = &buttonsState{}

// keyNames — карта кнопок NS220 (снята вручную 2026-11-21, scripts/btncap.py):
// боковые POWER=116, VOLUMEUP=115, VOLUMEDOWN=114; передняя панель
// BACK=158, UP=59, DOWN=60, HOME=102 (передняя панель); Reset — аппаратный, без события.
var keyNames = map[uint16]string{
	114: "VOLUMEDOWN", 115: "VOLUMEUP", 116: "POWER", 102: "HOME",
	158: "BACK", 59: "UP(передняя)", 60: "DOWN(передняя)",
	139: "MENU", 143: "WAKEUP", 152: "POWER2", 212: "CAMERA",
	103: "UP", 108: "DOWN", 105: "LEFT", 106: "RIGHT", 28: "ENTER",
	224: "BRIGHTNESSDOWN", 225: "BRIGHTNESSUP",
}

type inputEvent struct {
	Sec, Usec  int32
	Type, Code uint16
	Value      int32
}

// StartButtons читает evdev-устройства кнопок (event0/1/2).
func StartButtons() {
	go func() {
		devs := []string{"/dev/input/event1", "/dev/input/event0", "/dev/input/event2"}
		var opened []*os.File
		for _, p := range devs {
			if f, err := os.Open(p); err == nil {
				opened = append(opened, f)
				lg.Printf("BUTTONS: открыт %s", p)
			}
		}
		if len(opened) == 0 {
			buttons.setErr("нет доступа к /dev/input/event* (нужна группа input)")
			return
		}
		buttons.mu.Lock()
		buttons.devOK = true
		buttons.mu.Unlock()

		buf := make([]byte, 16) // struct input_event на 32-битном ARM
		for _, f := range opened {
			go func(file *os.File) {
				for {
					if _, err := io.ReadFull(file, buf); err != nil {
						return
					}
					ev := inputEvent{
						Sec:   int32(binary.LittleEndian.Uint32(buf[0:4])),
						Usec:  int32(binary.LittleEndian.Uint32(buf[4:8])),
						Type:  binary.LittleEndian.Uint16(buf[8:10]),
						Code:  binary.LittleEndian.Uint16(buf[10:12]),
						Value: int32(binary.LittleEndian.Uint32(buf[12:16])),
					}
					if ev.Type != 1 || ev.Value != 1 { // EV_KEY, нажатие
						continue
					}
					name := keyNames[ev.Code]
					if name == "" {
						name = fmt.Sprintf("KEY_%d", ev.Code)
					}
					buttons.mu.Lock()
					buttons.last = name
					buttons.count++
					buttons.t = time.Now()
					buttons.mu.Unlock()
					lg.Printf("BUTTON: %s (code %d)", name, ev.Code)
				}
			}(f)
		}
	}()
}

func (b *buttonsState) setErr(s string) {
	b.mu.Lock()
	b.err = s
	b.devOK = false
	b.mu.Unlock()
}

// ButtonsState — последняя кнопка, счётчик, свежесть, ошибка.
func ButtonsState() (last string, count int, age time.Duration, ok bool, err string) {
	buttons.mu.Lock()
	defer buttons.mu.Unlock()
	if !buttons.t.IsZero() {
		age = time.Since(buttons.t)
	}
	return buttons.last, buttons.count, age, buttons.devOK, buttons.err
}

// ---------- подсветка ----------

// Brightness возвращает (текущая, максимум) или ошибку.
func brightnessBin() string {
	for _, p := range []string{"/usr/bin/aurora-brightness", "/usr/local/bin/aurora-brightness"} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return "/usr/local/bin/aurora-brightness"
}

func Brightness() (int, int, error) {
	out, err := exec.Command(brightnessBin()).Output()
	if err != nil {
		lg.Printf("BRIGHT: %v", err)
		return 0, 0, err
	}
	var cur, max int
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d %d", &cur, &max)
	return cur, max, nil
}

// SetBrightness меняет яркость на delta (или задаёт абсолют, если absolute >= 0).
func SetBrightness(delta int) (int, int, error) {
	arg := fmt.Sprintf("%+d", delta)
	out, err := exec.Command(brightnessBin(), arg).Output()
	if err != nil {
		return 0, 0, err
	}
	var cur, max int
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d %d", &cur, &max)
	return cur, max, nil
}

// ---------- сон экрана (mce) ----------

func displayState(on bool) error {
	conn, err := dbus.SystemBus()
	if err != nil {
		return err
	}
	method := "req_display_state_off"
	if on {
		method = "req_display_state_on"
	}
	obj := conn.Object("com.nokia.mce", "/com/nokia/mce/request")
	return obj.Call("com.nokia.mce.request."+method, 0).Err
}

// DisplayOff/DisplayOn — погасить/включить экран через mce.
func DisplayOff() error { return displayState(false) }
func DisplayOn() error  { return displayState(true) }

// ---------- модем / SIM (ofono) ----------

type modemState struct {
	mu       sync.Mutex
	path     dbus.ObjectPath
	ok       bool
	online   bool
	imei     string
	revision string
	sim      bool
	err      string
}

var modem = &modemState{}

// StartModem читает свойства модема/SIM из ofono (раз в 5 с).
func StartModem() {
	go func() {
		for {
			readModem()
			time.Sleep(5 * time.Second)
		}
	}()
}

func readModem() {
	conn, err := dbus.SystemBus()
	if err != nil {
		modem.setErr("шина: " + err.Error())
		return
	}
	var raw []interface{}
	call := conn.Object("org.ofono", "/").Call("org.ofono.Manager.GetModems", 0)
	if call.Err != nil {
		modem.setErr("GetModems: " + call.Err.Error())
		return
	}
	if err := call.Store(&raw); err != nil || len(raw) == 0 {
		modem.setErr("модем не найден")
		return
	}
	pair, _ := raw[0].([]interface{})
	if len(pair) < 2 {
		modem.setErr("нет данных модема")
		return
	}
	path, _ := pair[0].(dbus.ObjectPath)
	props, _ := pair[1].(map[string]dbus.Variant)
	if props == nil {
		modem.setErr("GetModems: пустые свойства")
		return
	}

	s := modemState{ok: true, path: path}
	if v, ok := props["Online"]; ok {
		s.online, _ = v.Value().(bool)
	}
	if v, ok := props["Serial"]; ok {
		s.imei, _ = v.Value().(string)
	}
	if v, ok := props["Revision"]; ok {
		s.revision, _ = v.Value().(string)
	}
	// SIM
	var sp map[string]dbus.Variant
	if err := conn.Object("org.ofono", path).Call("org.ofono.SimManager.GetProperties", 0).Store(&sp); err == nil {
		if v, ok := sp["Present"]; ok {
			s.sim, _ = v.Value().(bool)
		}
	}
	// ВАЖНО: не присваивать *modem = s — в структуре есть sync.Mutex,
	// копирование под локом ломает его («unlock of unlocked mutex»).
	modem.mu.Lock()
	modem.path = s.path
	modem.ok = true
	modem.online = s.online
	modem.imei = s.imei
	modem.revision = s.revision
	modem.sim = s.sim
	modem.err = ""
	modem.mu.Unlock()
}

func (m *modemState) setErr(s string) {
	m.mu.Lock()
	m.err = s
	m.ok = false
	m.mu.Unlock()
}

// ModemState — снимок для панели.
func ModemState() (ok, online bool, imei, revision string, sim bool, errText string) {
	modem.mu.Lock()
	defer modem.mu.Unlock()
	return modem.ok, modem.online, modem.imei, modem.revision, modem.sim, modem.err
}

// ---------- «не гасить экран», пока приложение открыто ----------
//
// Выяснено 2026-11-21: когда дисплей гаснет по таймауту, Android sensors HAL
// приостанавливает сенсоры (ACC/компас = 0, helper молчит), а подсветка = 0.
// Поэтому приложение периодически продлевает «паузу гашения» через mce
// (req_display_blanking_pause), пока оно на переднем плане.

// StartKeepAwake продлевает паузу гашения экрана каждые 5 с.
func StartKeepAwake() {
	go func() {
		conn, err := dbus.SystemBus()
		if err != nil {
			lg.Printf("KEEPAWAKE: шина недоступна: %v", err)
			return
		}
		obj := conn.Object("com.nokia.mce", "/com/nokia/mce/request")
		failed := false
		for {
			err := obj.Call("com.nokia.mce.request.req_display_blanking_pause", 0).Err
			if err != nil && !failed {
				lg.Printf("KEEPAWAKE: req_display_blanking_pause: %v", err)
				failed = true
			}
			time.Sleep(5 * time.Second)
		}
	}()
}
