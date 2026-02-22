// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess

import "code.hybscloud.com/atomix"

// Serial is a monotonically increasing session identifier.
// Each call to New assigns the next serial value.
type Serial = uint32

// counter is the global monotonic counter for session serials.
var counter atomix.Uint32

// nextSerial returns the next monotonically increasing serial.
func nextSerial() Serial {
	return counter.Add(1)
}
