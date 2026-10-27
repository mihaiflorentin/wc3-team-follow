//go:build darwin

package discovery

/*
#include <libproc.h>
#include <mach/mach.h>
#include <mach/mach_vm.h>
#include <stdlib.h>

static int wf_pid_path(int pid, char *buf, int size) {
	return proc_pidpath(pid, buf, (uint32_t)size);
}

static int wf_list_pids(int *buf, int size) {
	return proc_listallpids(buf, size);
}

static kern_return_t wf_task(int pid, mach_port_t *task) {
	return task_for_pid(mach_task_self(), pid, task);
}

static void wf_release(mach_port_t task) {
	mach_port_deallocate(mach_task_self(), task);
}

static kern_return_t wf_region(mach_port_t task, mach_vm_address_t *addr, mach_vm_size_t *size, int *prot, int *share) {
	vm_region_extended_info_data_t info;
	mach_msg_type_number_t count = VM_REGION_EXTENDED_INFO_COUNT;
	mach_port_t object;
	kern_return_t kr = mach_vm_region(task, addr, size, VM_REGION_EXTENDED_INFO, (vm_region_info_t)&info, &count, &object);
	if (kr == KERN_SUCCESS) {
		*prot = info.protection;
		*share = info.share_mode;
	}
	return kr;
}

static kern_return_t wf_read(mach_port_t task, mach_vm_address_t addr, mach_vm_size_t size, void *buf, mach_vm_size_t *out) {
	return mach_vm_read_overwrite(task, addr, size, (mach_vm_address_t)buf, out);
}
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unsafe"
)

// gameBinary is the executable inside the macOS app bundle.
const gameBinary = "/Warcraft III.app/Contents/MacOS/Warcraft III"

type darwinPlatform struct{}

func newPlatform() platform { return darwinPlatform{} }

func (darwinPlatform) findGame(context.Context) (game, error) {
	n := C.wf_list_pids(nil, 0)
	if n <= 0 {
		return game{}, errors.New("list processes failed")
	}
	pids := make([]C.int, n+64)
	n = C.wf_list_pids(&pids[0], C.int(len(pids)*int(unsafe.Sizeof(pids[0]))))
	var found []game
	buf := make([]byte, 4096)
	for _, pid := range pids[:max(n, 0)] {
		if pid <= 0 {
			continue
		}
		l := C.wf_pid_path(pid, (*C.char)(unsafe.Pointer(&buf[0])), C.int(len(buf)))
		if l <= 0 {
			continue
		}
		if path := string(buf[:l]); strings.HasSuffix(path, gameBinary) {
			found = append(found, game{pid: int(pid), exe: path})
		}
	}
	switch len(found) {
	case 0:
		return game{}, ErrNoGame
	case 1:
		return found[0], nil
	default:
		return game{}, ErrManyGames
	}
}

// listeners asks lsof for the loopback TCP listeners of this user's
// processes: the game's own first, then Blizzard's browser helper.
func (darwinPlatform) listeners(ctx context.Context, g game) ([]int, error) {
	out, err := exec.CommandContext(ctx, "/usr/sbin/lsof", "-nP", "-iTCP@127.0.0.1", "-sTCP:LISTEN", "-a", "-u", strconv.Itoa(os.Getuid()), "-Fpcn").Output()
	if err != nil && len(out) == 0 {
		// lsof exits 1 when nothing matches.
		return nil, nil
	}
	return parseLsof(out, g.pid), nil
}

const (
	vmProtRead     = 1
	vmProtWrite    = 2
	smCOW          = 1
	smPrivate      = 2
	smPrivateAlias = 6
	kernSuccess    = 0
)

// session reads the game's private memory. The macOS game is built with
// get-task-allow, so a debugger-entitled app may read it once the user has
// allowed developer tools.
func (darwinPlatform) session(ctx context.Context, g game) (string, error) {
	var task C.mach_port_t
	if kr := C.wf_task(C.int(g.pid), &task); kr != kernSuccess {
		return "", fmt.Errorf("%w To allow it, run: sudo DevToolsSecurity -enable", ErrAccess)
	}
	defer C.wf_release(task)

	var regions []region
	var addr C.mach_vm_address_t
	for {
		var size C.mach_vm_size_t
		var prot, share C.int
		if C.wf_region(task, &addr, &size, &prot, &share) != kernSuccess {
			break
		}
		rw := prot&(vmProtRead|vmProtWrite) == vmProtRead|vmProtWrite
		if rw && (share == smCOW || share == smPrivate || share == smPrivateAlias) {
			regions = append(regions, region{uint64(addr), uint64(addr) + uint64(size)})
		}
		addr += C.mach_vm_address_t(size)
	}

	session := scanRegions(ctx, regions, func(buf []byte, at uint64) (int, error) {
		var n C.mach_vm_size_t
		if kr := C.wf_read(task, C.mach_vm_address_t(at), C.mach_vm_size_t(len(buf)), unsafe.Pointer(&buf[0]), &n); kr != kernSuccess {
			return 0, fmt.Errorf("mach_vm_read: %d", int(kr))
		}
		return int(n), nil
	})
	if session == "" {
		return "", ErrNoSession
	}
	return session, nil
}

var bundleVersion = regexp.MustCompile(`<key>CFBundleVersion</key>\s*<string>([^<]+)</string>`)

// version reads CFBundleVersion from the app bundle's Info.plist.
func (darwinPlatform) version(_ context.Context, g game) string {
	plist := filepath.Join(filepath.Dir(filepath.Dir(g.exe)), "Info.plist")
	raw, err := os.ReadFile(plist)
	if err != nil {
		return ""
	}
	if m := bundleVersion.FindSubmatch(raw); m != nil {
		return strings.TrimSpace(string(m[1]))
	}
	return ""
}
