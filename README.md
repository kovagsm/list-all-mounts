In order to have a better understanding of how the mounts are structured in a Linux system,
in terms of mount tree and mount namespaces, I created this program in go to explore it.

There were several iterations to get here:

## Version 1 (procfs):
* Iterate over all the pids in proc and parse `/proc/<pid>/mountinfo`
* Slow (needs to parse text)
* Not all the mounts was there
* Not able to get the unique mount id (64-bit)

## Version 2 (new api: listmount/statmount):
* List all the namespaces from `/proc/`
* Jump into each namespaces using `setns()`
* Use `listmount()` to list all the unique mount ids (64-bit) present in the namespace
* Call `statmount()` for each id
* Build a mount tree for each namespace
* Faster because text don't need to get parsed and goes directly in a struct (probably, not benchmarked)
* Still able to list the same mounts as the procfs version
* Able to get more information such as the unique mount id (64-bit integer, incremental)

## Version 3 (still with the new API):
* The previous method returned mount ids whose parents that weren't reported by `listmount()`
* If we ask `statmount()` it gives us information, and we can go all the way up to find the root mount (rootfs).
* This root of each namespace has itself as the parent mount id
* This allows us to get a better visualization of the entire mount namespaces but still not complete!

## Version 4 (bruteforce mount ids):
* For each mount namespace, iterate:
* From: The smallest possible mount id (2^32 = 4294967296)
* To: The highest mount id found in the previous step
* This will list several invisible mountpoints: They're mounted but hidden (probably beneath other mountpoints)

But there are still more mounts that are hidden!

* A mount namespace can exist while not being present in procfs:
* For example, snapd bind-mounts their mount namespaces to files: /run/snapd/ns/slack.mnt
* Other files can exist on the system pinning the mount namespace with no process attached to it (docker seems to do something like that)

Newer kernel version (from v6.12) seem to have a way to get more visibility into 
mountpoints and mount namespaces: https://brauner.io/2024/12/16/list-all-mounts.html 

## Example of a namespace tree

* The format is: (UniqueMountID → MountID / PeerGroupID) (DevMajor:DevMinor) [FsType] [MountPoint] [MountRoot]
* Mounts with an asterisk (*) aren’t visible and were captured by bruteforce

