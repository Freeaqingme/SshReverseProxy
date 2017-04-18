#!/bin/sh
#
# $FreeBSD$

# PROVIDE: sshreverseproxy
# REQUIRE: LOGIN unbound
# KEYWORD: shutdown

. /etc/rc.subr

name=sshreverseproxy
desc="sshreverseproxy startup script"
rcvar=sshreverseproxy_enable

load_rc_config ${name}

sshreverseproxy_enable=${sshreverseproxy_enable:-NO}
sshreverseproxy_loglevel=${sshreverseproxy_loglevel:-debug}
sshreverseproxy_config=${sshreverseproxy_config:-/usr/local/etc/sshreverseproxy/sshreverseproxy.conf}

command="/usr/sbin/daemon"
procname=/usr/bin/sshreverseproxy
command_args="-c -f -- ${procname} --config ${sshreverseproxy_config} --loglevel ${sshreverseproxy_loglevel} daemon start"

run_rc_command "$1"

