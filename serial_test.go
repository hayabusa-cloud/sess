// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess_test

import (
	"testing"

	"code.hybscloud.com/sess"
)

func TestSerialMonotonic(t *testing.T) {
	epA1, _ := sess.New()
	epA2, _ := sess.New()
	epA3, _ := sess.New()

	s1 := epA1.Serial()
	s2 := epA2.Serial()
	s3 := epA3.Serial()

	if s1 >= s2 {
		t.Fatalf("serials not increasing: %d >= %d", s1, s2)
	}
	if s2 >= s3 {
		t.Fatalf("serials not increasing: %d >= %d", s2, s3)
	}
}

func TestEndpointSerial(t *testing.T) {
	epA, epB := sess.New()

	if epA.Serial() != epB.Serial() {
		t.Fatalf("pair serials differ: %d != %d", epA.Serial(), epB.Serial())
	}
}
