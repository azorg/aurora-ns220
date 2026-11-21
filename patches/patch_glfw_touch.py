#!/usr/bin/env python3
"""Патч GLFW: поддержка wl_touch (тач → события мыши).
Применяется к каталогу модуля go-gl/glfw (по умолчанию /home/opt/aurora-fyne/go-glfw).
"""
import sys, os

MOD = sys.argv[1] if len(sys.argv) > 1 else '/home/opt/aurora-fyne/go-glfw'
S = os.path.join(MOD, 'glfw/src')

# ---------- wl_platform.h: поля ----------
p = os.path.join(S, 'wl_platform.h')
h = open(p).read()
if 'struct wl_touch*' not in h:
    h = h.replace('''    struct wl_pointer*          pointer;''',
                  '''    struct wl_pointer*          pointer;
    struct wl_touch*            touch;
    _GLFWwindow*                touchFocus;
    int32_t                     touchId;''', 1)
    open(p, 'w').write(h)
    print('patched wl_platform.h')
else:
    print('wl_platform.h уже пропатчен')

# ---------- wl_window.c: обработчики + создание ----------
p = os.path.join(S, 'wl_window.c')
c = open(p).read()

TOUCH = r'''
// ---- Aurora patch: wl_touch -> mouse (в lipstick нет эмуляции мыши из тача) ----
static void touchHandleDown(void* userData,
                            struct wl_touch* touch,
                            uint32_t serial,
                            uint32_t time,
                            struct wl_surface* surface,
                            int32_t id,
                            wl_fixed_t x,
                            wl_fixed_t y)
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

static void touchHandleUp(void* userData,
                          struct wl_touch* touch,
                          uint32_t serial,
                          uint32_t time,
                          int32_t id)
{
    _GLFWwindow* window = _glfw.wl.touchFocus;
    if (!window || id != _glfw.wl.touchId)
        return;

    _glfwInputMouseClick(window, GLFW_MOUSE_BUTTON_LEFT, GLFW_RELEASE, 0);
    _glfw.wl.touchFocus = NULL;
}

static void touchHandleMotion(void* userData,
                              struct wl_touch* touch,
                              uint32_t time,
                              int32_t id,
                              wl_fixed_t x,
                              wl_fixed_t y)
{
    _GLFWwindow* window = _glfw.wl.touchFocus;
    if (!window || id != _glfw.wl.touchId)
        return;

    window->wl.cursorPosX = wl_fixed_to_double(x);
    window->wl.cursorPosY = wl_fixed_to_double(y);
    _glfwInputCursorPos(window, window->wl.cursorPosX, window->wl.cursorPosY);
}

static void touchHandleFrame(void* userData, struct wl_touch* touch)
{
}

static void touchHandleCancel(void* userData, struct wl_touch* touch)
{
    _GLFWwindow* window = _glfw.wl.touchFocus;
    if (!window)
        return;

    _glfwInputMouseClick(window, GLFW_MOUSE_BUTTON_LEFT, GLFW_RELEASE, 0);
    _glfw.wl.touchFocus = NULL;
}

static void touchHandleShape(void* userData, struct wl_touch* touch,
                             int32_t id, wl_fixed_t major, wl_fixed_t minor)
{
}

static void touchHandleOrientation(void* userData, struct wl_touch* touch,
                                   int32_t id, wl_fixed_t orientation)
{
}

static const struct wl_touch_listener touchListener =
{
    touchHandleDown,
    touchHandleUp,
    touchHandleMotion,
    touchHandleFrame,
    touchHandleCancel,
    touchHandleShape,
    touchHandleOrientation
};

'''

if 'touchHandleDown' not in c:
    anchor = 'static void seatHandleCapabilities(void* userData,'
    assert anchor in c, 'не найден seatHandleCapabilities'
    c = c.replace(anchor, TOUCH + anchor, 1)

    # создать/уничтожить wl_touch по capabilities
    old = '''    if ((caps & WL_SEAT_CAPABILITY_KEYBOARD) && !_glfw.wl.keyboard)'''
    new = '''    if ((caps & WL_SEAT_CAPABILITY_TOUCH) && !_glfw.wl.touch)
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
    assert old in c
    c = c.replace(old, new, 1)
    open(p, 'w').write(c)
    print('patched wl_window.c')
else:
    print('wl_window.c уже пропатчен')
