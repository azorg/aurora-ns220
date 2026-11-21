#!/usr/bin/env python3
"""Двухпальцевая прокрутка для Aurora (GLFW v3.4 в go-gl/glfw/v3.4).

Применять К уже пропатченному под Aurora модулю (wl_shell + wl_touch→mouse):
    python3 patch_glfw_scroll.py /home/opt/aurora-fyne/go-glfw34

Что делает: заменяет обработчики wl_touch на версию v2 —
  * один палец  — как раньше: курсор + левая кнопка (тап/свайп/drag);
  * два пальца  — «клик» отменяется, вертикальное движение средней точки
    превращается в события прокрутки (glfw scroll), накапливаемые до Frame
    (одно событие на кадр — меньше нагрузки на UI-поток Fyne);
  * после отпускания любого из двух пальцев прокрутка завершается, одиночное
    касание снова работает как мышь (лишних кликов нет).

Идемпотентно: повторный запуск ничего не меняет (маркер AURORA_TOUCH_SCROLL_V2).
"""
import sys, os, re

MOD = sys.argv[1] if len(sys.argv) > 1 else '/home/opt/aurora-fyne/go-glfw34'
path = os.path.join(MOD, 'glfw/src/wl_window.c')
src = open(path).read()

if 'AURORA_TOUCH_SCROLL_V2' in src:
    print('уже пропатчено (AURORA_TOUCH_SCROLL_V2)')
    sys.exit(0)

start_marker = 'static void touchHandleDown('
end_marker = 'static const struct wl_touch_listener touchListener ='
i = src.find(start_marker)
j = src.find(end_marker)
if i < 0 or j < 0 or j <= i:
    sys.exit('не найдены обработчики wl_touch (patch_glfw34.py применён?)')

