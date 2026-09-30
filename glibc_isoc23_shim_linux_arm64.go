//go:build linux && arm64

package main

/*
#include <stdlib.h>

// __isoc23_strtol/__isoc23_strtoul: the apt-packaged aarch64-linux-gnu-gcc's
// own glibc (2.35 on Ubuntu 22.04) doesn't define these symbols, but
// go-fitz's prebuilt libmupdfthird_linux_arm64.a bundles a HarfBuzz object
// (hb-number.o) that calls them directly -- see compile-linux.sh's own
// comment on this build step for the full story. They only exist in
// glibc >= 2.38 (added for ISO C23's 0b/0B binary-literal parsing in
// strtol/strtoul); this app never needs that new behavior, so a thin
// wrapper around the ordinary strtol/strtoul this toolchain's own glibc
// already provides is enough to satisfy the linker -- without switching to
// a newer-glibc-targeting cross-toolchain, which would also raise this
// binary's own minimum glibc requirement on whatever machine runs it.
long __isoc23_strtol(const char *nptr, char **endptr, int base) {
	return strtol(nptr, endptr, base);
}

unsigned long __isoc23_strtoul(const char *nptr, char **endptr, int base) {
	return strtoul(nptr, endptr, base);
}
*/
import "C"
