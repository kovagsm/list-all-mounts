package mountlister

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unsafe"
)

// Adjust for your arch/kernel if needed (these match your current file)
const (
	SYS_STATMOUNT = 457
	SYS_LISTMOUNT = 458

	LSMT_ROOT = ^uint64(0)

	STATMOUNT_SB_BASIC       = 0x00000001
	STATMOUNT_MNT_BASIC      = 0x00000002
	STATMOUNT_PROPAGATE_FROM = 0x00000004
	STATMOUNT_MNT_ROOT       = 0x00000008
	STATMOUNT_MNT_POINT      = 0x00000010
	STATMOUNT_FS_TYPE        = 0x00000020
)

type mntIDReq struct {
	Size  uint32
	Spare uint32
	MntID uint64
	Param uint64
}

type statmountFixed struct {
	Size              uint32
	Spare1            uint32
	Mask              uint64
	Sb_dev_major      uint32
	Sb_dev_minor      uint32
	Sb_magic          uint64
	Sb_flags          uint32
	Fs_type           uint32
	Mnt_id            uint64
	Mnt_parent_id     uint64
	Mnt_id_old        uint32
	Mnt_parent_id_old uint32
	Mnt_attr          uint64
	Mnt_propagation   uint64
	Mnt_peer_group    uint64
	Mnt_master        uint64
	Propagate_from    uint64
	Mnt_root          uint32
	Mnt_point         uint32
	Spare2            [50]uint64
}

type Statmount struct {
	NamespaceInode    uint64
	Mask              uint64
	Sb_dev_major      uint32
	Sb_dev_minor      uint32
	Sb_magic          uint64
	Sb_flags          uint32
	Fs_type           string
	Mnt_id            uint64
	Mnt_parent_id     uint64
	Mnt_id_old        uint32
	Mnt_parent_id_old uint32
	Mnt_attr          uint64
	Mnt_propagation   uint64
	Mnt_peer_group    uint64
	Mnt_master        uint64
	Propagate_from    uint64
	Mnt_root          string
	Mnt_point         string

	// Isnt part of the statmount
	Shown              bool
	ObtainedOnScooping bool
}

