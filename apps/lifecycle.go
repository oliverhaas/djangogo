package apps

import (
	"errors"
	"fmt"
)

// Wire runs the second pass: Wire(ctx) of every Wirer in start order. It calls
// Resolve first when that has not happened. An error of an app is wrapped with
// the app name and stops the pass. A nil ctx is passed on as an empty Context,
// so an app can read its fields without a nil check. The wire pass precedes
// the ready pass: a call after Ready is an error.
func (r *Registry) Wire(ctx *Context) error {
	if r.state != stateLoading {
		return errors.New("apps: Wire called after Ready: the wire pass precedes the ready pass")
	}
	if err := r.Resolve(); err != nil {
		return err
	}
	if ctx == nil {
		ctx = &Context{}
	}
	for _, e := range r.inStartOrder() {
		wirer, ok := e.config.(Wirer)
		if !ok {
			continue
		}
		if err := wirer.Wire(ctx); err != nil {
			return fmt.Errorf("apps: %s.Wire: %w", e.name, err)
		}
	}
	return nil
}

// Ready runs the third pass: Ready() of every Initializer in start order. It
// calls Resolve first when that has not happened. An error of an app is
// wrapped with the app name and stops the pass.
//
// The pass runs once (Django's ready flag): a call after it completed does
// nothing, and a call after it failed returns the same error without running
// a hook again. A call from inside a Ready hook is an error, as Django's
// populate() is not reentrant.
func (r *Registry) Ready() error {
	switch r.state {
	case stateReady:
		return nil
	case stateFailed:
		return r.readyErr
	case stateStarting:
		return errReentrant
	}
	if err := r.Resolve(); err != nil {
		return err
	}
	r.state = stateStarting
	for _, e := range r.inStartOrder() {
		hook, ok := e.config.(Initializer)
		if !ok {
			continue
		}
		if err := hook.Ready(); err != nil {
			r.readyErr = fmt.Errorf("apps: %s.Ready: %w", e.name, err)
			r.state = stateFailed
			return r.readyErr
		}
	}
	r.state = stateReady
	return nil
}

// IsReady reports whether Ready completed (apps.ready).
func (r *Registry) IsReady() bool {
	return r.state == stateReady
}
