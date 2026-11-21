package main

// Доступ к вибромотору планшета NS220 (MT6735).
//
// Ядро MTK отдаёт вибро через sysfs timed_output:
//   /sys/class/timed_output/vibrator/enable — запись «<мс>» включает мотор на это
//   время, запись «0» — выключает. Узел доступен на запись пользователю
//   (группа input): root не нужен.
//
// Проверено на устройстве 2026-11-21: запись 300 → мотор отрабатывает 300 мс,
// rc=0. Модули (`lsmod`) не нужны — драйвер встроен (mtk_vibrator).

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const vibratorEnable = "/sys/class/timed_output/vibrator/enable"

// Vibrate включает мотор на d и возвращает фактически записанное значение.
// Дополнительно через d+100 мс пишется 0 — страховка, если ядро не сбросит
// таймер само (на MTK сбрасывает, но лишним не будет).
func Vibrate(d time.Duration) (string, error) {
	ms := d.Milliseconds()
	if ms <= 0 {
		ms = 1
	}
	if ms > 10000 {
		ms = 10000
	}
	if err := writeSysfs(vibratorEnable, strconv.FormatInt(ms, 10)); err != nil {
		return "", err
	}
	go func() {
		time.Sleep(d + 100*time.Millisecond)
		_ = writeSysfs(vibratorEnable, "0")
	}()
	return fmt.Sprintf("enable=%d", ms), nil
}

// VibrateStop выключает мотор немедленно.
func VibrateStop() error { return writeSysfs(vibratorEnable, "0") }

// VibratorInfo — что известно о моторе (для HUD).
func VibratorInfo() string {
	paths := []string{
		"/sys/class/timed_output/vibrator/enable",
		"/sys/bus/platform/devices/mtk_vibrator",
		"/sys/devices/platform/mtk_vibrator",
	}
	var found []string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			found = append(found, p)
		}
	}
	if len(found) == 0 {
		return "вибромотор: узел timed_output не найден"
	}
	b, err := os.ReadFile(vibratorEnable)
	cur := "?"
	if err == nil {
		cur = strings.TrimSpace(string(b))
	}
	return fmt.Sprintf("вибромотор: %s (сейчас: %s)", strings.Join(found, ", "), cur)
}

func writeSysfs(path, val string) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(val)
	return err
}
