//go:build windows

package discovery

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	memCommit       = 0x1000
	memPrivate      = 0x20000
	pageNoAccess    = 0x01
	pageGuard       = 0x100
	tcpTableListen  = 3 // TCP_TABLE_OWNER_PID_LISTENER
	errInsufficient = 122
	loopbackRaw     = 0x0100007F // 127.0.0.1 as GetExtendedTcpTable stores it
)

var procGetExtendedTCPTable = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")

type windowsPlatform struct{}

func newPlatform() platform { return windowsPlatform{} }

func (windowsPlatform) findGames(context.Context) ([]game, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	defer windows.CloseHandle(snapshot)

	var found []game
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), GameExe) {
			found = append(found, game{pid: int(entry.ProcessID), exe: imagePath(entry.ProcessID)})
		}
	}
	return found, nil
}

func imagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	return imageName(h)
}

func imageName(h windows.Handle) string {
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

// closable accepts only games whose token belongs to the user running
// Team Follow. A game whose owner can't be read is left alone.
func (windowsPlatform) closable(g game) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(g.pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var token windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token); err != nil {
		return false
	}
	defer token.Close()
	owner, err := token.GetTokenUser()
	if err != nil {
		return false
	}
	self, err := windows.GetCurrentProcessToken().GetTokenUser()
	return err == nil && owner.User.Sid.Equals(self.User.Sid)
}

// stop terminates g once its handle confirms it still runs the game.
// Windows has no polite request for a program without a window message
// loop of ours, so both calls terminate.
func (windowsPlatform) stop(g game, _ bool) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(g.pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return nil // already gone
		}
		return err
	}
	defer windows.CloseHandle(h)
	name := imageName(h)
	if name == "" {
		return errors.New("couldn't confirm the process is the game")
	}
	if !isGameExe(name) {
		return nil // the pid now belongs to another program
	}
	return windows.TerminateProcess(h, 1)
}

type tcpRow struct {
	state, localAddr, localPort, remoteAddr, remotePort, pid uint32
}

func (windowsPlatform) listeners(_ context.Context, g game) ([]int, error) {
	var size uint32
	for range 3 {
		buf := make([]byte, max(size, 4))
		r, _, _ := procGetExtendedTCPTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, windows.AF_INET, tcpTableListen, 0)
		if r == errInsufficient {
			continue
		}
		if r != 0 {
			return nil, fmt.Errorf("read the TCP table: error %d", r)
		}
		count := *(*uint32)(unsafe.Pointer(&buf[0]))
		rows := unsafe.Slice((*tcpRow)(unsafe.Pointer(&buf[4])), count)
		var ports []int
		for _, row := range rows {
			if row.pid != uint32(g.pid) || row.localAddr != loopbackRaw {
				continue
			}
			port := int(row.localPort&0xff)<<8 | int(row.localPort>>8&0xff)
			if port >= 1024 {
				ports = append(ports, port)
			}
		}
		slices.Sort(ports)
		return slices.Compact(ports), nil
	}
	return nil, fmt.Errorf("the TCP table kept changing")
}

// session searches the game's committed private memory, as the original
// Windows release did, with read-only access.
func (windowsPlatform) session(ctx context.Context, g game) (string, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, uint32(g.pid))
	if err != nil {
		return "", fmt.Errorf("%w Run Team Follow as the same Windows user as the game.", ErrAccess)
	}
	defer windows.CloseHandle(h)

	var regions []region
	var info windows.MemoryBasicInformation
	for addr := uintptr(0); ; {
		if err := windows.VirtualQueryEx(h, addr, &info, unsafe.Sizeof(info)); err != nil {
			break
		}
		end := info.BaseAddress + info.RegionSize
		if end <= addr {
			break
		}
		if info.State == memCommit && info.Type == memPrivate && info.Protect&(pageNoAccess|pageGuard) == 0 {
			regions = append(regions, region{uint64(info.BaseAddress), uint64(end)})
		}
		addr = end
	}
	session := scanRegions(ctx, regions, func(buf []byte, addr uint64) (int, error) {
		var n uintptr
		err := windows.ReadProcessMemory(h, uintptr(addr), &buf[0], uintptr(len(buf)), &n)
		return int(n), err
	})
	if session == "" {
		return "", ErrNoSession
	}
	return session, nil
}

func (windowsPlatform) version(_ context.Context, g game) string {
	if g.exe == "" {
		return ""
	}
	return peFileVersion(g.exe)
}
