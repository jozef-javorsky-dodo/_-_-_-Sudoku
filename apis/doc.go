/*
Copyright (C) 2026 by saba <contact me via issue>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.

In addition, no derivative work may use the name or imply association
with this application without prior consent.
*/
// Package apis exposes the Sudoku tunnel (HTTP mask + Sudoku obfuscation + AEAD) as a small Go API.
// It supports both pure Sudoku downlink and the bandwidth-optimized packed downlink, plus UDP-over-TCP (UoT),
// so the same primitives used by the CLI can be embedded by other projects.
//
// Key entry points:
//   - ProtocolConfig / DefaultConfig: describe all required parameters.
//   - Dial: client-side helper that connects to a Sudoku server and sends the target address.
//   - DialUDPOverTCP: client-side helper that primes a UoT tunnel.
//   - ServerHandshake: server-side helper that upgrades an accepted TCP connection and returns
//     the decrypted tunnel plus the requested target address (TCP mode).
//   - ServerHandshakeFlexible: server-side helper that upgrades connections and lets callers
//     detect UoT or read the target address themselves.
//   - HandshakeError: wraps errors while preserving bytes already consumed so callers can
//     gracefully fall back to raw TCP/HTTP handling if desired.
//
// The configuration mirrors the CLI behavior: build a Sudoku table via
// sudoku.NewTable(seed, "prefer_ascii"|"prefer_entropy") or sudoku.NewTableWithCustom
// (third arg: custom X/P/V pattern such as "xpxvvpvv"), pick an AEAD (chacha20-poly1305 is
// the default and required when using packed downlink), keep the key and padding settings
// consistent across client/server, and apply an optional handshake timeout on the server side.
package apis
