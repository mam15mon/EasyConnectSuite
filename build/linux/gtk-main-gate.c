#define _GNU_SOURCE
#include <dlfcn.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

// Test-only interposer holds Wails immediately before entering gtk_main.
void gtk_main(void) {
    void (*real_main)(void) = dlsym(RTLD_NEXT, "gtk_main");
    if (!real_main) _exit(90);
    const char *gate = getenv("ECS_GTK_TEST_GATE");
    if (!gate) _exit(91);
    dprintf(STDERR_FILENO, "GTK_TEST_MAIN_PAUSED\n");
    int fd = open(gate, O_RDONLY);
    char byte;
    if (fd < 0 || read(fd, &byte, 1) != 1) _exit(92);
    close(fd);
    real_main();
}
