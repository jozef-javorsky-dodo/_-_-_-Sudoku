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
package apis

import (
	"fmt"
	"net"
)

// HandshakeError wraps errors that occur during the handshake process.
// When this error is returned, the caller may perform fallback handling
// using RawConn, HTTPHeaderData, and ReadData.
//
// Fields:
//   - Err: Original error describing why the handshake failed.
//   - RawConn: The raw TCP connection, available for fallback.
//   - HTTPHeaderData: Header bytes read during the HTTP mask phase (ConsumeHeader stage).
//   - ReadData: Bytes read and recorded during the Sudoku decoding phase.
//
// Data replay order for fallback:
//  1. Write HTTPHeaderData first (if non-empty)
//  2. Then write ReadData (if non-empty)
//  3. Finally forward remaining data from RawConn
//
// Example usage:
//
//	conn, target, err := apis.ServerHandshake(rawConn, cfg)
//	if err != nil {
//	    var hsErr *apis.HandshakeError
//	    if errors.As(err, &hsErr) {
//	        // Perform fallback handling
//	        fallbackConn, _ := net.Dial("tcp", fallbackAddr)
//	        fallbackConn.Write(hsErr.HTTPHeaderData)
//	        fallbackConn.Write(hsErr.ReadData)
//	        io.Copy(fallbackConn, hsErr.RawConn)
//	        io.Copy(hsErr.RawConn, fallbackConn)
//	    }
//	    return
//	}
type HandshakeError struct {
	Err            error
	RawConn        net.Conn
	HTTPHeaderData []byte // Header data from the HTTP mask layer
	ReadData       []byte // Data already read by the Sudoku layer
}

func (e *HandshakeError) Error() string {
	return fmt.Sprintf("sudoku handshake failed: %v", e.Err)
}

func (e *HandshakeError) Unwrap() error {
	return e.Err
}
