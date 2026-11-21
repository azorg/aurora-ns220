#!/usr/bin/env python3
"""Патч GLFW v3.4 (go-gl/glfw/v3.4) для Aurora: wl_shell + wl_touch.
usage: patch_glfw34.py /home/opt/aurora-fyne/go-glfw34
"""
import sys, os, re

MOD = sys.argv[1] if len(sys.argv) > 1 else '/home/opt/aurora-fyne/go-glfw34'
S = os.path.join(MOD, 'glfw/src')

# ---------------- wl_platform.h ----------------
p = os.path.join(S, 'wl_platform.h')
h = open(p).read()
if 'struct wl_shell_surface*' not in h:
    # окно: добавить рядом с surface
    h = re.sub(r'(typedef struct _GLFWwindowWayland\s*\{[^}]*?struct wl_surface\*\s+surface;)',
               r'\1\n    struct wl_shell_surface*    shellSurface;', h, count=1, flags=re.S)
    # платформа: рядом с compositor
    h = re.sub(r'(struct wl_compositor\*\s+compositor;)',
               r'\1\n    struct wl_shell*            shell;\n'
               r'    struct wl_touch*            touch;\n'
               r'    _GLFWwindow*                touchFocus;\n'
               r'    int32_t                     touchId;', h, count=1)
    open(p, 'w').write(h)
    print('wl_platform.h: ok')
else:
    print('wl_platform.h: уже пропатчен')

# ---------------- wl_init.c ----------------
p = os.path.join(S, 'wl_init.c')
c = open(p).read()
if 'wl_shell_interface' not in c:
    anchor = 'else if (strcmp(interface, "zxdg_decoration_manager_v1") == 0)'
    assert anchor in c, 'не нашёл ветку zxdg в registryHandleGlobal'
    c = c.replace(anchor,
        'else if (strcmp(interface, "wl_shell") == 0)\n'
        '    {\n'
        '        _glfw.wl.shell = wl_registry_bind(registry, name, &wl_shell_interface, 1);\n'
        '    }\n'
        '    ' + anchor, 1)
    c = c.replace('if (!_glfw.wl.wmBase)', 'if (!_glfw.wl.wmBase && !_glfw.wl.shell)', 1)
    open(p, 'w').write(c)
    print('wl_init.c: ok')
else:
    print('wl_init.c: уже пропатчен')

# ---------------- wl_window.c ----------------
p = os.path.join(S, 'wl_window.c')
c = open(p).read()

