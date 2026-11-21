package main

// G-сенсор (акселерометр) через sensorfw: системная шина D-Bus,
// сервис com.nokia.SensorService, интерфейс local.SensorManager.
//
// Проверено на NS220 (2026-11-21):
//   loadPlugin("accelerometersensor") -> true
//   requestSensor("accelerometersensor", 100) -> id >= 0 (интервал в мс)
//   объект /SensorManager/accelerometersensor, интерфейс
//   local.AccelerometerSensor, сигнал dataAvailable(tiii):
//   (timestamp, x, y, z), значения — в тысячных м/с² (scale=1000).
//
// Данные могут отсутствовать без физического движения — это нормально.

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

type accelState struct {
	mu      sync.Mutex
	x, y, z float64
	t       time.Time
	ok      bool
	err     string
}

var accel = &accelState{}

// StartAccel подписывается на акселерометр и обновляет состояние в фоне.
func StartAccel() {
	go func() {
		conn, err := dbus.SystemBus()
		if err != nil {
			accel.setErr("шина: " + err.Error())
			return
		}
		mgr := conn.Object("com.nokia.SensorService", "/SensorManager")

		var loaded bool
		if err := mgr.Call("local.SensorManager.loadPlugin", 0, "accelerometersensor").Store(&loaded); err != nil {
			accel.setErr("loadPlugin: " + err.Error())
			return
		}
		var id int32
		if err := mgr.Call("local.SensorManager.requestSensor", 0,
			"accelerometersensor", int64(100)).Store(&id); err != nil || id < 0 {
			accel.setErr(fmt.Sprintf("requestSensor: id=%d err=%v", id, err))
			return
		}

		path := dbus.ObjectPath("/SensorManager/accelerometersensor")
		if err := conn.AddMatchSignal(
			dbus.WithMatchObjectPath(path),
			dbus.WithMatchInterface("local.AccelerometerSensor"),
			dbus.WithMatchMember("dataAvailable"),
		); err != nil {
			accel.setErr("AddMatch: " + err.Error())
			return
		}
		ch := make(chan *dbus.Signal, 32)
		conn.Signal(ch)

		// На случай, если сигналы уже идут, но объект ещё не готов —
		// пробуем и опрос (интерфейс не даёт «get», только сигнал).
		for sig := range ch {
			if sig.Name != "local.AccelerometerSensor.dataAvailable" || len(sig.Body) < 4 {
				continue
			}
			x := toFloat(sig.Body[1]) / 1000.0
			y := toFloat(sig.Body[2]) / 1000.0
			z := toFloat(sig.Body[3]) / 1000.0
			accel.set(x, y, z)
		}
	}()
}

func (a *accelState) set(x, y, z float64) {
	a.mu.Lock()
	a.x, a.y, a.z = x, y, z
	a.t = time.Now()
	a.ok = true
	a.err = ""
	a.mu.Unlock()
}

func (a *accelState) setErr(s string) {
	a.mu.Lock()
	a.err = s
	a.ok = false
	a.mu.Unlock()
}

// AccelState — (x, y, z, модуль, есть ли данные, свежесть, ошибка).
func AccelState() (x, y, z, g float64, ok bool, age time.Duration, err string) {
	accel.mu.Lock()
	defer accel.mu.Unlock()
	if !accel.t.IsZero() {
		age = time.Since(accel.t)
	}
	if accel.ok {
		g = math.Sqrt(accel.x*accel.x + accel.y*accel.y + accel.z*accel.z)
	}
	return accel.x, accel.y, accel.z, g, accel.ok, age, accel.err
}

func toFloat(v interface{}) float64 {
	switch t := v.(type) {
	case int32:
		return float64(t)
	case int64:
		return float64(t)
	case uint32:
		return float64(t)
	case uint64:
		return float64(t)
	case float64:
		return t
	case dbus.Variant:
		return toFloat(t.Value())
	}
	return 0
}
