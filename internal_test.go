// ©Hayabusa Cloud Co., Ltd. 2026. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package sess

import (
	"errors"
	"testing"

	"code.hybscloud.com/iox"
	"code.hybscloud.com/kont"
)

type failingSessionDispatcher struct {
	err   error
	calls int
}

func (d *failingSessionDispatcher) DispatchSession(*sessionContext) (kont.Resumed, error) {
	d.calls++
	return nil, d.err
}

func TestDispatchWaitPanicsOnUnexpectedError(t *testing.T) {
	d := &failingSessionDispatcher{err: errors.New("boom")}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for unexpected session dispatcher error")
		}
		msg, ok := r.(string)
		if !ok || msg != "sess: dispatch returned unexpected error: boom" {
			t.Fatalf("unexpected panic: %v", r)
		}
		if d.calls != 1 {
			t.Fatalf("dispatch calls = %d, want 1", d.calls)
		}
	}()

	dispatchWait(&sessionContext{}, d)
}

func TestDispatchWaitPanicsOnErrMore(t *testing.T) {
	d := &failingSessionDispatcher{err: iox.ErrMore}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for ErrMore in session dispatcher")
		}
		msg, ok := r.(string)
		if !ok || msg != "sess: dispatch returned unexpected error: io: expect more" {
			t.Fatalf("unexpected panic: %v", r)
		}
		if d.calls != 1 {
			t.Fatalf("dispatch calls = %d, want 1", d.calls)
		}
	}()

	dispatchWait(&sessionContext{}, d)
}