NEW = r'''// ==== AURORA_TOUCH_SCROLL_V2: 1 палец -> мышь, 2 пальца -> прокрутка ====
#include <math.h>

static int32_t auroraTouchIds[2] = { -1, -1 };
static double  auroraTouchX[2], auroraTouchY[2];
static int     auroraTouchCount = 0;
static int     auroraScrollMode = 0;
static _GLFWwindow* auroraTouchWindow = NULL;
static double  auroraScrollLastX = 0.0, auroraScrollLastY = 0.0;
static double  auroraScrollAccum = 0.0;

static int auroraTouchSlot(int32_t id)
{
    int i;
    for (i = 0; i < 2; i++)
        if (auroraTouchIds[i] == id)
            return i;
    return -1;
}

static void auroraTouchAvg(double* x, double* y)
{
    int i, n = 0;
    double sx = 0.0, sy = 0.0;
    for (i = 0; i < 2; i++)
    {
        if (auroraTouchIds[i] < 0)
            continue;
        sx += auroraTouchX[i];
        sy += auroraTouchY[i];
        n++;
    }
    if (n > 0)
    {
        *x = sx / n;
        *y = sy / n;
    }
}

static void touchHandleDown(void* userData, struct wl_touch* touch, uint32_t serial,
                            uint32_t time, struct wl_surface* surface, int32_t id,
                            wl_fixed_t x, wl_fixed_t y)
{
    _GLFWwindow* window = surface ? wl_surface_get_user_data(surface) : NULL;
    if (!window)
        return;

    if (auroraTouchCount == 0)
    {
        auroraTouchWindow = window;
        _glfw.wl.touchFocus = window;
        _glfw.wl.touchId = id;
        auroraTouchIds[0] = id;
        auroraTouchX[0] = wl_fixed_to_double(x);
        auroraTouchY[0] = wl_fixed_to_double(y);
        auroraTouchCount = 1;
        auroraScrollMode = 0;

        window->wl.cursorPosX = auroraTouchX[0];
        window->wl.cursorPosY = auroraTouchY[0];
        _glfwInputCursorPos(window, window->wl.cursorPosX, window->wl.cursorPosY);
        _glfwInputMouseClick(window, GLFW_MOUSE_BUTTON_LEFT, GLFW_PRESS, 0);
        return;
    }

    if (auroraTouchCount == 1)
    {
        // второй палец: отменяем «нажатие» первого и переходим к прокрутке
        if (auroraTouchWindow)
            _glfwInputMouseClick(auroraTouchWindow, GLFW_MOUSE_BUTTON_LEFT, GLFW_RELEASE, 0);

        auroraTouchIds[1] = id;
        auroraTouchX[1] = wl_fixed_to_double(x);
        auroraTouchY[1] = wl_fixed_to_double(y);
        auroraTouchCount = 2;
        auroraScrollMode = 1;
        auroraScrollAccum = 0.0;
        auroraTouchAvg(&auroraScrollLastX, &auroraScrollLastY);
    }
}

static void touchHandleUp(void* userData, struct wl_touch* touch, uint32_t serial,
                          uint32_t time, int32_t id)
{
    int slot = auroraTouchSlot(id);
    if (slot < 0)
        return;

    if (auroraScrollMode)
    {
        auroraTouchIds[slot] = -1;
        auroraTouchCount--;
        if (auroraTouchCount <= 0)
        {
            auroraTouchCount = 0;
            auroraScrollMode = 0;
            auroraScrollAccum = 0.0;
            auroraTouchWindow = NULL;
            _glfw.wl.touchFocus = NULL;
        }
        else
        {
            auroraTouchAvg(&auroraScrollLastX, &auroraScrollLastY);
        }
        return;
    }

    if (_glfw.wl.touchFocus && id == _glfw.wl.touchId)
    {
        _glfwInputMouseClick(_glfw.wl.touchFocus, GLFW_MOUSE_BUTTON_LEFT, GLFW_RELEASE, 0);
        _glfw.wl.touchFocus = NULL;
    }
    auroraTouchIds[slot] = -1;
    if (auroraTouchCount > 0)
        auroraTouchCount--;
}

static void touchHandleMotion(void* userData, struct wl_touch* touch, uint32_t time,
                              int32_t id, wl_fixed_t x, wl_fixed_t y)
{
    int slot = auroraTouchSlot(id);
    if (slot < 0)
        return;

    auroraTouchX[slot] = wl_fixed_to_double(x);
    auroraTouchY[slot] = wl_fixed_to_double(y);

    if (auroraScrollMode && auroraTouchCount == 2)
    {
        double ax, ay, dx, dy;
        auroraTouchAvg(&ax, &ay);
        dx = ax - auroraScrollLastX;
        dy = ay - auroraScrollLastY;
        auroraScrollLastX = ax;
        auroraScrollLastY = ay;
        if (fabs(dy) > fabs(dx))
            auroraScrollAccum += dy;   // вертикальное движение двух пальцев
        return;
    }

    if (_glfw.wl.touchFocus && id == _glfw.wl.touchId)
    {
        _glfw.wl.touchFocus->wl.cursorPosX = auroraTouchX[slot];
        _glfw.wl.touchFocus->wl.cursorPosY = auroraTouchY[slot];
        _glfwInputCursorPos(_glfw.wl.touchFocus, auroraTouchX[slot], auroraTouchY[slot]);
    }
}

static void touchHandleFrame(void* userData, struct wl_touch* touch)
{
    // одно событие прокрутки на кадр (Fyne обрабатывает в UI-потоке)
    if (auroraScrollMode && auroraTouchWindow && fabs(auroraScrollAccum) >= 1.0)
    {
        _glfwInputScroll(auroraTouchWindow, 0.0, -auroraScrollAccum * 0.6);
        auroraScrollAccum = 0.0;
    }
}

static void touchHandleCancel(void* userData, struct wl_touch* touch)
{
    if (auroraTouchWindow && !auroraScrollMode)
        _glfwInputMouseClick(auroraTouchWindow, GLFW_MOUSE_BUTTON_LEFT, GLFW_RELEASE, 0);
    auroraTouchIds[0] = auroraTouchIds[1] = -1;
    auroraTouchCount = 0;
    auroraScrollMode = 0;
    auroraScrollAccum = 0.0;
    auroraTouchWindow = NULL;
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

'''

src = src[:i] + NEW + src[j:]
open(path, 'w').write(src)
print('ok: двухпальцевая прокрутка добавлена в', path)
