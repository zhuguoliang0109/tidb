// Copyright 2019 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//go:build linux

package linux

import (
	"net"
	"syscall"

	"golang.org/x/exp/constraints"
	"golang.org/x/sys/unix"
)

func charsToString[T constraints.Integer](ca []T) string {
	s := make([]byte, len(ca))
	var lens int
	for ; lens < len(ca); lens++ {
		if ca[lens] == 0 {
			break
		}
		s[lens] = uint8(ca[lens])
	}
	return string(s[0:lens])
}

// OSVersion returns version info of operation system.
// e.g. Linux 4.15.0-45-generic.x86_64
func OSVersion() (osVersion string, err error) {
	var un syscall.Utsname
	err = syscall.Uname(&un)
	if err != nil {
		return
	}
	osVersion = charsToString(un.Sysname[:]) + " " + charsToString(un.Release[:]) + "." + charsToString(un.Machine[:])
	return
}

// SetAffinity sets cpu affinity.
func SetAffinity(cpus []int) error {
	var cpuSet unix.CPUSet
	cpuSet.Zero()
	for _, c := range cpus {
		cpuSet.Set(c)
	}
	return unix.SchedSetaffinity(unix.Getpid(), &cpuSet)
}

// GetSockUID gets the uid of the other end of the UNIX domain socket
func GetSockUID(uc net.UnixConn) (uid uint32, err error) {
	raw, err := uc.SyscallConn()
	if err != nil {
		return 0, err
	}

	var cred *unix.Ucred
	err = raw.Control(func(fd uintptr) {
		cred, err = unix.GetsockoptUcred(int(fd),
			unix.SOL_SOCKET,
			unix.SO_PEERCRED)
	})
	if err != nil {
		return 0, err
	}

	return cred.Uid, nil
}
