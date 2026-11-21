package main

// GPS через geoclue 0.12 на **сессионной** шине D-Bus.
//
// Проверено на NS220 (2026-11-21):
//   сервис org.freedesktop.Geoclue.Providers.Hybris (geoclue-provider-hybris),
//   объект /org/freedesktop/Geoclue/Providers/Hybris;
//   org.freedesktop.Geoclue.GetStatus() -> i (0..3)
//   org.freedesktop.Geoclue.Position.GetPosition() -> iiddd(idd):
//     (status, timestamp, lat, lon, alt, fields)
//   без фикса lat/lon = NaN, fields=6 (широта+долгота), status=1.
//   org.freedesktop.Geoclue.Satellite — интерфейс есть (GetSatellite).
//
// Master (org.freedesktop.Geoclue.Master) для нашей задачи не нужен: провайдер
// можно опрашивать напрямую; мастер ругался «no usable Position providers»,
// пока GPS не получил фикс.
//
// Провайдер пользовательский: модуль запускает systemd-юнит
// geoclue-providers-hybris (disabled в прошивке) — при старте приложение
// активирует D-Bus-имя, а если не удалось — просит systemd --user запустить юнит.

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	geoService = "org.freedesktop.Geoclue.Providers.Hybris"
	geoPath    = dbus.ObjectPath("/org/freedesktop/Geoclue/Providers/Hybris")
)

type gpsState struct {
	mu     sync.Mutex
	status int32
	lat    float64
	lon    float64
	alt    float64
	fields uint32
	ok     bool
	t      time.Time
	err    string
	provid string
}

var gps = &gpsState{}

// StartGPS опрашивает провайдера geoclue раз в секунду (плюс сигналы).
func StartGPS() {
	go func() {
		conn, err := dbus.SessionBus()
		if err != nil {
			gps.setErr("сессионная шина: " + err.Error())
			return
		}
		// Провайдер в прошивке выключен (disabled), а его D-Bus-файл
		// Exec=/bin/false: одна активация по имени не поднимает сервис.
		// Поэтому всегда просим systemd --user запустить юнит (идемпотентно),
		// и лишь затем — обычную D-Bus-активацию.
		var owner string
		if err := conn.Object("org.freedesktop.DBus", "/org/freedesktop/DBus").
			Call("org.freedesktop.DBus.GetNameOwner", 0, geoService).Store(&owner); err != nil || owner == "" {
			var got string
			_ = conn.Object("org.freedesktop.DBus", "/org/freedesktop/DBus").
				Call("org.freedesktop.DBus.StartServiceByName", 0, geoService, uint32(0)).Store(&got)
			time.Sleep(500 * time.Millisecond)
		}

		_ = conn.AddMatchSignal(
			dbus.WithMatchObjectPath(geoPath),
			dbus.WithMatchInterface("org.freedesktop.Geoclue.Position"),
			dbus.WithMatchMember("PositionChanged"),
		)
		ch := make(chan *dbus.Signal, 16)
		conn.Signal(ch)
		go func() {
			for sig := range ch {
				if sig.Name == "org.freedesktop.Geoclue.Position.PositionChanged" {
					gps.parse(sig.Body)
				}
			}
		}()

		obj := conn.Object(geoService, geoPath)
		for {
			// GetProviderInfo -> (ss): два отдельных значения, не массив.
			var infoName, infoDesc string
			if err := obj.Call("org.freedesktop.Geoclue.GetProviderInfo", 0).Store(&infoName, &infoDesc); err == nil && infoDesc != "" {
				gps.mu.Lock()
				gps.provid = infoDesc
				gps.mu.Unlock()
			}
			// Если провайдер ещё не поднят — поднимаем (раз в цикл, дёшево).
			var o string
			if err := conn.Object("org.freedesktop.DBus", "/org/freedesktop/DBus").
				Call("org.freedesktop.DBus.GetNameOwner", 0, geoService).Store(&o); err != nil || o == "" {
				if err := startGeoUnit(conn); err != nil {
					gps.setErr("systemd StartUnit: " + err.Error())
				} else {
					gps.setErr("запускаю geoclue-providers-hybris…")
				}
			}

			var st int32
			if err := obj.Call("org.freedesktop.Geoclue.GetStatus", 0).Store(&st); err == nil {
				gps.mu.Lock()
				gps.status = st
				gps.mu.Unlock()
			}
			// Внимание: у GetPosition сложная сигнатура iiddd(idd), dbus.Store
			// в неё не раскладывается (length mismatch) — берём Body как есть.
			call := obj.Call("org.freedesktop.Geoclue.Position.GetPosition", 0)
			if call.Err == nil {
				gps.parse(call.Body)
			} else {
				gps.setErr("GetPosition: " + call.Err.Error())
			}
			time.Sleep(time.Second)
		}
	}()
}

// startGeoUnit — активация через systemd --user (юнит в прошивке disabled,
// D-Bus-файл провайдера указывает на юнит).
func startGeoUnit(conn *dbus.Conn) error {
	sysd := conn.Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1")
	var job dbus.ObjectPath
	call := sysd.Call("org.freedesktop.systemd1.Manager.StartUnit", 0,
		"geoclue-providers-hybris.service", "fail")
	if call.Err != nil {
		return call.Err
	}
	return call.Store(&job)
}

func (g *gpsState) parse(body []interface{}) {
	if len(body) < 6 {
		return
	}
	lat := toFloat(body[2])
	lon := toFloat(body[3])
	alt := toFloat(body[4])
	fields := uint32(toFloat(body[5]))

	g.mu.Lock()
	g.fields = fields
	if !math.IsNaN(lat) && !math.IsNaN(lon) && (lat != 0 || lon != 0) {
		g.lat, g.lon, g.alt = lat, lon, alt
		g.ok = true
		g.t = time.Now()
		g.err = ""
	}
	g.mu.Unlock()
}

func (g *gpsState) setErr(s string) {
	g.mu.Lock()
	g.err = s
	g.mu.Unlock()
}

// GPSState — снимок состояния для панели.
func GPSState() (status int32, lat, lon, alt float64, ok bool, age time.Duration, provider, errText string) {
	gps.mu.Lock()
	defer gps.mu.Unlock()
	if !gps.t.IsZero() {
		age = time.Since(gps.t)
	}
	return gps.status, gps.lat, gps.lon, gps.alt, gps.ok, age, gps.provid, gps.err
}

// GPSStatusText — человекочитаемый статус geoclue.
func GPSStatusText(st int32) string {
	switch st {
	case 0:
		return "ошибка"
	case 1:
		return "нет сигнала (поиск)"
	case 2:
		return "получение данных"
	case 3:
		return "фикс есть"
	}
	return fmt.Sprintf("статус %d", st)
}

// toFloat приводит значение из D-Bus (int32/float64/Variant) к float64.
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