func ztToString(b []byte) string {
	for i, v := range b {
		if v == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

func parseStatmount(buf []byte) Statmount {
	var hdr statmountFixed
	_ = binary.Read(bytes.NewReader(buf), binary.NativeEndian, &hdr)
	base := uint32(unsafe.Sizeof(statmountFixed{}))
	return Statmount{
		Mask:              hdr.Mask,
		Sb_dev_major:      hdr.Sb_dev_major,
		Sb_dev_minor:      hdr.Sb_dev_minor,
		Sb_magic:          hdr.Sb_magic,
		Sb_flags:          hdr.Sb_flags,
		Fs_type:           ztToString(buf[base+hdr.Fs_type:]),
		Mnt_id:            hdr.Mnt_id,
		Mnt_parent_id:     hdr.Mnt_parent_id,
		Mnt_id_old:        hdr.Mnt_id_old,
		Mnt_parent_id_old: hdr.Mnt_parent_id_old,
		Mnt_attr:          hdr.Mnt_attr,
		Mnt_propagation:   hdr.Mnt_propagation,
		Mnt_peer_group:    hdr.Mnt_peer_group,
		Mnt_master:        hdr.Mnt_master,
		Propagate_from:    hdr.Propagate_from,
		Mnt_point:         ztToString(buf[base+hdr.Mnt_point:]),
		Mnt_root:          ztToString(buf[base+hdr.Mnt_root:]),
	}
}

func listmount(req *mntIDReq, ids []uint64) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	r1, _, e := unix.RawSyscall6(
		SYS_LISTMOUNT,
		uintptr(unsafe.Pointer(req)),
		uintptr(unsafe.Pointer(&ids[0])),
		uintptr(len(ids)),
		0, 0, 0,
	)
	if e != 0 {
		return 0, e
	}
	return int(r1), nil
}

func statmount(req *mntIDReq, buf []byte) error {
	_, _, e := unix.RawSyscall6(
		SYS_STATMOUNT,
		uintptr(unsafe.Pointer(req)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		0, 0, 0,
	)
	if e != 0 {
		return e
	}
	return nil
}

type mountNsInoFd struct {
	ino uint64
	fd  int
}

func getInodeNumFromLink(link string) (uint64, error) {
	start := strings.LastIndexByte(link, '[')
	end := strings.LastIndexByte(link, ']')
	if start == -1 || end == -1 || start >= end-1 {
		return 0, fmt.Errorf("invalid link: %s", link)
	}
	inodeStr := link[start+1 : end]
	ino, err := strconv.ParseUint(inodeStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid link: %s", link)
	}
	return ino, nil
}

func collectUniqueMountNSFDsFromProcFd(procfs string, seen map[uint64]struct{}) ([]mountNsInoFd, error) {
	var fds []mountNsInoFd

	pids, err := os.ReadDir(procfs)
	if err != nil {
		return nil, err
	}
	for _, p := range pids {
		if !p.IsDir() {
			continue
		}
		pid := p.Name()
		if _, err := strconv.Atoi(pid); err != nil {
			continue
		}

		procPidFd := filepath.Join(procfs, pid, "fd")
		fdsInPid, err := os.ReadDir(procPidFd)
		if err != nil {
			return nil, err
		}

		for _, fdPid := range fdsInPid {
			filename := filepath.Join(procfs, pid, "fd", fdPid.Name())

			linkTarget, err := os.Readlink(filename)

			if err != nil {
				continue
			}

			if len(linkTarget) < 5 || linkTarget[:5] != "mnt:[" {
				continue
			}

			ino, err := getInodeNumFromLink(linkTarget)
			if err != nil {
				continue
			}

			if _, ok := seen[ino]; ok {
				//fmt.Printf("Found inode %d but it was already seen\n", ino)
				continue
			}
			fmt.Printf("Inserting unseen inode %d\n", ino)

			fd, err := unix.Open(filename, unix.O_RDONLY|unix.O_CLOEXEC, 0)
			if err != nil {
				continue
			}

			seen[ino] = struct{}{}
			fdElem := mountNsInoFd{
				ino: ino,
				fd:  fd,
			}
			fds = append(fds, fdElem)
		}
	}
	return fds, nil
}

func collectUniqueMountNSFDs(procfs string, seen map[uint64]struct{}) ([]mountNsInoFd, error) {
	var fds []mountNsInoFd

	pids, err := os.ReadDir(procfs)
	if err != nil {
		return nil, err
	}
	for _, p := range pids {
		if !p.IsDir() {
			continue
		}
		pid := p.Name()
		if _, err := strconv.Atoi(pid); err != nil {
			continue
		}
		p := filepath.Join(procfs, pid, "ns", "mnt")
		linkTarget, err := os.Readlink(p)
		if err != nil {
			continue
		}

		ino, err := getInodeNumFromLink(linkTarget)
		if err != nil {
			continue
		}

		if _, ok := seen[ino]; ok {
			continue
		}
		fd, err := unix.Open(p, unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			continue
		}
		seen[ino] = struct{}{}
		fdElem := mountNsInoFd{
			ino: ino,
			fd:  fd,
		}
		fds = append(fds, fdElem)
	}
	return fds, nil
}

func GetStatmount(mountId uint64) (*Statmount, error) {
	mask := uint64(STATMOUNT_SB_BASIC |
		STATMOUNT_MNT_BASIC |
		STATMOUNT_PROPAGATE_FROM |
		STATMOUNT_MNT_ROOT |
		STATMOUNT_MNT_POINT |
		STATMOUNT_FS_TYPE)

	buf := make([]byte, 4096)
	req2 := mntIDReq{
		Size:  uint32(unsafe.Sizeof(mntIDReq{})),
		Spare: 0,
		MntID: mountId,
		Param: mask,
	}
	if err := statmount(&req2, buf); err != nil {
		return nil, fmt.Errorf("failed to statmount: %v", err)
	}
	ret := parseStatmount(buf)
	return &ret, nil
}

func GetAll(procfs string) (map[uint64]*Statmount, error) {
	seen := make(map[uint64]struct{})
	nsFDs, err := collectUniqueMountNSFDs(procfs, seen)

	fmt.Println("Collecting unique mount nsfd")
	nsFDs2, err := collectUniqueMountNSFDsFromProcFd(procfs, seen)

	for _, e := range nsFDs2 {
		fmt.Println(e)
	}

	if err != nil {
		return nil, err
	}
	defer func() {
		for _, inoFd := range nsFDs {
			unix.Close(inoFd.fd)
		}
	}()

	done := make(chan error, 1)
	ret := make(map[uint64]*Statmount)
	go func() {
		runtime.LockOSThread()
		// Lock but don't unlock, because as per go's runtime package documentation,
		// "If the calling goroutine exits without unlocking the thread, the thread will be terminated."

		if err := unix.Unshare(unix.CLONE_FS); err != nil {
			done <- err
			return
		}

		ids := make([]uint64, 2048)
		buf := make([]byte, 4096)
		mask := uint64(STATMOUNT_SB_BASIC |
			STATMOUNT_MNT_BASIC |
			STATMOUNT_PROPAGATE_FROM |
			STATMOUNT_MNT_ROOT |
			STATMOUNT_MNT_POINT |
			STATMOUNT_FS_TYPE)

		for _, inoFd := range nsFDs {
			firstIteration := true
			lastMountId := uint64(0)

			for {
				nsMounts := make(map[uint64]*Statmount)

				req := mntIDReq{
					Size:  uint32(unsafe.Sizeof(mntIDReq{})),
					Spare: 0,
					MntID: LSMT_ROOT,
					Param: 0,
				}
				if firstIteration {
					if err := unix.Setns(inoFd.fd, unix.CLONE_NEWNS); err != nil {
						done <- fmt.Errorf("failed to setns: %v", err)
						return
					}
				} else {
					req.Param = lastMountId
				}

				n, err := listmount(&req, ids)

				if err != nil || n < 0 {
					done <- fmt.Errorf("failed to listmount: %v", err)
					return
				}

				for i := 0; i < n; i++ {
					req2 := mntIDReq{
						Size:  uint32(unsafe.Sizeof(mntIDReq{})),
						Spare: 0,
						MntID: ids[i],
						Param: mask,
					}
					if err := statmount(&req2, buf); err != nil {
						done <- fmt.Errorf("failed to statmount: %v", err)
						return
					}
					sm := parseStatmount(buf)
					sm.NamespaceInode = inoFd.ino

					if _, ok := ret[sm.Mnt_id]; ok {
						fmt.Println("Element already existed:", sm.Mnt_id)
					}
					ret[sm.Mnt_id] = &sm
					nsMounts[sm.Mnt_id] = &sm
					lastMountId = req2.MntID
				}

				if n == len(ids) {
					// Not all mounts were obtained yet
					firstIteration = false
					continue
				}

				if n < len(ids) {
					// All mounts for this namespace were obtained
					foundNewStuff := true
					for foundNewStuff {
						foundNewStuff = false
						for _, sm := range nsMounts {
							if _, ok := nsMounts[sm.Mnt_parent_id]; ok {
								continue
							}

							if e, ok := ret[sm.Mnt_parent_id]; ok {
								// This is present in more than one namespace, so set the namespace inode to zero
								fmt.Println("Found in more than one namespace:", sm.Mnt_parent_id)
								e.NamespaceInode = 0
								continue
							}

							parent, err := GetStatmount(sm.Mnt_parent_id)
							if err != nil {
								fmt.Printf("Error getting stat mount for %d: %v", sm.Mnt_parent_id, err)
							} else {
								foundNewStuff = true
								parent.NamespaceInode = sm.NamespaceInode
								ret[parent.Mnt_id] = parent
								nsMounts[parent.Mnt_id] = parent
							}
						}
					}
					break
				}
			}
		}
		done <- nil
	}()
	err = <-done
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func BruteforceMissing(mounts map[uint64]*Statmount) {
	lowest := uint64(4294967296)
	highest := uint64(0)
	for id, _ := range mounts {
		if id > highest {
			highest = id
		}
	}

	seen := make(map[uint64]struct{})
	nsFDs, err := collectUniqueMountNSFDs("/proc", seen)
	if err != nil {
		fmt.Println("Error collecting mounts:", err)
		return
	}
	defer func() {
		for _, inoFd := range nsFDs {
			unix.Close(inoFd.fd)
		}
	}()

	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		// Lock but don't unlock, because as per go's runtime package documentation,
		// "If the calling goroutine exits without unlocking the thread, the thread will be terminated."

		if err := unix.Unshare(unix.CLONE_FS); err != nil {
			done <- err
			return
		}

		for _, inoFd := range nsFDs {

			if err := unix.Setns(inoFd.fd, unix.CLONE_NEWNS); err != nil {
				done <- fmt.Errorf("failed to setns: %v", err)
				return
			}

			for i := lowest; i <= highest; i++ {
				if _, ok := mounts[i]; ok {
					//already exists
					continue
				}

				sm, err := GetStatmount(i)
				if err != nil {
					continue
				}
				sm.NamespaceInode = inoFd.ino
				sm.ObtainedOnScooping = true
				mounts[sm.Mnt_id] = sm
			}
		}
		done <- nil
	}()

	err = <-done
	if err != nil {
		fmt.Println("Error getting mounts:", err)
		return
	}

}
