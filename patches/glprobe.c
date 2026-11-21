// glprobe.c — пробник EGL для Aurora (libhybris/Mali vs Mesa llvmpipe).
// Сборка: gcc -o glprobe glprobe.c -I/home/opt/usr/include -L/home/opt/libs \
//           -lEGL -lGLESv2 -lwayland-client -lwayland-egl -lm
// Запуск:  EGL_PLATFORM=wayland ./glprobe [wl|hwc] [ширина] [высота] [кадры]

// Пробник: какой EGL/GL реально доступен клиенту на Aurora (lipstick/wl_shell).
#include <stdio.h>
#include <string.h>
#include <stdlib.h>
#include <time.h>
#include <wayland-client.h>
#include <wayland-egl.h>
#include <EGL/egl.h>
#include <GLES2/gl2.h>

static struct wl_compositor *comp; static struct wl_shell *shell;
static struct wl_surface *surf; static struct wl_shell_surface *ssurf;
static void reg_global(void *d, struct wl_registry *r, uint32_t name, const char *iface, uint32_t ver) {
  if (!strcmp(iface, "wl_compositor")) comp = wl_registry_bind(r, name, &wl_compositor_interface, ver < 3 ? ver : 3);
  if (!strcmp(iface, "wl_shell")) shell = wl_registry_bind(r, name, &wl_shell_interface, 1);
}
static void reg_remove(void *d, struct wl_registry *r, uint32_t n) {}
static const struct wl_registry_listener rl = { reg_global, reg_remove };

int main(int argc, char **argv) {
  int hwc = (argc > 1 && !strcmp(argv[1], "hwc"));
  struct wl_display *dpy = NULL;
  if (!hwc) {
    dpy = wl_display_connect(NULL);
    if (!dpy) { printf("wl_display_connect FAILED\n"); return 1; }
    struct wl_registry *reg = wl_display_get_registry(dpy);
    wl_registry_add_listener(reg, &rl, NULL);
    wl_display_roundtrip(dpy);
    printf("globals: comp=%p shell=%p\n", (void*)comp, (void*)shell);
    if (!comp) return 2;
    surf = wl_compositor_create_surface(comp);
    if (shell) { ssurf = wl_shell_get_shell_surface(shell, surf); wl_shell_surface_set_toplevel(ssurf); }
    wl_surface_commit(surf);
  }
  EGLDisplay ed = eglGetDisplay(hwc ? (EGLNativeDisplayType)EGL_DEFAULT_DISPLAY : (EGLNativeDisplayType)dpy);
  if (ed == EGL_NO_DISPLAY) { printf("eglGetDisplay FAILED\n"); return 3; }
  EGLint maj, min;
  if (!eglInitialize(ed, &maj, &min)) { printf("eglInitialize FAILED err=0x%x\n", eglGetError()); return 4; }
  printf("EGL %d.%d\nVENDOR=%s\nVERSION=%s\nCLIENT_APIS=%s\n",
    maj, min, eglQueryString(ed, EGL_VENDOR), eglQueryString(ed, EGL_VERSION), eglQueryString(ed, EGL_CLIENT_APIS));
  printf("EXTENSIONS=%s\n", eglQueryString(ed, EGL_EXTENSIONS));
  EGLint cfga[] = { EGL_SURFACE_TYPE, EGL_WINDOW_BIT, EGL_RED_SIZE, 8, EGL_GREEN_SIZE, 8, EGL_BLUE_SIZE, 8,
                    EGL_ALPHA_SIZE, 8, EGL_RENDERABLE_TYPE, EGL_OPENGL_ES2_BIT, EGL_NONE };
  EGLConfig cfg; EGLint n = 0;
  if (!eglChooseConfig(ed, cfga, &cfg, 1, &n) || n < 1) { printf("eglChooseConfig FAILED n=%d err=0x%x\n", n, eglGetError()); return 5; }
  EGLNativeWindowType win = NULL;
  struct wl_egl_window *ew = NULL;
  if (!hwc) { ew = wl_egl_window_create(surf, argc > 2 ? atoi(argv[2]) : 400, argc > 3 ? atoi(argv[3]) : 400); win = (EGLNativeWindowType)ew; }
  EGLSurface es = eglCreateWindowSurface(ed, cfg, win, NULL);
  if (es == EGL_NO_SURFACE) { printf("eglCreateWindowSurface(win=%p) FAILED err=0x%x\n", (void*)win, eglGetError());
      es = eglCreateWindowSurface(ed, cfg, (EGLNativeWindowType)EGL_DEFAULT_DISPLAY, NULL);
      printf("  retry EGL_DEFAULT_DISPLAY -> %p err=0x%x\n", (void*)es, eglGetError());
      if (es == EGL_NO_SURFACE) return 6; }
  eglBindAPI(EGL_OPENGL_ES_API);
  EGLint ctxa[] = { EGL_CONTEXT_CLIENT_VERSION, 2, EGL_NONE };
  EGLContext ctx = eglCreateContext(ed, cfg, EGL_NO_CONTEXT, ctxa);
  if (ctx == EGL_NO_CONTEXT) { printf("eglCreateContext FAILED err=0x%x\n", eglGetError()); return 7; }
  if (!eglMakeCurrent(ed, es, es, ctx)) { printf("eglMakeCurrent FAILED err=0x%x\n", eglGetError()); return 8; }
  printf("GL_RENDERER=%s\nGL_VENDOR=%s\nGL_VERSION=%s\nGL_SL_VERSION=%s\n", glGetString(GL_RENDERER), glGetString(GL_VENDOR), glGetString(GL_VERSION), glGetString(GL_SHADING_LANGUAGE_VERSION));
  eglSwapInterval(ed, 0);
  int frames = argc > 2 ? atoi(argv[2]) : 120;
  struct timespec t0, t1; clock_gettime(CLOCK_MONOTONIC, &t0);
  for (int i = 0; i < frames; i++) {
    glClearColor((i%10)*0.1f, 0.5f, 0.2f, 1.0f);
    glClear(GL_COLOR_BUFFER_BIT);
    eglSwapBuffers(ed, es);
  }
  glFinish();
  clock_gettime(CLOCK_MONOTONIC, &t1);
  double dt = (t1.tv_sec-t0.tv_sec) + (t1.tv_nsec-t0.tv_nsec)/1e9;
  printf("frames=%d time=%.3fs fps=%.1f ms_per_frame=%.2f\n", frames, dt, frames/dt, 1000*dt/frames);
  printf("PROBE_OK\n");
  return 0;
}
