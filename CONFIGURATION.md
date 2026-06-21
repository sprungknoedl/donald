# Configuring donald

This covers all flags, custom targets, encryption, uploads, the archive metadata, and the
default collection paths. For an overview of what donald does, see the
[README](README.md).

## Flags

```text
$ donald -h
  -c string
        Add custom collection paths (one entry per line). See targets/example.quack.
  -od string
        Directory the zip archive is created in. (default ".")
  -of string
        Name of the zip archive created. (default "<hostname>-<timestamp>.zip")
  -raw
        Use raw NTFS access. Windows only. (default true)
  -replace-paths
        Replace the default collection paths with those given via -c.
  -root value
        Search root path(s), traversed in order. (default "/" or "C:\")
  -sftp-addr string
        SFTP server address.
  -sftp-dir string
        Output directory on the SFTP server. (default ".")
  -sftp-file string
        Name of the archive on the SFTP server. (default "<hostname>-<timestamp>.zip")
  -sftp-pass string
        SFTP password.
  -sftp-user string
        SFTP username.
  -zip-level int
        Zip compression level: 0 = store (no compression), 1 (fastest) .. 9 (smallest).
        If unset, the standard Deflate default is used. (default: standard Deflate)
  -zip-pass string
        Password for the output archive. If set, the archive is AES-256 encrypted.
```

## Examples

```text
# Standard collection
donald.exe    # Windows
./donald      # Linux / macOS

# Store output in a specific folder
donald.exe -od "C:\Temp\LRData"
donald.exe -od LRData

# Custom folder and filename
donald -od data -of important-data.zip

# Send data to an SFTP server
donald.exe -sftp-user username -sftp-pass password -sftp-addr 8.8.8.8
```

## Compression level

`-zip-level` trades CPU/time for archive size. `0` stores files uncompressed (zip `Store`
method — fastest, and ideal for already-compressed evidence like `.evtx` or browser caches),
`1` (fastest) to `9` (smallest) selects the Deflate level, and leaving the flag unset keeps
today's standard Deflate default.

```sh
donald -zip-level 0    # no compression, fastest
donald -zip-level 9    # maximum compression, smallest archive
```

It applies to both the normal and raw-NTFS collection paths and composes with `-zip-pass`
(files are compressed, then encrypted). It does not affect the per-file digests recorded in
the manifest, which are taken over the source bytes.

## Encrypted output

By default the archive is a plain `.zip`. Pass `-zip-pass` to encrypt it on the host
before it leaves, using per-entry WinZip AES-256 (file contents are encrypted, filenames
stay visible). The same encrypted file goes everywhere: local disk, SFTP, and Dagobert.

```sh
donald -zip-pass 'S3cret!'
```

Open the archive with any AES-zip-capable tool (7-Zip, WinZip, Keka) using the password.
For Dagobert, the password is sent in the upload so it can decrypt the evidence; for SFTP,
share it out-of-band.

> A password passed on the command line is visible in the host process list and shell
> history, same as `-sftp-pass`.

## Custom targets

A target is a rule for which paths to collect. Provide your own with a tab-delimited
`.quack` file via `-c`:

```text
donald.exe -c custom.quack -replace-paths=true    # use only your paths
donald     -c custom.quack -replace-paths=false   # your paths plus the defaults
```

`targets/example.quack` in the repository is the full documented template.

### Quack format

Tab-delimited `<type>\t<pattern>`, one rule per line. Lines starting with `#` are ignored.
You must put a tab between the type and the pattern. Spaces won't work as the separator,
though spaces are allowed inside the pattern itself.

