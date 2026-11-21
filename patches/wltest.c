#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/mman.h>
#include <wayland-client.h>

static struct wl_compositor *comp; static struct wl_shm *shm; static struct wl_shell *shell;
static struct wl_shm_pool *pool; static struct wl_buffer *buf; static void *data;

static void global(void *d, struct wl_registry *r, uint32_t name, const char *iface, uint32_t ver){
    if(!strcmp(iface,"wl_compositor")) comp = wl_registry_bind(r,name,&wl_compositor_interface,1);
    else if(!strcmp(iface,"wl_shm")) shm = wl_registry_bind(r,name,&wl_shm_interface,1);
    else if(!strcmp(iface,"wl_shell")) shell = wl_registry_bind(r,name,&wl_shell_interface,1);
}
static const struct wl_registry_listener rl = { global, NULL };
static void ping(void *d, struct wl_shell_surface *s, uint32_t serial){ wl_shell_surface_pong(s, serial); }
static void conf(void *d, struct wl_shell_surface *s, uint32_t edges, int32_t w, int32_t h){ printf("configure %dx%d edges=%u\n", w,h,edges); }
static const struct wl_shell_surface_listener sl = { ping, conf, NULL };

int main(int argc, char **argv){
    int W=640,H=400;
    struct wl_display *dpy = wl_display_connect(NULL);
    if(!dpy){ fprintf(stderr,"no display\n"); return 1; }
    struct wl_registry *reg = wl_display_get_registry(dpy);
    wl_registry_add_listener(reg,&rl,NULL);
    wl_display_roundtrip(dpy);
    if(!comp||!shm||!shell){ fprintf(stderr,"missing globals comp=%p shm=%p shell=%p\n",comp,shm,shell); return 1; }
    int stride=W*4, size=stride*H;
    char tmpl[]="/tmp/wlshm-XXXXXX";
    int fd=mkstemp(tmpl); unlink(tmpl); ftruncate(fd,size);
    data=mmap(NULL,size,PROT_READ|PROT_WRITE,MAP_SHARED,fd,0);
    for(int i=0;i<W*H;i++){ ((uint32_t*)data)[i]=0xFFFF0000; }
    pool=wl_shm_create_pool(shm,fd,size);
    buf=wl_shm_pool_create_buffer(pool,0,W,H,stride,WL_SHM_FORMAT_XRGB8888);
    struct wl_surface *surf=wl_compositor_create_surface(comp);
    struct wl_shell_surface *ss=wl_shell_get_shell_surface(shell,surf);
    wl_shell_surface_add_listener(ss,&sl,NULL);
    if(argc>1) wl_shell_surface_set_class(ss, argv[1]);
    wl_shell_surface_set_title(ss,"WL Test Window");
    wl_shell_surface_set_toplevel(ss);
    wl_surface_attach(surf,buf,0,0);
    wl_surface_damage(surf,0,0,W,H);
    wl_surface_commit(surf);
    wl_display_roundtrip(dpy);
    printf("window created, waiting 20s\n"); fflush(stdout);
    for(int i=0;i<200;i++){ wl_display_dispatch_pending(dpy); usleep(100000); }
    return 0;
}
