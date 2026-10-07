# list-all-mounts

`list-all-mounts` is an exploratory Linux tool for enumerating mount trees
across mount namespaces. It uses the `listmount(2)` and `statmount(2)` kernel
APIs to inspect mount relationships, 64-bit mount IDs, visibility, and mounts
that are not returned by normal namespace enumeration.

The project grew from an investigation into a simple question: can every mount
on a Linux host be discovered and assigned to its mount namespace? The answer
is more complicated than reading `/proc/<pid>/mountinfo`.

> [!WARNING]
> This is a research prototype, not a production inventory tool. It changes
> mount namespaces on a dedicated thread and performs an expensive brute-force
> scan of mount IDs. Run it only on systems where that cost is acceptable.

## What it does

For every mount namespace it can discover through `/proc`, the program:

1. Opens one namespace file descriptor from `/proc/<pid>/ns/mnt`.
2. Locks a goroutine to its OS thread and separates its filesystem context with
   `unshare(CLONE_FS)`.
3. Enters each namespace with `setns(2)`.
4. Uses `listmount(2)` to enumerate 64-bit mount IDs.
5. Uses `statmount(2)` to retrieve each mount's parent, legacy ID, peer group,
   device, filesystem type, mount point, and mount root.
6. Walks missing parent IDs to reconstruct the tree up to the namespace root.
7. Probes the remaining mount ID range to find mounts omitted from the initial
   enumeration.
8. Prints one tree per namespace, together with the processes using it.

The namespace work runs on a dedicated OS thread. The goroutine intentionally
exits without calling `runtime.UnlockOSThread`, causing the Go runtime to
discard that thread instead of returning a thread in a changed namespace to
the scheduler.

## Requirements

- Linux with `listmount(2)` and `statmount(2)` support. These APIs were
  introduced in Linux 6.8, but the available fields and enumeration behavior
  depend on the kernel version.
- Go 1.24.7, as specified by `go.mod`.
- Root or equivalent privileges for reading process namespace data and using
  `setns(2)` and `unshare(2)`.
- An architecture using syscall numbers 457 for `statmount` and 458 for
  `listmount`. The prototype currently hard-codes these values in
  `mountlister/listmount.go`.

## Build and run

```bash
git clone https://github.com/kovagsm/list-all-mounts.git
cd list-all-mounts
go build -o list-all-mounts .
sudo ./list-all-mounts
```

The program currently has no command-line options and always inspects `/proc`.
The brute-force phase can take a long time when the highest observed mount ID
is far above `2^32`.

## Reading the output

Each namespace starts with its inode and the processes currently attached to
it:

```text
Namespace: 4026532715 Mounts: 178

Processes in this namespace:
[1549] /usr/lib/systemd/systemd-oomd
```

Each mount uses this format:

```text
(unique_mount_id -> legacy_mount_id / peer_group_id) (major:minor) [filesystem] [mount_point] [mount_root]
```

For example:

```text
(4294967626 -> 298 / 0) (0:2) [rootfs] [] [/]
|--(4294967627 -> 299 / 0) (252:2) [ext4] [] [/]
   |--*(4294967670 -> 342 / 0) (7:18) [squashfs] [] [/]
```

An asterisk marks a mount found only during the mount ID scan, not during the
initial `listmount(2)` enumeration. These entries are commonly mounted objects
that are hidden beneath another mount or otherwise absent from the visible
namespace tree.

## Investigation history

### Version 1: `/proc/<pid>/mountinfo`

The first version iterated over processes and parsed
`/proc/<pid>/mountinfo`.

- It required repeated parsing of text files.
- Multiple processes often represented the same mount namespace.
- It exposed only the mounts visible through that procfs view.
- It did not provide the kernel's 64-bit unique mount ID.

### Version 2: `listmount(2)` and `statmount(2)`

The second version collected distinct namespace file descriptors from `/proc`,
entered each namespace, listed its 64-bit mount IDs, and called `statmount(2)`
for their metadata.

This removed the need to parse mountinfo text, exposed more mount metadata, and
provided enough information to build a tree for each namespace. The performance
difference from procfs was not benchmarked.

### Version 3: recovering missing parents

`listmount(2)` could return mounts whose parent IDs were not present in the same
result. Calling `statmount(2)` directly for those parent IDs allowed the tool to
walk upward until reaching the namespace root, whose parent ID points to
itself.

This produced a more complete tree, but it still did not reveal every mount
known to the kernel.

### Version 4: scanning the mount ID space

The current version probes mount IDs from the smallest possible 64-bit unique
ID, `2^32`, through the highest ID observed during normal enumeration.

Successful `statmount(2)` calls reveal additional mounted objects that were not
returned by `listmount(2)`. The output marks these entries with an asterisk.
This method is deliberately brute force and is useful for investigation rather
than routine collection.

## Findings and limitations

- `/proc` is not a complete index of mount namespaces. A namespace can outlive
  its last process when another object keeps it referenced.
- Namespace files can be pinned with bind mounts, as snapd does under
  `/run/snapd/ns`, or kept alive through open file descriptors.
- Enumerating every process namespace therefore cannot prove that every
  namespace on the host has been found.
- Walking missing parent IDs improves the mount tree but does not make
  `listmount(2)` enumeration complete.
- The brute-force scan stops at the highest ID already observed. It cannot
  discover an otherwise hidden mount with a higher ID.
- The system can change while it is being inspected. Processes, namespaces,
  and mounts may disappear between enumeration and lookup.
- Output order is not stable because namespaces and mounts are stored in Go
  maps.
- Syscall numbers are hard-coded and may need to be changed for another
  architecture.

Newer kernel work continues to improve mount and mount-namespace enumeration.
See Christian Brauner's article
[Listing all mounts](https://brauner.io/2024/12/16/list-all-mounts.html) for
the APIs added around Linux 6.12 and the kernel-side constraints behind this
problem.

## References

- [`listmount(2)`](https://man7.org/linux/man-pages/man2/listmount.2.html)
- [`statmount(2)`](https://man7.org/linux/man-pages/man2/statmount.2.html)
- [`mount_namespaces(7)`](https://man7.org/linux/man-pages/man7/mount_namespaces.7.html)
- [Listing all mounts](https://brauner.io/2024/12/16/list-all-mounts.html)
