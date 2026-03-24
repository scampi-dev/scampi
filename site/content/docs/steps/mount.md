---
title: mount
---

Manage filesystem mounts and `/etc/fstab` entries.

```python
mount(
    src="10.10.2.2:/volume2/data",
    dest="/mnt/data",
    type="nfs",
    opts="defaults,noatime",
    state="mounted",
    desc="NAS data mount",
)
```

## Fields

| Field   | Required | Default    | Description                             |
| ------- | -------- | ---------- | --------------------------------------- |
| `src`   | yes      |            | Mount source (device or remote)         |
| `dest`  | yes      |            | Mount point path (must be absolute)     |
| `type`  | yes      |            | Filesystem type (nfs, cifs, ext4, etc.) |
| `opts`  | no       | `defaults` | Mount options                           |
| `state` | no       | `mounted`  | Desired state                           |
| `desc`  | no       |            | Human-readable description              |

## States

| State       | Fstab   | Mounted |
| ----------- | ------- | ------- |
| `mounted`   | present | yes     |
| `unmounted` | present | no      |
| `absent`    | removed | no      |

## Network filesystem tools

Network mounts require helper tools on the target. The mount step checks
for these at plan time and gives a targeted diagnostic if they're missing:

| Type           | Required package                                   |
| -------------- | -------------------------------------------------- |
| `nfs` / `nfs4` | `nfs-common` (Debian/Ubuntu) or `nfs-utils` (RHEL) |
| `cifs`         | `cifs-utils`                                       |
| `ceph`         | `ceph-common`                                      |
| `glusterfs`    | `glusterfs-client`                                 |

Add a `pkg` step before the mount to ensure the tools are installed:

```python
deploy(
    name="mounts",
    targets=["server"],
    steps=[
        pkg(
            packages=["nfs-common"],
            state="present",
            source=system(),
            desc="NFS client tools",
        ),
        mount(
            src="10.10.2.2:/volume2/data",
            dest="/mnt/data",
            type="nfs",
            opts="defaults,noatime",
            desc="NAS data mount",
        ),
    ],
)
```

## Examples

### NFS mount

```python
mount(
    src="nas.local:/exports/media",
    dest="/mnt/media",
    type="nfs",
    opts="defaults,noatime,soft",
    desc="media library",
)
```

### CIFS/SMB mount

```python
mount(
    src="//fileserver/shared",
    dest="/mnt/shared",
    type="cifs",
    opts="credentials=/etc/smbcreds,uid=1000,gid=1000",
    desc="SMB file share",
)
```

### Remove a mount

```python
mount(
    src="nas.local:/old-export",
    dest="/mnt/old",
    type="nfs",
    state="absent",
)
```
