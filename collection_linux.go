package main

import (
	"archive/zip"
)

func DefaulRootPaths() []string {
	return []string{
		"/",
	}
}

func DefaultCollection() []Matcher {
	return []Matcher{
		// == static matchers
		// Super user
		NewStaticMatcher("/root/.ssh/config"),
		NewStaticMatcher("/root/.ssh/known_hosts"),
		NewStaticMatcher("/root/.ssh/authorized_keys"),
		NewStaticMatcher("/root/.selected_editor"),
		NewStaticMatcher("/root/.viminfo"),
		NewStaticMatcher("/root/.lesshist"),
		NewStaticMatcher("/root/.profile"),
		NewStaticMatcher("/root/.selected_editor"),

		// Boot
		NewStaticMatcher("/boot/grub/grub.cfg"),
		NewStaticMatcher("/boot/grub2/grub.cfg"),

		// Sys
		NewStaticMatcher("/sys/firmware/acpi/tables/DSDT"),

		//etc
		NewStaticMatcher("/etc/hosts.allow"),
		NewStaticMatcher("/etc/hosts.deny"),
		NewStaticMatcher("/etc/hosts"),
		NewStaticMatcher("/etc/passwd"),
		NewStaticMatcher("/etc/group"),
		NewStaticMatcher("/etc/crontab"),
		NewStaticMatcher("/etc/cron.allow"),
		NewStaticMatcher("/etc/cron.deny"),
		NewStaticMatcher("/etc/anacrontab"),
		NewStaticMatcher("/var/spool/anacron/cron.daily"),
		NewStaticMatcher("/var/spool/anacron/cron.hourly"),
		NewStaticMatcher("/var/spool/anacron/cron.weekly"),
		NewStaticMatcher("/var/spool/anacron/cron.monthly"),
		NewStaticMatcher("/etc/apt/sources.list"),
		NewStaticMatcher("/etc/apt/trusted.gpg"),
		NewStaticMatcher("/etc/apt/trustdb.gpg"),
		NewStaticMatcher("/etc/resolv.conf"),
		NewStaticMatcher("/etc/fstab"),
		NewStaticMatcher("/etc/issues"),
		NewStaticMatcher("/etc/issues.net"),
		NewStaticMatcher("/etc/insserv.conf"),
		NewStaticMatcher("/etc/localtime"),
		NewStaticMatcher("/etc/timezone"),
		NewStaticMatcher("/etc/pam.conf"),
		NewStaticMatcher("/etc/rsyslog.conf"),
		NewStaticMatcher("/etc/xinetd.conf"),
		NewStaticMatcher("/etc/netgroup"),
		NewStaticMatcher("/etc/nsswitch.conf"),
		NewStaticMatcher("/etc/ntp.conf"),
		NewStaticMatcher("/etc/yum.conf"),
		NewStaticMatcher("/etc/chrony.conf"),
		NewStaticMatcher("/etc/chrony"),
		NewStaticMatcher("/etc/sudoers"),
		NewStaticMatcher("/etc/logrotate.conf"),
		NewStaticMatcher("/etc/environment"),
		NewStaticMatcher("/etc/hostname"),
		NewStaticMatcher("/etc/host.conf"),
		NewStaticMatcher("/etc/fstab"),
		NewStaticMatcher("/etc/machine-id"),
		NewStaticMatcher("/etc/screen-rc"),

		// == glob matchers
		// User profiles
		NewGlobMatcher("/home/*/.*history"),
		NewGlobMatcher("/home/*/.ssh/known_hosts"),
		NewGlobMatcher("/home/*/.ssh/config"),
		NewGlobMatcher("/home/*/.ssh/autorized_keys"),
		NewGlobMatcher("/home/*/.viminfo"),
		NewGlobMatcher("/home/*/.profile"),
		NewGlobMatcher("/home/*/.*rc"),
		NewGlobMatcher("/home/*/.*_logout"),
		NewGlobMatcher("/home/*/.selected_editor"),
		NewGlobMatcher("/home/*/.wget-hsts"),
		NewGlobMatcher("/home/*/.gitconfig"),

		// Firefox artifacts
		NewGlobMatcher("/home/*/.mozilla/firefox/*.default*/**/*.sqlite*"),
		NewGlobMatcher("/home/*/.mozilla/firefox/*.default*/**/*.json"),
		NewGlobMatcher("/home/*/.mozilla/firefox/*.default*/**/*.txt"),
		NewGlobMatcher("/home/*/.mozilla/firefox/*.default*/**/*.db*"),

		// Chrome artifacts
		NewGlobMatcher("/home/*/.config/google-chrome/Default/History*"),
		NewGlobMatcher("/home/*/.config/google-chrome/Default/Cookies*"),
		NewGlobMatcher("/home/*/.config/google-chrome/Default/Bookmarks*"),
		NewGlobMatcher("/home/*/.config/google-chrome/Default/Extensions/**"),
		NewGlobMatcher("/home/*/.config/google-chrome/Default/Last*"),
		NewGlobMatcher("/home/*/.config/google-chrome/Default/Shortcuts*"),
		NewGlobMatcher("/home/*/.config/google-chrome/Default/Top*"),
		NewGlobMatcher("/home/*/.config/google-chrome/Default/Visited*"),
		NewGlobMatcher("/home/*/.config/google-chrome/Default/Preferences*"),
		NewGlobMatcher("/home/*/.config/google-chrome/Default/Login Data*"),
		NewGlobMatcher("/home/*/.config/google-chrome/Default/Web Data*"),

		// Superuser profiles
		NewGlobMatcher("/root/.*history"),
		NewGlobMatcher("/root/.*rc"),
		NewGlobMatcher("/root/.*_logout"),

		// var
		NewGlobMatcher("/var/log/**"),
		NewGlobMatcher("/var/spool/at/**"),
		NewGlobMatcher("/var/spool/cron/**"),

		// etc
		NewGlobMatcher("/etc/rc.d/**"),
		NewGlobMatcher("/etc/cron.daily/**"),
		NewGlobMatcher("/etc/cron.hourly/**"),
		NewGlobMatcher("/etc/cron.weekly/**"),
		NewGlobMatcher("/etc/cron.monthly/**"),
		NewGlobMatcher("/etc/modprobe.d/**"),
		NewGlobMatcher("/etc/modprobe-load.d/**"),
		NewGlobMatcher("/etc/*-release"),
		NewGlobMatcher("/etc/pam.d/**"),
		NewGlobMatcher("/etc/rsyslog.d/**"),
		NewGlobMatcher("/etc/yum.repos.d/**"),
		NewGlobMatcher("/etc/init.d/**"),
		NewGlobMatcher("/etc/systemd.d/**"),
		NewGlobMatcher("/etc/default/**"),

		// == regexp matchers
		// nil
	}
}

func ForcedFiles() []string {
	return []string{}
}

func CollectFileRaw(cfg *Configuration, archive *zip.Writer, path string) error {
	return CollectFile(cfg, archive, path)
}
