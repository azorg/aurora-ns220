#include <stdio.h>
#include <string.h>
#include <wayland-client.h>
static void global(void *d, struct wl_registry *r, uint32_t name, const char *iface, uint32_t ver){
    (void)d;(void)r;(void)name;
    printf("  %-45s v%u\n", iface, ver);
}
static const struct wl_registry_listener li = { global, NULL };
int main(void){
    struct wl_display *dpy = wl_display_connect(NULL);
    if(!dpy){ fprintf(stderr,"cannot connect\n"); return 1; }
    struct wl_registry *reg = wl_display_get_registry(dpy);
    wl_registry_add_listener(reg, &li, NULL);
    wl_display_roundtrip(dpy);
    wl_display_disconnect(dpy);
    return 0;
}
