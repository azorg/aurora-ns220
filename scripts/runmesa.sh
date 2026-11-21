set +e
cd /home/user && rm -rf /home/opt/mesa && mkdir -p /home/opt/mesa && tar -xzf mesa.tgz -C /home/opt/mesa
ln -sf libEGL.so.1.0.0 /home/opt/mesa/usr/lib/libEGL.so.1
ln -sf libGLESv2.so.2.0.0 /home/opt/mesa/usr/lib/libGLESv2.so.2
ln -sf libglapi.so.0.0.0 /home/opt/mesa/usr/lib/libglapi.so.0
ln -sf libgbm.so.1.0.0 /home/opt/mesa/usr/lib/libgbm.so.1
ln -sf libdrm.so.2.4.0 /home/opt/mesa/usr/lib/libdrm.so.2
ls /home/opt/mesa/usr/lib/*.so.* | head -5
cd /home/user/fyne-demo
pkill -f 'demo-wl' 2>/dev/null; sleep 1
nohup env XDG_RUNTIME_DIR=/run/user/100000 WAYLAND_DISPLAY=/run/display/wayland-0 \
  LD_LIBRARY_PATH=/home/opt/mesa/usr/lib \
  LIBGL_DRIVERS_PATH=/home/opt/mesa/usr/lib/dri \
  MESA_LOADER_DRIVER_OVERRIDE=swrast LIBGL_ALWAYS_SOFTWARE=1 EGL_PLATFORM=wayland \
  GALLIUM_DRIVER=llvmpipe \
  ./demo-wl > /tmp/fyne.out 2>&1 &
sleep 12
echo "pid=$(pgrep -f './demo-wl' | head -1)"
echo "--- output:"; head -15 /tmp/fyne.out
pgrep -f './demo-wl' >/dev/null && echo "ALIVE — окно Fyne должно быть на экране" || echo "EXITED"
