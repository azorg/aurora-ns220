package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
)

// lg — журнал в stderr (при запуске через run-fyne-gpu.sh попадает в лог).
var lg = log.New(os.Stderr, "", log.Ltime|log.Lmicroseconds)

// envInfo — строки HUD: реальное окружение, в котором работает приложение.
// Служит доказательством «мы действительно на устройстве, а не в эмуляторе».
func envInfo() []string {
	var out []string
	out = append(out, "ОС: "+osPretty())
	out = append(out, "Ядро: "+kv(kernelRelease()))
	out = append(out, fmt.Sprintf("Архитектура: %s · CPU: %d · Go %s",
		runtime.GOARCH, runtime.NumCPU(), strings.TrimPrefix(runtime.Version(), "go")))
	out = append(out, "GPU: "+gpuInUse())
	out = append(out, "IMA: "+imaState())
	out = append(out, "Fyne: "+modVersion("fyne.io/fyne/v2")+" · GLFW: "+modVersion("github.com/go-gl/glfw/v3.4/glfw"))
	return out
}

func osPretty() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "неизвестно"
	}
	name, ver := "", ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			return strings.Trim(v, `"`)
		}
		if v, ok := strings.CutPrefix(line, "NAME="); ok {
			name = strings.Trim(v, `"`)
		}
		if v, ok := strings.CutPrefix(line, "VERSION="); ok {
			ver = strings.Trim(v, `"`)
		}
	}
	if name == "" {
		return "неизвестно"
	}
	return strings.TrimSpace(name + " " + ver)
}

func kernelRelease() string {
	if data, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		return strings.TrimSpace(string(data))
	}
	return "неизвестно"
}

func kv(s string) string { return s }

// gpuInUse определяет, какой GL-стек реально загружен в процесс:
// libhybris/Mali (аппаратный GPU) или Mesa llvmpipe (программный).
func gpuInUse() string {
	data, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		return "неизвестно"
	}
	s := string(data)
	switch {
	case strings.Contains(s, "libGLES_mali"):
		return "Mali (аппаратный, libGLES_mali через libhybris)"
	case strings.Contains(s, "swrast_dri"), strings.Contains(s, "llvmpipe"):
		return "llvmpipe (Mesa, программный)"
	case strings.Contains(s, "libEGL"):
		return "EGL (драйвер не определён)"
	}
	return "не загружен"
}

// imaState показывает, включена ли проверка подписи исполняемых файлов.
func imaState() string {
	data, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return "неизвестно"
	}
	ima, sig := "?", "?"
	for _, f := range strings.Fields(string(data)) {
		if v, ok := strings.CutPrefix(f, "ima_appraise="); ok {
			ima = v
		}
		if v, ok := strings.CutPrefix(f, "module.sig_enforce="); ok {
			sig = v
		}
	}
	if ima == "off" || sig == "0" {
		return fmt.Sprintf("ima_appraise=%s, sig_enforce=%s → разработка разрешена", ima, sig)
	}
	return fmt.Sprintf("ima_appraise=%s, sig_enforce=%s → только подписанный код", ima, sig)
}

func modVersion(path string) string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "?"
	}
	for _, d := range bi.Deps {
		if d.Path == path {
			v := d.Version
			if d.Replace != nil {
				v = "replace→" + d.Replace.Path
			}
			return v
		}
	}
	return "?"
}