All matching is case-insensitive. Use the path separator of the target OS: `\` for
Windows, `/` for macOS and Linux.

There are four pattern types:

* **static** is an exact path. It's the fastest, so use it whenever you know the full
  path.
  ```
  static	C:\Windows\System32\config\SAM
  ```
* **glob** uses [gobwas/glob](https://github.com/gobwas/glob) patterns (`**`, `*`, char
  classes). It's faster than regex, so prefer it for matching by name or extension, or for
  grabbing a folder recursively.
  ```
  glob	C:\Users\*\ntuser.dat            # NTUSER.DAT from every user
  glob	C:\**\malware.exe                # malware.exe in any folder
  glob	**\*.lnk                         # all .lnk files
  ```
* **regex** uses Go regular expressions, matched against the full path. It's the slowest
  option, so use it only for patterns globbing can't express.
  ```
  regex	C:\[0-9]+.exe                    # numeric-only exe in C:\
  ```
* **force** works like static, but collects the file even if directory enumeration never
  sees it. Use it for alternate data streams and special files.
  ```
  force	C:\$Extend\$UsnJrnl:$J
  ```

### KAPE targets

donald can also consume [KAPE](https://github.com/EricZimmerman/KapeFiles) `.tkape`
target files:

```text
donald.exe -kt <TargetName> -kf KapeFiles
```

`-kf` is the directory of KAPE files (default `KapeFiles`). Nested `.tkape` references are
resolved automatically. Module (`.mkape`) files are parsed but not executed.

## Archive metadata (`_donald/`)

Every collecting run adds a `_donald/` folder to the archive automatically.

### `manifest.jsonl`

One JSON object per line, in three record types:

* **`file`**, one per collection attempt. A `status:"collected"` record carries the zip
  `entry` name, `size` in bytes, `source` (`match` or `force`), and `sha256`/`md5` digests
  of the file's source bytes. The digests are computed before compression and encryption,
  so they verify against both the original and the extracted file. A `status:"error"`
  record carries an `error` message and no digests. This is the only place a file that
  matched but failed to copy gets recorded, since it's absent from the archive.
* **`dir_skipped`**, one per directory the traversal couldn't enumerate (permission
  denied, locked). This is the main reason whole subtrees go uncollected.
* **`summary`**, a final line with host, version, targets, roots, counts
  (`scanned`/`matched`/`collected`/`errors`/`dirs_skipped`/`bytes_total`), timestamps,
  and duration.

```sh
jq 'select(.type=="file" and .status=="error")' _donald/manifest.jsonl  # what failed?
jq 'select(.type=="dir_skipped")'               _donald/manifest.jsonl  # subtrees missed
jq 'select(.type=="summary")'                   _donald/manifest.jsonl  # outcome
```

### `collection.log`

A verbatim copy of the console output from the traverse and collect stages. Upload and
cleanup run after the archive is sealed, so their output isn't included. The manifest
`summary` is the durable record of the outcome.

### `sha256sums.txt` / `md5sums.txt`

The per-file digests in standard coreutils format. Verify the whole evidence tree after
extracting:

```sh
cd extracted/
sha256sum -c _donald/sha256sums.txt
md5sum   -c _donald/md5sums.txt
```

## Default collection paths

All paths are case-insensitive. Extend them with `-c` (see above) or open a pull request.

### Windows

System Root (e.g. `C:\Windows`):

* `Windows\Tasks\**`
* `Windows\Prefetch\**`
* `Windows\System32\sru\**`
* `Windows\System32\winevt\Logs\**`
* `Windows\System32\Tasks\**`
* `Windows\System32\Logfiles\W3SVC1\**`
* `Windows\Appcompat\Programs\**`
* `Windows\SchedLgU.txt`
* `Windows\inf\setupapi.dev.log`
* `Windows\System32\drivers\etc\hosts`
* `Windows\System32\config\SAM`
* `Windows\System32\config\SOFTWARE`
* `Windows\System32\config\SECURITY`
* `Windows\System32\config\SAM.LOG1`
* `Windows\System32\config\SOFTWARE.LOG1`
* `Windows\System32\config\SECURITY.LOG1`
* `Windows\System32\config\SAM.LOG2`
* `Windows\System32\config\SOFTWARE.LOG2`
* `Windows\System32\config\SECURITY.LOG2`

Program Data (e.g. `C:\ProgramData`):

* `ProgramData\Microsoft\Windows\Start Menu\Programs\Startup\**`

Drive Root (e.g. `C:`):

* `$Recycle.Bin\**\$I*`
* `$Recycle.Bin\$I*`
* `$LogFile`
* `$MFT`

User Profiles (e.g. `C:\Users\*`):

* `Users\*\NTUSER.DAT`
* `Users\*\NTUSER.DAT.LOG1`
* `Users\*\NTUSER.DAT.LOG2`
* `Users\*\AppData\Local\Microsoft\Windows\UsrClass.dat`
* `Users\*\AppData\Local\Microsoft\Windows\UsrClass.dat.LOG1`
* `Users\*\AppData\Local\Microsoft\Windows\UsrClass.dat.LOG2`
* `Users\*\AppData\Local\Google\Chrome\User Data\Default\History`
* `Users\*\AppData\Local\Microsoft\Edge\User Data\Default\History`
* `Users\*\AppData\Roaming\Microsoft\Windows\PowerShell\PSReadline\ConsoleHost_history.txt`
* `Users\*\AppData\Roaming\Microsoft\Windows\Recent\**`
* `Users\*\AppData\Local\Microsoft\Windows\WebCache\**`
* `Users\*\AppData\Roaming\Microsoft\Windows\Recent\AutomaticDestinations\**`
* `Users\*\AppData\Roaming\Mozilla\Firefox\Profiles\**`
* `Users\*\AppData\Local\ConnectedDevicesPlatform\**`
* `Users\*\AppData\Local\Microsoft\Windows\Explorer\**`

### macOS

> Modern macOS prompts per-application for access to sensitive locations. Grant Full Disk
> Access to the donald binary and its parent process (e.g. Terminal) in System Settings.

System paths:

* `/etc/hosts.allow`
* `/etc/hosts.deny`
* `/etc/hosts`
* `/etc/passwd`
* `/etc/group`
* `/etc/rc.d/**`
* `/var/log/**`
* `/private/etc/rc.d/**`
* `/private/etc/hosts.allow`
* `/private/etc/hosts.deny`
* `/private/etc/hosts`
* `/private/etc/passwd`
* `/private/etc/group`
* `/private/var/log/**`
* `/System/Library/StartupItems/**`
* `/System/Library/LaunchAgents/**`
* `/System/Library/LaunchDaemons/**`
* `/Library/StartupItems/**`
* `/Library/LaunchAgents/**`
* `/Library/LaunchDaemons/**`
* `/.fseventsd/**`

Library paths:

* `**/Library/*Support/Google/Chrome/Default/*`
* `**/Library/*Support/Google/Chrome/Default/History*`
* `**/Library/*Support/Google/Chrome/Default/Cookies*`
* `**/Library/*Support/Google/Chrome/Default/Bookmarks*`
* `**/Library/*Support/Google/Chrome/Default/Extensions/**`
* `**/Library/*Support/Google/Chrome/Default/Extensions/Last*`
* `**/Library/*Support/Google/Chrome/Default/Extensions/Shortcuts*`
* `**/Library/*Support/Google/Chrome/Default/Extensions/Top*`
* `**/Library/*Support/Google/Chrome/Default/Extensions/Visited*`

User paths:

* `/root/.*history`
* `/Users/*/.*history`

Other paths:

* `**/places.sqlite*`
* `**/downloads.sqlite*`

### Linux

System paths:

* `/etc/hosts.allow`
* `/etc/hosts.deny`
* `/etc/hosts`
* `/etc/passwd`
* `/etc/group`
* `/etc/crontab`
* `/etc/cron.allow`
* `/etc/cron.deny`
* `/etc/anacrontab`
* `/etc/apt/sources.list`
* `/etc/apt/trusted.gpg`
* `/etc/apt/trustdb.gpg`
* `/etc/resolv.conf`
* `/etc/fstab`
* `/etc/issues`
* `/etc/issues.net`
* `/etc/insserv.conf`
* `/etc/localtime`
* `/etc/timezone`
* `/etc/pam.conf`
* `/etc/rsyslog.conf`
* `/etc/xinetd.conf`
* `/etc/netgroup`
* `/etc/nsswitch.conf`
* `/etc/ntp.conf`
* `/etc/yum.conf`
* `/etc/chrony.conf`
* `/etc/chrony`
* `/etc/sudoers`
* `/etc/logrotate.conf`
* `/etc/environment`
* `/etc/hostname`
* `/etc/host.conf`
* `/etc/machine-id`
* `/etc/screen-rc`
* `/etc/rc.d/**`
* `/etc/cron.daily/**`
* `/etc/cron.hourly/**`
* `/etc/cron.weekly/**`
* `/etc/cron.monthly/**`
* `/etc/modprobe.d/**`
* `/etc/modprobe-load.d/**`
* `/etc/*-release`
* `/etc/pam.d/**`
* `/etc/rsyslog.d/**`
* `/etc/yum.repos.d/**`
* `/etc/init.d/**`
* `/etc/systemd.d/**`
* `/etc/default/**`
* `/var/log/**`
* `/var/spool/at/**`
* `/var/spool/cron/**`
* `/var/spool/anacron/cron.daily`
* `/var/spool/anacron/cron.hourly`
* `/var/spool/anacron/cron.weekly`
* `/var/spool/anacron/cron.monthly`
* `/boot/grub/grub.cfg`
* `/boot/grub2/grub.cfg`
* `/sys/firmware/acpi/tables/DSDT`

User paths:

* `/root/.*history`
* `/root/.*rc`
* `/root/.*_logout`
* `/root/.ssh/config`
* `/root/.ssh/known_hosts`
* `/root/.ssh/authorized_keys`
* `/root/.selected_editor`
* `/root/.viminfo`
* `/root/.lesshist`
* `/root/.profile`
* `/home/*/.*history`
* `/home/*/.ssh/known_hosts`
* `/home/*/.ssh/config`
* `/home/*/.ssh/autorized_keys`
* `/home/*/.viminfo`
* `/home/*/.profile`
* `/home/*/.*rc`
* `/home/*/.*_logout`
* `/home/*/.selected_editor`
* `/home/*/.wget-hsts`
* `/home/*/.gitconfig`
* `/home/*/.mozilla/firefox/*.default*/**/*.sqlite*`
* `/home/*/.mozilla/firefox/*.default*/**/*.json`
* `/home/*/.mozilla/firefox/*.default*/**/*.txt`
* `/home/*/.mozilla/firefox/*.default*/**/*.db*`
* `/home/*/.config/google-chrome/Default/History*`
* `/home/*/.config/google-chrome/Default/Cookies*`
* `/home/*/.config/google-chrome/Default/Bookmarks*`
* `/home/*/.config/google-chrome/Default/Extensions/**`
* `/home/*/.config/google-chrome/Default/Last*`
* `/home/*/.config/google-chrome/Default/Shortcuts*`
* `/home/*/.config/google-chrome/Default/Top*`
* `/home/*/.config/google-chrome/Default/Visited*`
* `/home/*/.config/google-chrome/Default/Preferences*`
* `/home/*/.config/google-chrome/Default/Login Data*`
* `/home/*/.config/google-chrome/Default/Web Data*`
