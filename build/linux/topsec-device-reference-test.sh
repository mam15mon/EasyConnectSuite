#!/usr/bin/env bash
set -euo pipefail

# Run only synthetic identities through the pinned native library. Nothing from
# the official package is installed, committed, or included in release outputs.
[[ $(uname -sm) == 'Linux x86_64' ]]
if [ "$#" -ne 1 ]; then
  echo 'Usage: topsec-device-reference-test.sh /path/to/TopSAP-3.6.3.32.2.22375_amd64.deb' >&2
  exit 2
fi
package=$1
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
printf '%s  %s\n' b80632d99c9ae1a1283a03f0c6379285e652e1aec016788661d4d270eb705ba1 "$package" | sha256sum --check --status
dpkg-deb --fsys-tarfile "$package" | tar -x -C "$work" ./opt/apps/cn.com.topvpn/files/libtopsec_vpn.so
library="$work/opt/apps/cn.com.topvpn/files/libtopsec_vpn.so"
printf '%s  %s\n' 4bad4c46933551d9e8e132821779b3b77230ea5306f4ee713dd40849d83dd981 "$library" | sha256sum --check --status
echo 'Official package and library SHA-256 verified; building synthetic native probe'

cat >"$work/reference.cpp" <<'CPP'
#include <cerrno>
#include <cstddef>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <cstdarg>
#include <dlfcn.h>
#include <fcntl.h>
#include <linux/filter.h>
#include <linux/seccomp.h>
#include <string>
#include <sys/prctl.h>
#include <sys/syscall.h>
#include <unistd.h>

static bool disk_available;
static unsigned mount_reads, disk_opens, disk_queries, uuid_reads;
static constexpr int disk_fd = 901;

// These symbols replace only the observed native hardware calls. Unexpected
// calls fail the test; no real filesystem/device identity is consulted.
extern "C" FILE *fopen(const char *path, const char *mode) {
    if (std::strcmp(path, "/etc/mtab") || std::strcmp(mode, "r")) _exit(81);
    ++mount_reads;
    static char mounts[] = "/dev/ignored1 /boot ext4 rw 0 0\n/dev/sda12 / ext4 rw 0 0\n";
    return fmemopen(mounts, sizeof(mounts) - 1, "r");
}
extern "C" int open(const char *path, int flags, ...) {
    if (std::strcmp(path, "/dev/sda") || flags != O_RDONLY) _exit(82);
    ++disk_opens;
    return disk_fd;
}
extern "C" int close(int fd) {
    if (fd == disk_fd) return 0;
    return static_cast<int>(syscall(SYS_close, fd));
}
extern "C" int ioctl(int fd, unsigned long request, ...) {
    if (fd != disk_fd || request != 0x030d) _exit(83); // HDIO_GET_IDENTITY
    ++disk_queries;
    if (!disk_available) { errno = ENOTTY; return -1; }
    va_list args;
    va_start(args, request);
    auto data = va_arg(args, unsigned char *);
    va_end(args);
    std::memset(data, 0, 512);
    std::memcpy(data + 20, " a b c ", 7);
    return 0;
}
extern "C" FILE *popen(const char *command, const char *mode) {
    if (std::strcmp(command, "cat /sys/class/dmi/id/product_uuid") ||
        std::strcmp(mode, "r") || disk_available) _exit(84);
    ++uuid_reads;
    static char uuid[] = "00000000-1111-2222-3333-444444444444\n";
    return fmemopen(uuid, sizeof(uuid) - 1, "r");
}
extern "C" int pclose(FILE *stream) { return fclose(stream); }
int FomatAndPushLog(int, const char *, const char *, ...) { return 0; }

static void deny_real_identity_io() {
    // dlopen has completed. Kill on any attempt to bypass the synthetic inputs,
    // execute a command, or open a network connection during the native call.
#define DENY(n) BPF_JUMP(BPF_JMP | BPF_JEQ | BPF_K, SYS_##n, 0, 1), BPF_STMT(BPF_RET | BPF_K, SECCOMP_RET_KILL_PROCESS)
    sock_filter filter[] = {
        BPF_STMT(BPF_LD | BPF_W | BPF_ABS, offsetof(seccomp_data, nr)),
        DENY(open), DENY(openat), DENY(openat2), DENY(creat),
        DENY(execve), DENY(execveat), DENY(socket), DENY(connect),
        BPF_STMT(BPF_RET | BPF_K, SECCOMP_RET_ALLOW),
    };
#undef DENY
    sock_fprog program{static_cast<unsigned short>(sizeof(filter) / sizeof(filter[0])), filter};
    if (prctl(PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) ||
        prctl(PR_SET_SECCOMP, SECCOMP_MODE_FILTER, &program)) _exit(85);
}

int main(int argc, char **argv) {
    if (argc != 3) return 86;
    disk_available = std::strcmp(argv[2], "disk") == 0;
    void *library = dlopen(argv[1], RTLD_NOW | RTLD_LOCAL);
    if (!library) { std::fputs(dlerror(), stderr); return 87; }
    auto ctor = reinterpret_cast<void (*)(void *)>(dlsym(library, "_ZN17FeatureCodeHelperC1Ev"));
    auto dtor = reinterpret_cast<void (*)(void *)>(dlsym(library, "_ZN17FeatureCodeHelperD1Ev"));
    auto feature = reinterpret_cast<std::string (*)(void *)>(dlsym(library, "_ZN17FeatureCodeHelper15getFeatureCodesB5cxx11Ev"));
    auto unhex = reinterpret_cast<void (*)(const char *, int, void *)>(dlsym(library, "HexStringToByte"));
    auto base64 = reinterpret_cast<size_t (*)(const void *, size_t, char *)>(dlsym(library, "Base64Encode"));
    if (!ctor || !dtor || !feature || !unhex || !base64 || sizeof(std::string) != 32) return 88;
    deny_real_identity_io();
    alignas(std::string) unsigned char object[3 * sizeof(std::string)];
    ctor(object);
    std::string hex = feature(object);
    dtor(object);
    if (hex.size() != 32 || mount_reads != 1 || disk_opens != 1 || disk_queries != 1 ||
        uuid_reads != (disk_available ? 0u : 1u)) return 89;
    unsigned char digest[16]{};
    char code[64]{};
    unhex(hex.c_str(), 32, digest);
    base64(digest, sizeof(digest), code);
    const char *want = disk_available ? "9ZgAH2EHpqCdTiEgY662wA==" : "0kHjlf0il4PC1hgCAOqddA==";
    if (std::strcmp(code, want)) return 90;
    std::puts(disk_available ? "native synthetic disk vector: PASS" : "native synthetic DMI vector: PASS");
    return 0;
}
CPP
c++ -std=c++17 -Wall -Wextra -Werror -rdynamic "$work/reference.cpp" -ldl -o "$work/reference"
"$work/reference" "$library" disk
"$work/reference" "$library" dmi

# The Go tests use the same literal vectors and assert hardware order/redaction.
go test ./engine/topsec -run '^TestLinuxHardwareIdentityOrderAndRedaction$' -count=1
