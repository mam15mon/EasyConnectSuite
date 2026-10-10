# UniVPN protocol port

This package ports the observed UniVPN protocol and the experimental OpenConnect
implementation from Bruce-lhq/univpn-connect, revision
037acad1b5101c756f5583a3aa8effaa260a81db:

https://github.com/Bruce-lhq/univpn-connect/tree/037acad1b5101c756f5583a3aa8effaa260a81db/native/openconnect/patches

Reference implementation copyright: (C) 2026 UniVPN Connect contributors.
The Go port retains the GNU Lesser General Public License, version 2.1 or later.
The complete version 2.1 license is included in LICENSE. It is provided without
warranty, as described in that license.

The MIT license of the upstream Python management layer does not relicense this
protocol implementation. This package does not include or invoke a native
OpenConnect binary.
