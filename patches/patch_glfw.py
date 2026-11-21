#!/usr/bin/env python3
"""Patch vendored GLFW (go-gl/glfw) to fall back to wl_shell when xdg-shell is absent."""
import re, sys, os

D = '/home/user/fyne-demo/vendor/github.com/go-gl/glfw/v3.3/glfw/glfw/src'

# ---------- wl_platform.h ----------
p = os.path.join(D, 'wl_platform.h')
s = open(p).read()
if 'wl-shell-client-protocol.h' not in s:
    s = s.replace('#include "wayland-xdg-shell-client-protocol.h"',
                  '#include "wayland-xdg-shell-client-protocol.h"\n#include "wl-shell-client-protocol.h"', 1)
if 'struct wl_shell*' not in s:
    # add platform global next to compositor
    s = s.replace('    struct wl_compositor*       compositor;',
                  '    struct wl_compositor*       compositor;\n    struct wl_shell*            shell;', 1)
if 'shellSurface' not in s:
    s = s.replace('    struct wl_surface*          surface;',
                  '    struct wl_surface*          surface;\n    struct wl_shell_surface*    shellSurface;', 1)
open(p, 'w').write(s)
print('patched wl_platform.h')

# ---------- wl_init.c ----------
p = os.path.join(D, 'wl_init.c')
s = open(p).read()
if 'wl_shell_interface' not in s:
    # bind wl_shell in registry handler: insert right after xdg_wm_base block
    m = re.search(r'(        _glfw\.wl\.wmBase =.*?\n.*?\n.*?\n.*?\n)', s, re.S)
    if not m:
        print('! could not find wmBase bind block')
        sys.exit(1)
    bind = m.group(1)
    add = (
        '\n    else if (strcmp(interface, "wl_shell") == 0)\n'
        '    {\n'
        '        _glfw.wl.shell = wl_registry_bind(registry, name, &wl_shell_interface, 1);\n'
        '    }\n'
    )
    # insert after the closing brace of the wmBase branch: find the end of that if-block
    idx = s.index(bind)
    # locate the closing "    }\n" of the if-branch after bind
    close = s.index('\n    }\n', idx)
    s = s[:close] + '\n' + add + s[close + len('\n    }\n'):]
    if 'if (strcmp(interface, "wl_shell")' not in s:
        print('! wl_shell bind insertion failed')
        sys.exit(1)

s = s.replace('''    if (!_glfw.wl.wmBase)
    {''', '''    if (!_glfw.wl.wmBase && !_glfw.wl.shell)
    {''', 1)
open(p, 'w').write(s)
print('patched wl_init.c')

# ---------- wl_window.c ----------
p = os.path.join(D, 'wl_window.c')
s = open(p).read()

SHELL_CODE = r'''
// ---- Aurora patch: wl_shell fallback (no xdg-shell in lipstick) ----
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
}

static const struct wl_shell_surface_listener wlShellSurfaceListener =
{
    wlShellSurfaceHandlePing,
    wlShellSurfaceHandleConfigure,
    NULL
};

static GLFWbool createWlShellObjects(_GLFWwindow* window)
{
    window->wl.shellSurface = wl_shell_get_shell_surface(_glfw.wl.shell, window->wl.surface);
    if (!window->wl.shellSurface)
    {
        _glfwInputError(GLFW_PLATFORM_ERROR,
                        "Wayland: Failed to create wl_shell_surface");
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

'''

if 'createWlShellObjects' not in s:
    s = s.replace('static GLFWbool createShellObjects(_GLFWwindow* window)\n{',
                  SHELL_CODE + 'static GLFWbool createShellObjects(_GLFWwindow* window)\n{', 1)
    # fallback at the top of createShellObjects
    s = s.replace('''static GLFWbool createShellObjects(_GLFWwindow* window)
{
''', '''static GLFWbool createShellObjects(_GLFWwindow* window)
{
    if (!_glfw.wl.wmBase)
    {
        if (_glfw.wl.shell)
            return createWlShellObjects(window);

        _glfwInputError(GLFW_PLATFORM_ERROR,
                        "Wayland: Failed to find xdg-shell or wl_shell in your compositor");
        return GLFW_FALSE;
    }

''', 1)

# destroy shell surface
s = s.replace('''static void destroyShellObjects(_GLFWwindow* window)
{
''', '''static void destroyShellObjects(_GLFWwindow* window)
{
    if (window->wl.shellSurface)
    {
        wl_shell_surface_destroy(window->wl.shellSurface);
        window->wl.shellSurface = NULL;
    }

''', 1)

# title: support wl_shell
s = s.replace('''        xdg_toplevel_set_title(window->wl.xdg.toplevel, title);''',
'''        if (window->wl.xdg.toplevel)
            xdg_toplevel_set_title(window->wl.xdg.toplevel, title);
        else if (window->wl.shellSurface)
            wl_shell_surface_set_title(window->wl.shellSurface, title);''')

# guard every remaining xdg_toplevel_* statement on the toplevel object
def guard(m):
    indent, call = m.group(1), m.group(2)
    head = indent + 'if (window->wl.xdg.toplevel)\n' + indent + '    '
    return head + call + '(window->wl.xdg.toplevel'

s = re.sub(r'(?m)^(\s+)(xdg_toplevel_[a-z_]+)\(window->wl\.xdg\.toplevel', guard, s)
# guard decoration mode calls
s = s.replace('zxdg_toplevel_decoration_v1_set_mode(window->wl.xdg.decoration, mode);',
              'if (window->wl.xdg.decoration)\n                zxdg_toplevel_decoration_v1_set_mode(window->wl.xdg.decoration, mode);')
open(p, 'w').write(s)
print('patched wl_window.c')
print('remaining unguarded xdg_toplevel calls:', len(re.findall(r'(?m)^\s+xdg_toplevel_\w+\(window->wl\.xdg\.toplevel', s)))
