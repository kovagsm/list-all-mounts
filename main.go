package main

import (
	"fmt"
	"list-all-mounts/mountlister"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ProcInfo struct {
	Pid  int
	Name string
}

func getMountNsProcesses(procfs string, mountNsIno uint64) ([]ProcInfo, error) {
	seen := make(map[uint64]struct{})
	var ret []ProcInfo

	ents, err := os.ReadDir(procfs)
	if err != nil {
		return nil, fmt.Errorf("Error reading procfs %w", err)
	}

	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		pid := e.Name()
		iPid, err := strconv.Atoi(pid)
		if err != nil {
			continue
		}
		p := filepath.Join(procfs, pid, "ns", "mnt")
		linkTarget, err := os.Readlink(p)
		if err != nil {
			continue
		}
		// linkTarget example: "mnt:[4026531841]"
		start := strings.LastIndexByte(linkTarget, '[')
		end := strings.LastIndexByte(linkTarget, ']')
		if start == -1 || end == -1 || start >= end-1 {
			continue
		}
		inodeStr := linkTarget[start+1 : end]
		ino, err := strconv.ParseUint(inodeStr, 10, 64)
		if _, ok := seen[ino]; ok {
			continue
		}

		if ino != mountNsIno {
			continue
		}

		p = filepath.Join(procfs, pid, "exe")
		linkTarget, err = os.Readlink(p)
		if err != nil {
			continue
		}
		var proc ProcInfo
		proc.Pid = iPid
		proc.Name = linkTarget
		ret = append(ret, proc)
	}
	return ret, nil

}

// toNamespaceMap returns a mapping of the mount namespace to a list of mountpoints in that mount namespace
func toNamespaceMap(lst []*mountlister.Statmount) map[uint64][]*mountlister.Statmount {
	var ret = make(map[uint64][]*mountlister.Statmount)

	for _, e := range lst {
		ret[e.NamespaceInode] = append(ret[e.NamespaceInode], e)
	}

	return ret
}

func toMountidMap(lst []*mountlister.Statmount) map[uint64]*mountlister.Statmount {
	ret := make(map[uint64]*mountlister.Statmount)

	for _, e := range lst {
		ret[e.Mnt_id] = e
	}

	return ret
}

// toParentMap returns mapping from the mountid to a pointer to its parent. if the parent isn't found, it maps to nil
func toParentMap(lst map[uint64]*mountlister.Statmount) map[uint64]*mountlister.Statmount {
	var ret = make(map[uint64]*mountlister.Statmount)

	for _, e := range lst {
		parent, ok := lst[e.Mnt_parent_id]
		if !ok {
			ret[e.Mnt_id] = nil
		} else {
			ret[e.Mnt_id] = parent
		}
	}

	return ret
}

// toMountIdChildrenMap maps the mount ids to a list of its children
func toMountIDChildrenMap(lst []*mountlister.Statmount) map[uint64][]*mountlister.Statmount {
	ret := make(map[uint64][]*mountlister.Statmount)

	for _, e := range lst {
		for _, f := range lst {
			if f.Mnt_parent_id == e.Mnt_id {
				ret[e.Mnt_id] = append(ret[e.Mnt_id], f)
			}
		}
	}

	return ret
}

func printLevel(level int, str string) {
	for i := 0; i < level; i++ {
		fmt.Printf("   ")
	}
	fmt.Printf("|--")
	fmt.Printf("%s\n", str)
}

func formatMount(m *mountlister.Statmount) string {
	if m.Shown == true {
		fmt.Printf("ERROR: Mount %d already shown", m.Mnt_id)
	}
	m.Shown = true
	str := ""
	if m.ObtainedOnScooping == true {
		str = "*"
	}
	return str + fmt.Sprintf("(%d -> %d / %d) (%d:%d) [%s] [%s] [%s]", m.Mnt_id, m.Mnt_id_old, m.Mnt_peer_group, m.Sb_dev_major, m.Sb_dev_minor, m.Fs_type, m.Mnt_point, m.Mnt_root)
}

func printChildrenRecursive(level int, m *mountlister.Statmount, childrenMap map[uint64][]*mountlister.Statmount, mountIdMap map[uint64]*mountlister.Statmount) {
	str := formatMount(m)

	printLevel(level, str)

	children := childrenMap[m.Mnt_id]
	for _, child := range children {
		printChildrenRecursive(level+1, child, childrenMap, mountIdMap)
	}
}

type mountFiles struct {
	namespaceInode uint64
	filename       string
}

func getMountFiles(list map[uint64]*mountlister.Statmount) []mountFiles {
	ret := make([]mountFiles, 0)
	for _, m := range list {
		if len(m.Mnt_root) > 5 && m.Mnt_root[:5] == "mnt:[" {
			ret = append(ret, mountFiles{
				m.NamespaceInode,
				m.Mnt_point,
			})
		}
	}

	return ret
}

func main() {
	procDir := "/proc"
	list, err := mountlister.GetAll(procDir)

	if err != nil {
		fmt.Println("error:", err)
		return
	}

	mountlister.BruteforceMissing(list)

	mountFiles := getMountFiles(list)
	for _, m := range mountFiles {
		fmt.Println("Mountfile:", m)
	}

	values := make([]*mountlister.Statmount, 0, len(list))
	for _, sm := range list {
		values = append(values, sm)
	}

	mountsOnNamespace := toNamespaceMap(values)

	for namespace, _ := range mountsOnNamespace {
		fmt.Println("Namespace:", namespace, "Mounts:", len(mountsOnNamespace[namespace]))
		fmt.Println()
		processes, err := getMountNsProcesses(procDir, namespace)
		if err != nil {
			fmt.Println("Error:", err)
		} else {
			fmt.Println("Processes in this namespace:")
			for _, process := range processes {
				fmt.Printf("[%d] %s\n", process.Pid, process.Name)
			}

			fmt.Println()
		}
		mountsOnThisNamespace := mountsOnNamespace[namespace]
		mountIdMapping := toMountidMap(mountsOnThisNamespace)
		childrenMapping := toMountIDChildrenMap(mountsOnThisNamespace)

		parent := uint64(0)
		for _, e := range mountsOnThisNamespace {
			if e.Mnt_parent_id == e.Mnt_id {
				fmt.Println(formatMount(e))
				parent = e.Mnt_parent_id
				break
			}
		}

		for _, e := range mountIdMapping {
			if e.Mnt_parent_id == parent && e.Mnt_id != parent {
				printChildrenRecursive(0, mountIdMapping[e.Mnt_id], childrenMapping, list)
			}
		}

		fmt.Println()
		fmt.Println("========================================================")
	}

	fmt.Println("Mounts that were never shown:")
	for _, e := range list {
		if !e.Shown {
			fmt.Println(formatMount(e))
		}
	}
}
