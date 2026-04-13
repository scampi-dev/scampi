---
title: mount
---

Manage filesystem mounts and `/etc/fstab` entries.

```scampi
posix.mount {
  src     = "198.51.100.2:/volume2/data"
  dest    = "/mnt/data"
  fs_type = posix.MountType.nfs
  opts    = "defaults,noatime"
  state   = posix.MountState.mounted
  desc    = "NAS data mount"
}
```

## Fields

| Field       | Type          | Required | Default              | Description                                       |
| ----------- | ------------- | :------: | -------------------- | ------------------------------------------------- |
| `src`       | string        |    ✓     |                      | Mount source — device or remote (`@std.nonempty`) |
| `dest`      | string        |    ✓     |                      | Mount point path (`@std.path(absolute=true)`)     |
| `fs_type`   | `MountType`   |    ✓     |                      | Filesystem type — see [below](#filesystem-types)  |
| `opts`      | string?       |          | `"defaults"`         | Mount options                                     |
| `state`     | `MountState?` |          | `MountState.mounted` | Desired state                                     |
| `desc`      | string?       |          |                      | Human-readable description                        |
| `on_change` | list\[Step]   |          |                      | Steps to trigger when this mount changes          |

## Filesystem types

`posix.MountType` is an enum:

| Value       | Notes              |
| ----------- | ------------------ |
| `nfs`       | Standard NFS       |
| `nfs4`      | NFSv4              |
| `cifs`      | SMB / CIFS         |
| `ext4`      | Local ext4 device  |
| `xfs`       | Local XFS device   |
| `btrfs`     | Local Btrfs device |
| `tmpfs`     | In-memory          |
| `glusterfs` | GlusterFS          |
| `ceph`      | Ceph               |

## States

`posix.MountState` is an enum:

| State                  | Fstab   | Mounted |
| ---------------------- | ------- | ------- |
| `MountState.mounted`   | present | yes     |
| `MountState.unmounted` | present | no      |
| `MountState.absent`    | removed | no      |

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

```scampi
std.deploy(name = "mounts", targets = [server]) {
  posix.pkg {
    packages = ["nfs-common"]
    state    = posix.PkgState.present
    source   = posix.pkg_system {}
    desc     = "NFS client tools"
  }

  posix.mount {
    src     = "198.51.100.2:/volume2/data"
    dest    = "/mnt/data"
    fs_type = posix.MountType.nfs
    opts    = "defaults,noatime"
    desc    = "NAS data mount"
  }
}
```

## Examples

### NFS mount

```scampi
posix.mount {
  src     = "nas.local:/exports/media"
  dest    = "/mnt/media"
  fs_type = posix.MountType.nfs
  opts    = "defaults,noatime,soft"
  desc    = "media library"
}
```

### CIFS/SMB mount

```scampi
posix.mount {
  src     = "//fileserver/shared"
  dest    = "/mnt/shared"
  fs_type = posix.MountType.cifs
  opts    = "credentials=/etc/smbcreds,uid=1000,gid=1000"
  desc    = "SMB file share"
}
```

### Remove a mount

```scampi
posix.mount {
  src     = "nas.local:/old-export"
  dest    = "/mnt/old"
  fs_type = posix.MountType.nfs
  state   = posix.MountState.absent
}
```
