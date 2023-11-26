# donald

donald — Live Response Collection tool by Thomas Kastner

## What is donald

Donald is a tool to collect forensic artifacts from hosts quickly, and securely and with minimal impact to the host. 

Donald is heavily inspired by [CYLR](https://github.com/orlikoski/CyLR). 

The main features are:

* Quick collection (it's really fast)
* Raw file collection process does not use Windows API
* Collection of key artifacts by default.
* Ability to specify custom targets for collection.
* Acquisition of special and in-use files, including alternate data streams, system files, and hidden files.
* Glob and regular expression patterns are available to specify custom targets.
* Data is collected into a zip file.
* Output can be uploaded directly to a SFTP server.

## SYNOPSIS

Below is the output of donald:

```text
$ donald -h
Usage of ./donald:
  -c string
        Add custom collection paths (one entry per line). NOTE: Please see CUSTOM_PATH_TEMPLATE.txt for an example.
  -od string
        Defines the directory that the zip archive will be created in. (default ".")
  -of string
        Defines the name of the zip archive created. (default "meiBook.local.zip")
  -raw
        Use raw NTFS access. Only supported on Windows. (default true)
  -replace-paths
        Replace the default collection paths with those specified via '-c FILE'.
  -root value
        Defines the search root path(s). If multiple root paths are given, they are traversed in order. (default "/")
  -sftp-addr string
        SFTP server address
  -sftp-dir string
        Defines the output directory on the SFTP server, as it may be a different location than the archive generated on disk. (default ".")
  -sftp-file string
        Defines the name of the zip archive created on the SFTP server. (default "meiBook.local.zip")
  -sftp-pass string
        SFTP password
  -sftp-user string
        SFTP username
```

## Default Collection Paths

All collection paths are case-insensitive. You can easily extend this list
through the use of patterns as shown in CUSTOM_PATH_TEMPLATE.txt or by opening
a pull request.

The standard list of collected artifacts is as follows.

### Windows

System Root (ie `C:\Windows`):

* `\Windows\Tasks\**`
* `\Windows\Prefetch\**`
* `\Windows\System32\sru\**`
* `\Windows\System32\winevt\Logs\**`
* `\Windows\System32\Tasks\**`
* `\Windows\System32\Logfiles\W3SVC1\**`
* `\Windows\Appcompat\Programs\**`
* `\Windows\SchedLgU.txt`
* `\Windows\inf\setupapi.dev.log`
* `\Windows\System32\drivers\etc\hosts`
* `\Windows\System32\config\SAM`
* `\Windows\System32\config\SOFTWARE`
* `\Windows\System32\config\SECURITY`
* `\Windows\System32\config\SOFTWARE`
* `\Windows\System32\config\SAM.LOG1`
* `\Windows\System32\config\SOFTWARE.LOG1`
* `\Windows\System32\config\SECURITY.LOG1`
* `\Windows\System32\config\SOFTWARE.LOG1`
* `\Windows\System32\config\SAM.LOG2`
* `\Windows\System32\config\SOFTWARE.LOG2`
* `\Windows\System32\config\SECURITY.LOG2`
* `\Windows\System32\config\SOFTWARE.LOG2`

Program Data (ie `C:\ProgramData`):

* `\ProgramData\Microsoft\Windows\Start Menu\Programs\Startup\**`

Drive Root (ie `C:`)

* `\$Recycle.Bin\**\$I*`
* `\$Recycle.Bin\$I*`
* `\$LogFile`
* `\$MFT`

User Profiles (ie `C:\Users\*`):

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

**Note**: Modern macOS systems have functionality that will prompt the user to
approve on a per-application basis, access to sensitive locations on a system.
This can be overridden by modifying the System Preferences to give the donald
binary and its parent process (such as Terminal) full disk access.

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

Libraries paths:

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

Other Paths:

* `**/places.sqlite*`
* `**/downloads.sqlite*`

### Linux

System Paths:

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
* `/etc/fstab`
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
* `/root/.selected_editor`
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

## DEPENDENCIES

In general: some kind of administrative rights on the target (root, sudo,
administrator,...).

Donald is a native binary that runs on Windows, Linux, and MacOS as
a a self-contained executable. No external dependencies should be required (if so, please file a bug report).

## EXAMPLES

### Standard collection

```text
donald.exe
```

### Linux/macOS collection

```text
./donald
```

### Collect artifacts and store data in "C:\Temp\LRData"

```text
donald.exe -od "C:\Temp\LRData"
```

### Collect artifacts and store data in ".\LRData"

```text
donald.exe -od LRData
```

### Collect artifacts and send data to SFTP server 8.8.8.8

```text
donald.exe -sftp-user username -sftp-pass password -sftp-addr 8.8.8.8
```

### Collect to another folder and filename

```text
donald -od data -of important-data.zip
```

### Collect USN $J Journal
Collect a custom list of artifacts from a file containing paths
The sample `custom.txt` requires a **tab delimiter** between pattern
definition and pattern. Lines starting with `#` will be ignored:

```text
# Static paths are fixed, case-insensitive paths to compare
# against files found on a system. This is the fastest search
# method available, please use when possible.
#
static  C:\Windows\System32\Config\SAM
#
# Glob paths leverage glob patterns specified at
# `https://github.com/dazinator/DotNet.Glob`. This is faster than RegEx and
# should be favored unless more complex patterns are required. Useful for
# scanning for files by name or extension recursively. Also useful for
# collecting a folder recursively.
#
glob    **\malware.exe
#
# Regex paths leverage the Go Regex capabilities and will search for
# specified patterns across accessible files. This is the slowest option and
# should be saved for unique use cases that are not supported by globbing.
#
regex   .*\Windows\Temp\[a-z]{8}\+*
```

This can then be supplied to donald for a custom collection of just these paths:

```text
donald.exe -c custom.txt --replace-paths=true
```

### Collection of custom paths in addition to the default paths

```text
donald -c custom.txt --replace-paths=false
```

## Custom collection paths

Donald allows for the specification of custom collection paths with the use of
a configuration file provided after `-c` at the command line. A summary of the format is below, though full details are available within the `CUSTOM_PATH_TEMPLATE.txt` provided in the repository.

The custom collection path file allows for the specification of files to collect
from a target system. The format is tab-delimited, where the first field is a
pattern type indicator and the second field is the pattern to collect.

* **NOTE**: As previously mentioned, all collection paths are case-insensitive.
* **NOTE**: The path specifier needs to match the platform you are collecting
  from. For Windows, it must be `\`, and `/` for macOS and Linux.
* **NOTE**: You must use tabs to delimit the patterns. Spaces will not
  work. This means that spaces are allowed in the second field containing
  pattern content

### Pattern Types

There are 4 pattern types, summarized below:

* static
  * This format allows for the specification of a specific file at a known path.
  * This is the fastest pattern type, as it is performing a string comparison.
  * Example: `static    C:\Windows\System32\config\SAM`
* glob
  * This format allows the specification of basic patterns. Most commonly used
    to collect the contents of a folder, even recursively. Has a few common
    implementations, demonstrated in the examples below.
  * While not as fast as static paths, it allows for some common pattern
    matching and is faster than leveraging regular expressions.
  * Example: `glob    C:\Users\*\ntuser.dat` - collects the NTUser.dat from each user.
  * Example: `glob    C:\**\malware.exe` - collects files named `malware.exe`
    regardless of what folder they are in, recursively.
  * Example: `glob    C:\Users\*\AppData\Microsoft\Windows\Recent\*.lnk` -
    collects all files ending with `.lnk`
  * Example: `glob    **\*malware*` - collects all files recursively.
* regex
  * Allows the specification of advanced patterns through Go's regular
    expression implementation.
  * Example: `regex    C:\[0-9]+.exe` - collect all numeric-only executables in
    the root of the `C:\` drive.
* force
  * Same as the static option, though will attempt collection even if the file
    is not identified in the file enumeration process.
  * This is useful in the collection of alternate data streams and special
    files not generally exposed to directory traversal functions.
  * Example: `force    C:\$Extend\$UsnJrnl:$J`

## Building

Donald binaries are available for download, and prebuilt for use on macOS, Linux, and
Windows operating systems. The following operating systems were tested:

* Windows 11 Pro 22H2
* macOS 14.1.1

**Please help to test Donald on more platforms**

To build donald yourself, follow the below steps:

1. Install Go on your platform
1. Clone this repository
1. Run `go get` to fetch all dependencies
1. Run `go build` to build a binary for your platform

## AUTHORS

* [Thomas Kastner](https://github.com/sprungknoedl)

Special thanks to the original CyLR authors:

* [Jason Yegge](https://github.com/Lansatac)
* [Alan Orlikoski](https://github.com/rough007)
