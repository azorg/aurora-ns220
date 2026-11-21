set +e
V=/home/user/fyne-demo/vendor/github.com/go-gl/glfw/v3.3/glfw
grep -q "wl-shell-protocol.c" $V/c_glfw_lin.go || sed -i 's|#include "glfw/src/wayland-xdg-shell-client-protocol.c"|#include "glfw/src/wayland-xdg-shell-client-protocol.c"\n\t#include "glfw/src/wl-shell-protocol.c"|' $V/c_glfw_lin.go
grep -n "wl-shell-protocol.c" $V/c_glfw_lin.go
export PATH=/home/opt/go/bin:$PATH GOMODCACHE=/home/opt/gopath/pkg/mod GOCACHE=/home/opt/gocache GOPATH=/home/opt/gopath
export PKG_CONFIG_PATH=/home/opt/pc:/home/opt/glfw/lib/pkgconfig:/home/opt/usr/lib/pkgconfig:/usr/lib/pkgconfig
export CGO_CFLAGS="-D_GNU_SOURCE -DEGL_NO_X11 -I/home/opt/usr/include -I/home/opt/glfw/include"
export CGO_LDFLAGS="-L/home/opt/glfw/lib -L/home/opt/libs -lwayland-client -lwayland-cursor -lwayland-egl -lxkbcommon -lEGL -lGLESv2 -lpthread -lm -ldl"
cd /home/user/fyne-demo
go build -mod=vendor -tags "wayland gles egl" -o demo-wl . 2>&1 | tail -15
ls -lh demo-wl 2>/dev/null