HELPERS = r'''
// ==== Aurora patch: wl_shell (нет xdg-shell в lipstick) и wl_touch -> mouse ====
static void wlShellSurfaceHandlePing(void* userData,
                                     struct wl_shell_surface* shellSurface,
                                     uint32_t serial)
{
    wl_shell_surface_pong(shellSurface, serial);
}

static void wlShellSurfaceHandleConfigure(void* userData,
                                          struct wl_shell_surface* shellSurface,
                                          uint32_t edges,
                                          int32_t width,
                                          int32_t height)
{
    _GLFWwindow* window = (_GLFWwindow*) userData;

    if (width > 0 && height > 0)
    {
        window->wl.width = width;
        window->wl.height = height;
        if (window->wl.egl.window)
            wl_egl_window_resize(window->wl.egl.window, width, height, 0, 0);
        _glfwInputFramebufferSize(window, width, height);
        _glfwInputWindowSize(window, width, height);
    }

    if (!window->wl.visible)
    {
        window->wl.visible = GLFW_TRUE;
        _glfwInputWindowDamage(window);
    }
}

static const struct wl_shell_surface_listener wlShellSurfaceListener =
{
    wlShellSurfaceHandlePing,
    wlShellSurfaceHandleConfigure,
    NULL
};

static GLFWbool createWlShellObjects(_GLFWwindow* window)
{
    if (window->wl.shellSurface)
        return GLFW_TRUE;

    window->wl.shellSurface = wl_shell_get_shell_surface(_glfw.wl.shell, window->wl.surface);
    if (!window->wl.shellSurface)
    {
        _glfwInputError(GLFW_PLATFORM_ERROR, "Wayland: Failed to create wl_shell_surface");
        return GLFW_FALSE;
    }

    wl_shell_surface_add_listener(window->wl.shellSurface, &wlShellSurfaceListener, window);

    if (window->wl.title)
        wl_shell_surface_set_title(window->wl.shellSurface, window->wl.title);

    wl_shell_surface_set_toplevel(window->wl.shellSurface);
    wl_surface_commit(window->wl.surface);
    wl_display_roundtrip(_glfw.wl.display);
    return GLFW_TRUE;
}

static void touchHandleDown(void* userData, struct wl_touch* touch, uint32_t serial,
                            uint32_t time, struct wl_surface* surface, int32_t id,
                            wl_fixed_t x, wl_fixed_t y)
{
    _GLFWwindow* window = surface ? wl_surface_get_user_data(surface) : NULL;
    if (!window)
        return;

    window->wl.cursorPosX = wl_fixed_to_double(x);
    window->wl.cursorPosY = wl_fixed_to_double(y);
    _glfw.wl.touchFocus = window;
    _glfw.wl.touchId = id;

    _glfwInputCursorPos(window, window->wl.cursorPosX, window->wl.cursorPosY);
    _glfwInputMouseClick(window, GLFW_MOUSE_BUTTON_LEFT, GLFW_PRESS, 0);
}

static void touchHandleUp(void* userData, struct wl_touch* touch, uint32_t serial,
                          uint32_t time, int32_t id)
{
    _GLFWwindow* window = _glfw.wl.touchFocus;
    if (!window || id != _glfw.wl.touchId)
        return;
    _glfwInputMouseClick(window, GLFW_MOUSE_BUTTON_LEFT, GLFW_RELEASE, 0);
    _glfw.wl.touchFocus = NULL;
}

static void touchHandleMotion(void* userData, struct wl_touch* touch, uint32_t time,
                              int32_t id, wl_fixed_t x, wl_fixed_t y)
{
    _GLFWwindow* window = _glfw.wl.touchFocus;
    if (!window || id != _glfw.wl.touchId)
        return;
    window->wl.cursorPosX = wl_fixed_to_double(x);
    window->wl.cursorPosY = wl_fixed_to_double(y);
    _glfwInputCursorPos(window, window->wl.cursorPosX, window->wl.cursorPosY);
}

static void touchHandleFrame(void* userData, struct wl_touch* touch) {}
static void touchHandleCancel(void* userData, struct wl_touch* touch)
{
    _GLFWwindow* window = _glfw.wl.touchFocus;
    if (!window)
        return;
    _glfwInputMouseClick(window, GLFW_MOUSE_BUTTON_LEFT, GLFW_RELEASE, 0);
    _glfw.wl.touchFocus = NULL;
}
#if defined(WL_TOUCH_SHAPE_SINCE_VERSION)
static void touchHandleShape(void* userData, struct wl_touch* touch, int32_t id,
                             wl_fixed_t major, wl_fixed_t minor) {}
#endif
#if defined(WL_TOUCH_ORIENTATION_SINCE_VERSION)
static void touchHandleOrientation(void* userData, struct wl_touch* touch, int32_t id,
                                   wl_fixed_t orientation) {}
#endif

static const struct wl_touch_listener touchListener =
{
    touchHandleDown,
    touchHandleUp,
    touchHandleMotion,
    touchHandleFrame,
    touchHandleCancel,
#if defined(WL_TOUCH_SHAPE_SINCE_VERSION)
    touchHandleShape,
#else
    NULL,
#endif
#if defined(WL_TOUCH_ORIENTATION_SINCE_VERSION)
    touchHandleOrientation
#else
    NULL
#endif
};
'''