```
Namespace: 4026532715 Mounts: 178

Processes in this namespace:
[1549] /usr/lib/systemd/systemd-oomd

(4294967626 -> 298 / 0) (0:2) [rootfs] [] [/]
|--(4294967627 -> 299 / 0) (252:2) [ext4] [] [/]
   |--*(4295242837 -> 6841 / 0) (7:28) [xfs] [] [/]
   |--*(4294967670 -> 342 / 0) (7:18) [squashfs] [] [/]
   |--*(4295187292 -> 3091 / 0) (0:85) [fuse] [] [/]
   |--*(4294967665 -> 337 / 0) (7:13) [squashfs] [] [/]
   |--*(4295140619 -> 3231 / 0) (0:73) [overlay] [] [/]
   |--*(4294967676 -> 348 / 0) (7:24) [squashfs] [] [/]
   |--*(4295123940 -> 3407 / 0) (7:16) [squashfs] [] [/]
   |--*(4294967652 -> 324 / 0) (7:1) [squashfs] [] [/]
   |--*(4294967633 -> 305 / 0) (0:26) [tmpfs] [] [/]
      |--*(4295140803 -> 3449 / 0) (0:4) [nsfs] [] [net:[4026532328]]
      |--*(4294967637 -> 309 / 0) (0:41) [rpc_pipefs] [] [/]
      |--*(4295141336 -> 3875 / 0) (0:4) [nsfs] [] [net:[4026533202]]
      |--*(4294972889 -> 3824 / 0) (0:77) [tmpfs] [] [/]
         |--*(4294972901 -> 3836 / 0) (0:78) [fuse] [] [/]
         |--*(4294973437 -> 4372 / 0) (0:79) [fuse] [] [/]
      |--*(4294967636 -> 308 / 0) (0:39) [tmpfs] [] [/]
      |--*(4294967635 -> 307 / 0) (0:37) [ramfs] [] [/]
      |--*(4294967634 -> 306 / 0) (0:30) [tmpfs] [] [/]
      |--*(4294970014 -> 2185 / 0) (0:26) [tmpfs] [] [/snapd/ns]
   |--*(4294967664 -> 336 / 0) (7:12) [squashfs] [] [/]
   |--*(4294971872 -> 3383 / 0) (0:72) [bpf] [] [/]
   |--*(4294967655 -> 327 / 0) (7:3) [squashfs] [] [/]
   |--*(4294967659 -> 331 / 0) (7:7) [squashfs] [] [/]
   |--*(4295176971 -> 1883 / 0) (7:26) [squashfs] [] [/]
   |--*(4295158371 -> 5441 / 0) (252:2) [ext4] [] [/tmp/TestMountPropagated2876723755/001/dir1]
      |--*(4295158372 -> 5806 / 0) (7:9) [xfs] [] [/]
   |--*(4294967657 -> 329 / 0) (7:5) [squashfs] [] [/]
   |--*(4295288708 -> 7106 / 0) (252:2) [ext4] [] [/tmp/TestMountPropagated568392143/001/dir1]
      |--*(4295288709 -> 7107 / 0) (7:32) [xfs] [] [/]
   |--*(4294967654 -> 326 / 0) (7:2) [squashfs] [] [/]
   |--*(4294967675 -> 347 / 0) (7:23) [squashfs] [] [/]
   |--*(4295266403 -> 6950 / 0) (7:31) [squashfs] [] [/]
   |--*(4295158274 -> 3578 / 0) (7:9) [xfs] [] [/]
   |--*(4294967663 -> 335 / 0) (7:11) [squashfs] [] [/]
   |--*(4295240075 -> 3662 / 0) (7:22) [xfs] [] [/]
   |--*(4294971826 -> 3337 / 0) (0:12) [tracefs] [] [/]
   |--*(4295227251 -> 3181 / 0) (7:15) [squashfs] [] [/]
   |--*(4294967677 -> 349 / 0) (7:25) [squashfs] [] [/]
   |--*(4294967658 -> 330 / 0) (7:6) [squashfs] [] [/]
   |--*(4294967638 -> 310 / 0) (0:23) [sysfs] [] [/]
      |--*(4294967645 -> 317 / 0) (0:12) [tracefs] [] [/]
      |--*(4294967646 -> 318 / 0) (0:36) [fusectl] [] [/]
      |--*(4294967641 -> 313 / 0) (0:31) [cgroup2] [] [/]
      |--*(4294967644 -> 316 / 0) (0:7) [debugfs] [] [/]
         |--*(4295086359 -> 3729 / 0) (0:12) [tracefs] [] [/]
      |--*(4294967643 -> 315 / 0) (0:33) [bpf] [] [/]
      |--*(4294967647 -> 319 / 0) (0:21) [configfs] [] [/]
      |--*(4294967642 -> 314 / 0) (0:32) [pstore] [] [/]
      |--*(4294967639 -> 311 / 0) (0:27) [efivarfs] [] [/]
      |--*(4294967640 -> 312 / 0) (0:6) [securityfs] [] [/]
   |--*(4295240172 -> 6773 / 0) (252:2) [ext4] [] [/tmp/TestMountPropagated2250566632/001/dir1]
      |--*(4295240173 -> 6774 / 0) (7:22) [xfs] [] [/]
   |--*(4294967666 -> 338 / 0) (7:14) [squashfs] [] [/]
   |--*(4295248700 -> 7281 / 0) (252:2) [ext4] [] [/tmp/TestMountPropagated3149697396/001/dir1]
      |--*(4295248701 -> 7282 / 0) (7:30) [xfs] [] [/]
   |--*(4294967672 -> 344 / 0) (7:20) [squashfs] [] [/]
   |--*(4294967669 -> 341 / 0) (7:17) [squashfs] [] [/]
   |--*(4294967662 -> 334 / 0) (7:10) [squashfs] [] [/]
   |--*(4294967671 -> 343 / 0) (7:19) [squashfs] [] [/]
   |--*(4295273715 -> 3974 / 0) (7:4) [xfs] [] [/]
   |--*(4295141006 -> 3498 / 0) (0:75) [overlay] [] [/]
   |--*(4294967674 -> 346 / 0) (7:21) [squashfs] [] [/]
   |--*(4295174865 -> 6675 / 0) (7:27) [squashfs] [] [/]
   |--*(4294967678 -> 350 / 0) (259:3) [ext4] [] [/]
      |--*(4294967679 -> 351 / 0) (259:2) [vfat] [] [/]
   |--*(4294967648 -> 320 / 0) (0:24) [proc] [] [/]
      |--*(4294967649 -> 321 / 0) (0:34) [autofs] [] [/]
         |--*(4294967650 -> 322 / 0) (0:40) [binfmt_misc] [] [/]
      |--*(4294967651 -> 323 / 0) (0:38) [nfsd] [] [/]
   |--*(4295248601 -> 7182 / 0) (7:30) [xfs] [] [/]
   |--*(4295288611 -> 7008 / 0) (7:32) [xfs] [] [/]
   |--*(4295095355 -> 3776 / 0) (7:8) [squashfs] [] [/]
   |--*(4294967628 -> 300 / 0) (0:5) [devtmpfs] [] [/]
      |--*(4294967629 -> 301 / 0) (0:25) [devpts] [] [/]
      |--*(4294967631 -> 303 / 0) (0:35) [hugetlbfs] [] [/]
      |--*(4294967632 -> 304 / 0) (0:20) [mqueue] [] [/]
      |--*(4294967630 -> 302 / 0) (0:29) [tmpfs] [] [/]
   |--(4294967788 -> 460 / 120) (252:2) [ext4] [/] [/]
      |--(4294967826 -> 498 / 194) (7:13) [squashfs] [/snap/gnome-42-2204/202] [/]
      |--(4294967825 -> 497 / 193) (7:12) [squashfs] [/snap/gnome-3-38-2004/143] [/]
      |--(4294967947 -> 619 / 209) (0:43) [tmpfs] [/dev] [/]
         |--(4294967948 -> 620 / 210) (0:25) [devpts] [/dev/pts] [/]
         |--(4294967952 -> 624 / 212) (0:20) [mqueue] [/dev/mqueue] [/]
         |--(4294967954 -> 626 / 213) (0:35) [hugetlbfs] [/dev/hugepages] [/]
         |--(4294967951 -> 623 / 211) (0:29) [tmpfs] [/dev/shm] [/]
      |--(4295242838 -> 6842 / 1361) (7:28) [xfs] [/tmp/TestMountEvent2939733372/001] [/]
      |--(4294968020 -> 662 / 231) (0:26) [tmpfs] [/usr/lib/modules] [/systemd/inaccessible/dir]
      |--(4294967827 -> 499 / 195) (7:14) [squashfs] [/snap/gnome-42-2204/226] [/]
      |--(4294967838 -> 510 / 206) (7:25) [squashfs] [/snap/snapd-desktop-integration/315] [/]
      |--(4294967794 -> 466 / 164) (0:26) [tmpfs] [/run] [/]
         |--(4294967797 -> 469 / 166) (0:39) [tmpfs] [/run/qemu] [/]
         |--(4294967798 -> 470 / 167) (0:41) [rpc_pipefs] [/run/rpc_pipefs] [/]
         |--(4294972890 -> 3825 / 1682) (0:77) [tmpfs] [/run/user/1000] [/]
            |--(4294972900 -> 3835 / 1688) (0:78) [fuse] [/run/user/1000/doc] [/]
            |--(4294973436 -> 4371 / 1713) (0:79) [fuse] [/run/user/1000/gvfs] [/]
         |--(4294970015 -> 2186 / 1175) (0:26) [tmpfs] [/run/snapd/ns] [/snapd/ns]
         |--(4294968016 -> 658 / 170) (0:26) [tmpfs] [/run/user] [/systemd/inaccessible/dir]
         |--(4294968011 -> 574 / 0) (0:26) [tmpfs] [/run/systemd/incoming] [/systemd/propagate/systemd-oomd.service]
         |--(4294967795 -> 467 / 165) (0:30) [tmpfs] [/run/lock] [/]
         |--(4294968010 -> 521 / 168) (0:26) [tmpfs] [/run/credentials] [/systemd/inaccessible/dir]
         |--(4295140804 -> 3450 / 1038) (0:4) [nsfs] [/run/docker/netns/d67c9e819587] [net:[4026532328]]
         |--(4295141337 -> 3876 / 1319) (0:4) [nsfs] [/run/docker/netns/01e4e9a11136] [net:[4026533202]]
      |--(4295288612 -> 7009 / 1416) (7:32) [xfs] [/tmp/TestMountPropagated568392143/001/dir1/test-drive] [/]
      |--(4295123941 -> 3408 / 1125) (7:16) [squashfs] [/snap/firefox/7024] [/]
      |--(4294967994 -> 639 / 229) (0:26) [tmpfs] [/root] [/systemd/inaccessible/dir]
      |--(4295176972 -> 1884 / 1013) (7:26) [squashfs] [/snap/firefox/7084] [/]
      |--(4295266404 -> 6951 / 1398) (7:31) [squashfs] [/snap/core22/2139] [/]
      |--(4294967799 -> 471 / 171) (0:23) [sysfs] [/sys] [/]
         |--(4294967803 -> 475 / 175) (0:32) [pstore] [/sys/fs/pstore] [/]
         |--(4294967802 -> 474 / 174) (0:31) [cgroup2] [/sys/fs/cgroup] [/]
         |--(4294967805 -> 477 / 177) (0:7) [debugfs] [/sys/kernel/debug] [/]
            |--(4295086360 -> 3730 / 1159) (0:12) [tracefs] [/sys/kernel/debug/tracing] [/]
         |--(4294967808 -> 480 / 180) (0:21) [configfs] [/sys/kernel/config] [/]
         |--(4294967800 -> 472 / 172) (0:27) [efivarfs] [/sys/firmware/efi/efivars] [/]
         |--(4294967804 -> 476 / 176) (0:33) [bpf] [/sys/fs/bpf] [/]
         |--(4294967801 -> 473 / 173) (0:6) [securityfs] [/sys/kernel/security] [/]
         |--(4294967807 -> 479 / 179) (0:36) [fusectl] [/sys/fs/fuse/connections] [/]
         |--(4294967806 -> 478 / 178) (0:12) [tracefs] [/sys/kernel/tracing] [/]
      |--(4295273716 -> 4059 / 963) (7:4) [xfs] [/tmp/TestMountEvent2921652766/001] [/]
      |--(4295227252 -> 3182 / 974) (7:15) [squashfs] [/snap/snapd/25577] [/]
      |--(4295288710 -> 7108 / 1530) (252:2) [ext4] [/tmp/TestMountPropagated568392143/001/dir1-bind-mounted] [/tmp/TestMountPropagated568392143/001/dir1]
         |--(4295288711 -> 7109 / 1531) (7:32) [xfs] [/tmp/TestMountPropagated568392143/001/dir1-bind-mounted/test-drive] [/]
      |--(4295158373 -> 5807 / 1217) (252:2) [ext4] [/tmp/TestMountPropagated2876723755/001/dir1-bind-mounted] [/tmp/TestMountPropagated2876723755/001/dir1]
         |--(4295158374 -> 5808 / 1218) (7:9) [xfs] [/tmp/TestMountPropagated2876723755/001/dir1-bind-mounted/test-drive] [/]
      |--(4294967813 -> 485 / 181) (7:1) [squashfs] [/snap/bare/5] [/]
      |--(4294967830 -> 502 / 198) (7:17) [squashfs] [/snap/gtk-common-themes/1535] [/]
      |--(4294967819 -> 491 / 187) (7:6) [squashfs] [/snap/core22/2133] [/]
      |--(4294967815 -> 487 / 183) (7:2) [squashfs] [/snap/core18/2952] [/]
      |--(4294967962 -> 461 / 214) (0:26) [tmpfs] [/home] [/systemd/inaccessible/dir]
      |--(4294971873 -> 3384 / 1490) (0:72) [bpf] [/opt/sentinelone/ebpfs/bpf_mount] [/]
      |--(4294967820 -> 492 / 188) (7:7) [squashfs] [/snap/core24/1196] [/]
      |--(4294968022 -> 664 / 232) (252:2) [ext4] [/var/tmp] [/var/tmp/systemd-private-2c4d7490f6df492391429ca0f275ce22-systemd-oomd.service-ZUMqEZ/tmp]
      |--(4294967823 -> 495 / 191) (7:10) [squashfs] [/snap/fwupd/7611] [/]
      |--(4294967833 -> 505 / 201) (7:20) [squashfs] [/snap/snap-store/1113] [/]
      |--(4294967965 -> 464 / 215) (0:46) [proc] [/proc] [/]
         |--(4294967970 -> 484 / 220) (0:46) [proc] [/proc/irq] [/irq]
         |--(4294967967 -> 481 / 217) (0:46) [proc] [/proc/asound] [/asound]
         |--(4294967969 -> 483 / 219) (0:46) [proc] [/proc/fs] [/fs]
         |--(4294967971 -> 514 / 221) (0:26) [tmpfs] [/proc/kallsyms] [/systemd/inaccessible/reg]
         |--(4294967966 -> 465 / 216) (0:46) [proc] [/proc/acpi] [/acpi]
         |--(4294967985 -> 587 / 223) (0:26) [tmpfs] [/proc/kmsg] [/systemd/inaccessible/reg]
         |--(4294967975 -> 518 / 222) (0:26) [tmpfs] [/proc/kcore] [/systemd/inaccessible/reg]
         |--(4294967987 -> 589 / 225) (0:46) [proc] [/proc/mtrr] [/mtrr]
         |--(4294967968 -> 482 / 218) (0:46) [proc] [/proc/bus] [/bus]
         |--(4294967986 -> 588 / 224) (0:46) [proc] [/proc/latency_stats] [/latency_stats]
         |--(4294967989 -> 634 / 227) (0:46) [proc] [/proc/sys] [/sys]
         |--(4294967988 -> 590 / 226) (0:46) [proc] [/proc/scsi] [/scsi]
         |--(4294967990 -> 635 / 228) (0:46) [proc] [/proc/sysrq-trigger] [/sysrq-trigger]
      |--(4294967814 -> 486 / 182) (7:0) [squashfs] [/snap/core18/2947] [/]
      |--(4295244181 -> 6897 / 1379) (7:29) [xfs] [/tmp/TestMountEvent3985931949/001] [/]
      |--(4294967816 -> 488 / 184) (7:3) [squashfs] [/snap/core20/2599] [/]
      |--(4294971827 -> 3338 / 1468) (0:12) [tracefs] [/opt/sentinelone/mount] [/]
      |--(4294968019 -> 661 / 230) (252:2) [ext4] [/tmp] [/tmp/systemd-private-2c4d7490f6df492391429ca0f275ce22-systemd-oomd.service-kWc8Va/tmp]
      |--(4295095356 -> 3777 / 1193) (7:8) [squashfs] [/snap/google-cloud-cli/389] [/]
      |--(4295187293 -> 3092 / 426) (0:85) [fuse] [/tmp/.mount_Cursor1XnT4v] [/]
      |--(4294967831 -> 503 / 199) (7:18) [squashfs] [/snap/slack/212] [/]
      |--(4295174866 -> 6676 / 1239) (7:27) [squashfs] [/snap/google-cloud-cli/392] [/]
      |--(4295248702 -> 7283 / 1496) (252:2) [ext4] [/tmp/TestMountPropagated3149697396/001/dir1-bind-mounted] [/tmp/TestMountPropagated3149697396/001/dir1]
         |--(4295248703 -> 7284 / 1497) (7:30) [xfs] [/tmp/TestMountPropagated3149697396/001/dir1-bind-mounted/test-drive] [/]
      |--(4295248602 -> 7183 / 1434) (7:30) [xfs] [/tmp/TestMountPropagated3149697396/001/dir1/test-drive] [/]
      |--(4295240076 -> 3935 / 980) (7:22) [xfs] [/tmp/TestMountPropagated2250566632/001/dir1/test-drive] [/]
      |--(4294967835 -> 507 / 203) (7:21) [squashfs] [/snap/snap-store/1216] [/]
      |--(4294967818 -> 490 / 186) (7:5) [squashfs] [/snap/core20/2669] [/]
      |--(4295141007 -> 3499 / 1058) (0:75) [overlay] [/var/lib/docker/overlay2/ff27a99c01a1d00558d5a0f2eed65c5c02d2c0f39eddd6479841b3e5d848207e/merged] [/]
      |--(4294967839 -> 511 / 207) (259:3) [ext4] [/boot] [/]
         |--(4294967840 -> 512 / 208) (259:2) [vfat] [/boot/efi] [/]
      |--(4295240174 -> 6775 / 1340) (252:2) [ext4] [/tmp/TestMountPropagated2250566632/001/dir1-bind-mounted] [/tmp/TestMountPropagated2250566632/001/dir1]
         |--(4295240175 -> 6776 / 1341) (7:22) [xfs] [/tmp/TestMountPropagated2250566632/001/dir1-bind-mounted/test-drive] [/]
      |--(4294967832 -> 504 / 200) (7:19) [squashfs] [/snap/slack/216] [/]
      |--(4294967824 -> 496 / 192) (7:11) [squashfs] [/snap/gnome-3-34-1804/93] [/]
      |--(4295158275 -> 3923 / 1042) (7:9) [xfs] [/tmp/TestMountPropagated2876723755/001/dir1/test-drive] [/]
      |--(4294967836 -> 508 / 204) (7:23) [squashfs] [/snap/snapd/25202] [/]
      |--(4294967837 -> 509 / 205) (7:24) [squashfs] [/snap/snapd-desktop-integration/253] [/]
      |--(4295140620 -> 3232 / 1018) (0:73) [overlay] [/var/lib/docker/overlay2/3b4fbeeef45c73ed5952e13e6c7274d4596d7e2d564860ffb81c238884f7c8a7/merged] [/]
   |--*(4295244180 -> 6896 / 0) (7:29) [xfs] [] [/]
   |--*(4294967653 -> 325 / 0) (7:0) [squashfs] [] [/]
```

## Building & running:

```bash
go build .
sudo ./list-all-mounts
```