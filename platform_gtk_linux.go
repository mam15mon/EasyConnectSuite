package main

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>

typedef struct {
    GMutex mutex;
    GCond cond;
    gboolean ready;
} LoopWait;

static gboolean loop_ready(gpointer data) {
    LoopWait *wait = data;
    if (gtk_main_level() == 0) return G_SOURCE_CONTINUE;
    g_mutex_lock(&wait->mutex);
    wait->ready = TRUE;
    g_cond_signal(&wait->cond);
    g_mutex_unlock(&wait->mutex);
    return G_SOURCE_REMOVE;
}

static void wait_for_gtk_loop(void) {
    LoopWait wait = {0};
    g_mutex_init(&wait.mutex);
    g_cond_init(&wait.cond);
    g_mutex_lock(&wait.mutex);
    g_idle_add(loop_ready, &wait);
    while (!wait.ready) g_cond_wait(&wait.cond, &wait.mutex);
    g_mutex_unlock(&wait.mutex);
    g_cond_clear(&wait.cond);
    g_mutex_clear(&wait.mutex);
}
*/
import "C"

import (
	"fmt"
	"os"

	vpnlog "EasyConnectSuite/vpncore/log"
)

// Wails can receive a shutdown signal before it enters gtk_main. Waiting for
// a callback in that loop ensures gtk_main_quit can actually stop the loop.
func waitForLinuxGTKMainLoop() {
	op := vpnlog.BeginOperation(fmt.Sprintf("[VPN pid=%d] ", os.Getpid()), "linux_close", nil)
	op.Step("wait_gtk_main_loop")
	C.wait_for_gtk_loop()
	op.Finish("ready", nil)
}