if 'createWlShellObjects' not in c:
    anchor = 'static GLFWbool createShellObjects(_GLFWwindow* window)'
    assert anchor in c
    c = c.replace(anchor, HELPERS + '\n' + anchor, 1)

    # fallback + guard
    old = '''static GLFWbool createShellObjects(_GLFWwindow* window)
{
    if (_glfw.wl.libdecor.context)'''
    new = '''static GLFWbool createShellObjects(_GLFWwindow* window)
{
    if (window->wl.shellSurface || window->wl.xdg.surface)
        return GLFW_TRUE;

    if (!_glfw.wl.wmBase)
    {
        if (_glfw.wl.shell)
            return createWlShellObjects(window);

        _glfwInputError(GLFW_PLATFORM_ERROR,
                        "Wayland: Failed to find xdg-shell or wl_shell in your compositor");
        return GLFW_FALSE;
    }

    if (_glfw.wl.libdecor.context)'''
    assert old in c
    c = c.replace(old, new, 1)

    # destroy shell surface
    old2 = '''static void destroyShellObjects(_GLFWwindow* window)
{
    destroyFallbackDecorations(window);'''
    new2 = '''static void destroyShellObjects(_GLFWwindow* window)
{
    if (window->wl.shellSurface)
    {
        wl_shell_surface_destroy(window->wl.shellSurface);
        window->wl.shellSurface = NULL;
    }

    destroyFallbackDecorations(window);'''
    assert old2 in c
    c = c.replace(old2, new2, 1)

    # touch в seat capabilities
    old3 = '''    if ((caps & WL_SEAT_CAPABILITY_KEYBOARD) && !_glfw.wl.keyboard)'''
    new3 = '''    if ((caps & WL_SEAT_CAPABILITY_TOUCH) && !_glfw.wl.touch)
    {
        _glfw.wl.touch = wl_seat_get_touch(seat);
        wl_touch_add_listener(_glfw.wl.touch, &touchListener, NULL);
    }
    else if (!(caps & WL_SEAT_CAPABILITY_TOUCH) && _glfw.wl.touch)
    {
        wl_touch_destroy(_glfw.wl.touch);
        _glfw.wl.touch = NULL;
    }

    if ((caps & WL_SEAT_CAPABILITY_KEYBOARD) && !_glfw.wl.keyboard)'''
    assert old3 in c
    c = c.replace(old3, new3, 1)

    # заголовок окна: ветка wl_shell
    old4 = '    xdg_toplevel_set_title(window->wl.xdg.toplevel, title);'
    if old4 in c:
        c = c.replace(old4,
            '    if (window->wl.xdg.toplevel)\n'
            '        xdg_toplevel_set_title(window->wl.xdg.toplevel, title);\n'
            '    else if (window->wl.shellSurface)\n'
            '        wl_shell_surface_set_title(window->wl.shellSurface, title);', 1)

    # показ окна: для wl_shell выставить visible и commit
    m = re.search(r'void _glfwShowWindowWayland\(_GLFWwindow\* window\)\n\{\n', c)
    if m:
        ins = ('    if (!window->wl.xdg.toplevel && !window->wl.shellSurface)\n'
               '        createShellObjects(window);\n'
               '    if (window->wl.shellSurface)\n'
               '    {\n'
               '        window->wl.visible = GLFW_TRUE;\n'
               '        wl_surface_commit(window->wl.surface);\n'
               '        _glfwInputWindowDamage(window);\n'
               '    }\n\n')
        c = c[:m.end()] + ins + c[m.end():]

    # все xdg_toplevel_* — под проверкой на NULL
    def guard(mm):
        ind, call = mm.group(1), mm.group(2)
        return ind + 'if (window->wl.xdg.toplevel)\n' + ind + '    ' + call + '(window->wl.xdg.toplevel'
    c = re.sub(r'(?m)^(\s+)(xdg_toplevel_[a-z_]+)\(window->wl\.xdg\.toplevel', guard, c)

    open(p, 'w').write(c)
    print('wl_window.c: ok')
else:
    print('wl_window.c: уже пропатчен')
