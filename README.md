# donald

donald is a live-response forensic collection tool by Thomas Kastner, inspired by
[CYLR](https://github.com/orlikoski/CyLR).

It pulls forensic artifacts off a running host quickly and with little impact, then packs
them into a single zip archive. On Windows it reads files through raw NTFS access, so it
can collect locked, in-use, and special system files that the Windows API won't hand over.

## What it does

* **Collects fast** using sensible default targets for each OS.
* **Raw NTFS access on Windows.** It bypasses the Windows API to grab locked and in-use
  files, alternate data streams, system files like `$MFT` and `$LogFile`, and hidden
  files.
* **Single binary, no dependencies.** It runs on Windows, Linux, and macOS.
* **Custom targets.** Add your own paths with glob, regex, static, or force patterns, or
  point it at [KAPE](https://github.com/EricZimmerman/KapeFiles) target files.
* **Zip output**, optionally encrypted with AES-256.
* **Direct upload** to an SFTP server or a
  [Dagobert](https://github.com/sprungknoedl/dagobert) case-management instance.
* **Self-documenting archives.** Every archive carries a `_donald/` folder with a
  manifest, a log, and checksums of everything collected.
* **Hash sidecar.** Every archive ships with a `<archive>.sha256` companion so the
  receiver can verify the file arrived intact with `sha256sum -c`.

## How it works

donald runs a fixed four-stage pipeline:

1. **Traverse.** Walk the filesystem and find files matching the collection targets.
2. **Collect.** Copy each match into a zip archive.
3. **Upload.** Optionally send the archive to SFTP and/or Dagobert.
4. **Cleanup.** Optionally delete the local archive after upload.

It ships with default targets for Windows, macOS, and Linux: registry hives, event logs,
browser and shell history, cron and launch items, and more. You can extend or replace
them. See [CONFIGURATION.md](CONFIGURATION.md).

## Quickstart

You need administrative rights on the target (Administrator on Windows, root/sudo
elsewhere).

```sh
# Windows: collect default artifacts into the current folder
donald.exe

# Linux / macOS
./donald

# Collect into a specific folder
donald.exe -od "C:\Temp\LRData"

# Encrypt the output archive
donald -zip-pass 'S3cret!'

# Collect and upload to an SFTP server
donald.exe -sftp-user user -sftp-pass pass -sftp-addr 8.8.8.8
```

The archive is named `<hostname>-<timestamp>.zip` by default.

On macOS, sensitive locations require Full Disk Access. Grant it to the donald binary and
its parent process (such as Terminal) in System Settings.

## Inside the archive

Alongside the collected evidence, each archive holds a `_donald/` folder that documents the run:

```
<hostname>-<timestamp>.zip
├── _donald/
│   ├── manifest.jsonl     # one JSON record per file + a summary
│   ├── collection.log     # console transcript of the collection
│   ├── sha256sums.txt      # SHA-256 of every collected file
│   └── md5sums.txt         # MD5 of every collected file
└── ...                    # the collected evidence
```

After extracting, you can run `sha256sum` or `md5sum` against `sha256sums.txt` or
`md5sums.txt` to confirm the contents are intact.

## Verifying the archive

Next to every archive donald writes a `<archive>.sha256` sidecar — the SHA-256 of the
final archive bytes in `sha256sum` format. It attests to the *stored* container, so it
verifies the file in transit (SFTP, removable media, Dagobert) without opening it:

```sh
# in the directory holding both files
sha256sum -c <hostname>-<timestamp>.zip.sha256
# => <hostname>-<timestamp>.zip: OK
```

The digest is taken over the bytes on disk, so when `-zip-pass` is set it covers the
encrypted archive — checkable without the password. On SFTP the sidecar is uploaded
alongside the archive; on Dagobert the digest is sent as a `Hash` field.

## Build

```sh
go build -o donald        # build for the current platform
make all                  # cross-compile all platforms/architectures
```

Prebuilt binaries are available for Windows, Linux, and macOS.

## Configuration

All flags, custom targets, encryption, uploads, and the full default collection paths are
documented in **[CONFIGURATION.md](CONFIGURATION.md)**.

## Authors

* [Thomas Kastner](https://github.com/sprungknoedl)

Special thanks to the original CyLR authors:

* [Jason Yegge](https://github.com/Lansatac)
* [Alan Orlikoski](https://github.com/rough007)
