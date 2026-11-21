package main

// Звук на Aurora: PulseAudio через libpulse-simple (cgo).
//
// Проверено 2026-11-21: `pa_simple_new` + `pa_simple_write` играют тон на
// sink.primary_output (module-droid-card, MTK); `pa_simple_read` с source.droid
// даёт микрофон. Пакеты: pulseaudio + nemo-модули; dev-заголовков в системе нет,
// поэтому прототипы объявлены здесь (ABI pa_sample_spec стабилен: int, uint32,
// uint8_t), а линкуемся прямо на soname (`-l:libpulse-simple.so.0`), т.к.
// симлинков разработчика на устройстве нет.
//
//	PA_STREAM_PLAYBACK = 1, PA_STREAM_RECORD = 2
//	PA_SAMPLE_S16LE    = 3

/*
// Симлинки libpulse*.so -> libpulse*.so.0 лежат в /home/opt/libs (dev-пакета
// на устройстве нет). -L/home/opt/libs приходит из CGO_LDFLAGS в build.sh.
#cgo LDFLAGS: -lpulse-simple -lpulse
#include <stdlib.h>
#include <stdint.h>

typedef struct pa_simple pa_simple;
typedef struct { int format; uint32_t rate; uint8_t channels; } pa_sample_spec_aurora;

extern pa_simple* pa_simple_new(const char* server, const char* name, int dir,
                                const char* dev, const char* stream_name,
                                const pa_sample_spec_aurora* ss, const void* map,
                                const void* attr, int* error);
extern void pa_simple_free(pa_simple* s);
extern int  pa_simple_write(pa_simple* s, const void* data, size_t bytes, int* error);
extern int  pa_simple_read(pa_simple* s, void* data, size_t bytes, int* error);
extern int  pa_simple_drain(pa_simple* s, int* error);
*/
import "C"

import (
	"fmt"
	"math"
	"sync"
	"time"
	"unsafe"
)

const (
	audioRate     = 44100
	paPlayback    = 1
	paRecord      = 2
	paSampleS16LE = 3
)

var audioMu sync.Mutex

// PlayTone проигрывает синус freq Гц длительностью d (моно, 44.1 кГц, s16le)
// на устройстве вывода по умолчанию. Блокирующая — вызывать из горутины.
func PlayTone(freq float64, d time.Duration) error {
	if freq <= 0 {
		freq = 440
	}
	n := int(float64(audioRate) * d.Seconds())
	if n <= 0 {
		return nil
	}
	buf := make([]int16, n)
	for i := 0; i < n; i++ {
		// плавные фронты 5 мс, чтобы не щёлкало
		env := 1.0
		ms := float64(i) / float64(audioRate)
		const ramp = 0.005
		if ms < ramp {
			env = ms / ramp
		} else if rem := d.Seconds() - ms; rem < ramp {
			env = rem / ramp
		}
		buf[i] = int16(11000 * env * math.Sin(2*math.Pi*freq*ms))
	}

	audioMu.Lock()
	defer audioMu.Unlock()
	var perr C.int
	name := C.CString("aurora-hello-ns220")
	stream := C.CString("tone")
	defer C.free(unsafe.Pointer(name))
	defer C.free(unsafe.Pointer(stream))
	ss := C.pa_sample_spec_aurora{format: paSampleS16LE, rate: audioRate, channels: 1}
	s := C.pa_simple_new(nil, name, paPlayback, nil, stream, &ss, nil, nil, &perr)
	if s == nil {
		return fmt.Errorf("pa_simple_new(playback): код %d", int(perr))
	}
	defer C.pa_simple_free(s)
	if rc := C.pa_simple_write(s, unsafe.Pointer(&buf[0]), C.size_t(len(buf)*2), &perr); rc < 0 {
		return fmt.Errorf("pa_simple_write: код %d", int(perr))
	}
	_ = C.pa_simple_drain(s, &perr)
	return nil
}

// micMeter — состояние измерителя микрофона.
type micMeter struct {
	mu      sync.Mutex
	db      float64 // сглаженный уровень, dBFS (-90..0)
	peak    float64
	peakAt  time.Time
	started bool
	stop    chan struct{}
	err     string
	running bool
}

var mic = &micMeter{db: -90, peak: -90, stop: make(chan struct{})}

// StartMic запускает непрерывный захват с микрофона и расчёт уровня.
// device — имя источника PulseAudio ("" — по умолчанию).
func StartMic() {
	mic.mu.Lock()
	if mic.started {
		mic.mu.Unlock()
		return
	}
	mic.started = true
	mic.running = true
	mic.mu.Unlock()
	go mic.loop()
}

// MicState — текущие значения: сглаженный dBFS, пик за последнюю секунду,
// признак работы и текст ошибки (если была).
func MicState() (db, peak float64, running bool, errText string) {
	mic.mu.Lock()
	defer mic.mu.Unlock()
	// пик держим ~1 с
	if time.Since(mic.peakAt) > time.Second {
		mic.peak = mic.db
	}
	return mic.db, mic.peak, mic.running, mic.err
}

func (m *micMeter) setErr(s string) {
	m.mu.Lock()
	m.err = s
	m.running = false
	m.mu.Unlock()
}

func (m *micMeter) loop() {
	name := C.CString("aurora-hello-ns220")
	stream := C.CString("microphone")
	defer C.free(unsafe.Pointer(name))
	defer C.free(unsafe.Pointer(stream))
	ss := C.pa_sample_spec_aurora{format: paSampleS16LE, rate: audioRate, channels: 1}

	const chunk = 1024
	buf := make([]int16, chunk)

	for {
		select {
		case <-m.stop:
			return
		default:
		}
		var perr C.int
		s := C.pa_simple_new(nil, name, paRecord, nil, stream, &ss, nil, nil, &perr)
		if s == nil {
			m.setErr(fmt.Sprintf("микрофон: pa_simple_new код %d", int(perr)))
			time.Sleep(time.Second)
			continue
		}
		m.mu.Lock()
		m.running = true
		m.err = ""
		m.mu.Unlock()

		for {
			select {
			case <-m.stop:
				C.pa_simple_free(s)
				return
			default:
			}
			if rc := C.pa_simple_read(s, unsafe.Pointer(&buf[0]), C.size_t(len(buf)*2), &perr); rc < 0 {
				m.setErr(fmt.Sprintf("микрофон: pa_simple_read код %d", int(perr)))
				break
			}
			var sum float64
			var peak int32
			for _, v := range buf {
				f := float64(v)
				sum += f * f
				if v > 0 {
					if int32(v) > peak {
						peak = int32(v)
					}
				} else if -int32(v) > peak {
					peak = -int32(v)
				}
			}
			rms := math.Sqrt(sum / float64(len(buf)))
			db := 20 * math.Log10(math.Max(rms, 1)/32768.0)
			if db < -90 {
				db = -90
			}
			m.mu.Lock()
			if db > m.db {
				m.db = db
			} else {
				m.db = m.db*0.75 + db*0.25
			}
			if m.db > m.peak {
				m.peak = m.db
				m.peakAt = time.Now()
			}
			m.mu.Unlock()
		}
		C.pa_simple_free(s)
		time.Sleep(300 * time.Millisecond)
	}
}
