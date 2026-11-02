// utmp accounting for PAM-spawned shells. pam_unix's session component only
// syslog's open/close nowadays (see modules/pam_unix/pam_unix_sess.c upstream);
// it no longer writes utmp/wtmp. login(1) and sshd write those themselves, so
// we do the same here once a real PTY exists - OpenSession runs before any
// shell is spawned, so there is no TTY to record at authenticate time.
//
// One USER_PROCESS entry per spawned shell (keyed by pts line + pid), cleared
// to DEAD_PROCESS when that shell exits or the login tears down. who(1)/w(1)
// read /var/run/utmp, so this is what makes a tuios-web trainee show up as
// logged in on the host.

package main

/*
#cgo CFLAGS: -D_GNU_SOURCE
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <sys/time.h>
#include <utmpx.h>
#include <paths.h>

// tuios_utmp_login writes a USER_PROCESS record for line (e.g. "pts/3", no
// /dev/ prefix) to utmp and appends the same record to wtmp.
static void tuios_utmp_login(const char *user, const char *line, const char *host, int pid) {
	struct utmpx ut;
	struct timeval tv;

	memset(&ut, 0, sizeof(ut));
	ut.ut_type = USER_PROCESS;
	ut.ut_pid = pid;
	strncpy(ut.ut_user, user, sizeof(ut.ut_user) - 1);
	strncpy(ut.ut_line, line, sizeof(ut.ut_line) - 1);
	// ut_id uniquely tags the slot; for pts/N use N (same convention as
	// login(1) / The Linux Programming Interface §40.2).
	if (strncmp(line, "pts/", 4) == 0) {
		strncpy(ut.ut_id, line + 4, sizeof(ut.ut_id) - 1);
	} else {
		strncpy(ut.ut_id, line, sizeof(ut.ut_id) - 1);
	}
	if (host != NULL && host[0] != '\0') {
		strncpy(ut.ut_host, host, sizeof(ut.ut_host) - 1);
	}
	gettimeofday(&tv, NULL);
	ut.ut_tv.tv_sec = tv.tv_sec;
	ut.ut_tv.tv_usec = tv.tv_usec;

	setutxent();
	pututxline(&ut);
	updwtmpx(_PATH_WTMP, &ut);
	endutxent();
}

// tuios_utmp_logout marks the utmp slot for line as DEAD_PROCESS and appends
// a matching wtmp logout record. Looks up the live entry by line first so we
// clear the right slot even if another process reused the pid.
static void tuios_utmp_logout(const char *line, int pid) {
	struct utmpx key, *found, ut;
	struct timeval tv;

	memset(&key, 0, sizeof(key));
	strncpy(key.ut_line, line, sizeof(key.ut_line) - 1);

	setutxent();
	found = getutxline(&key);
	if (found != NULL) {
		ut = *found;
	} else {
		memset(&ut, 0, sizeof(ut));
		ut.ut_pid = pid;
		strncpy(ut.ut_line, line, sizeof(ut.ut_line) - 1);
		if (strncmp(line, "pts/", 4) == 0) {
			strncpy(ut.ut_id, line + 4, sizeof(ut.ut_id) - 1);
		} else {
			strncpy(ut.ut_id, line, sizeof(ut.ut_id) - 1);
		}
	}
	ut.ut_type = DEAD_PROCESS;
	memset(ut.ut_user, 0, sizeof(ut.ut_user));
	memset(ut.ut_host, 0, sizeof(ut.ut_host));
	gettimeofday(&tv, NULL);
	ut.ut_tv.tv_sec = tv.tv_sec;
	ut.ut_tv.tv_usec = tv.tv_usec;

	pututxline(&ut);
	updwtmpx(_PATH_WTMP, &ut);
	endutxent();
}
*/
import "C"

import (
	"unsafe"
)

func utmpLogin(user, line, host string, pid int) {
	cUser := C.CString(user)
	cLine := C.CString(line)
	cHost := C.CString(host)
	defer C.free(unsafe.Pointer(cUser))
	defer C.free(unsafe.Pointer(cLine))
	defer C.free(unsafe.Pointer(cHost))
	C.tuios_utmp_login(cUser, cLine, cHost, C.int(pid))
}

func utmpLogout(line string, pid int) {
	cLine := C.CString(line)
	defer C.free(unsafe.Pointer(cLine))
	C.tuios_utmp_logout(cLine, C.int(pid))
}
