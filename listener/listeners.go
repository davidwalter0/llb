package listener

import (
	"github.com/davidwalter0/llb/tracer"
)

// Equal compares two ManagedListener objects
func (lhs *ManagedListener) Equal(rhs *ManagedListener) bool {
	defer trace.Tracer.ScopedTrace()()
	return lhs.Key == rhs.Key &&
		lhs.Source == rhs.Source &&
		lhs.Sink == rhs.Sink &&
		lhs.EnableEp == rhs.EnableEp &&
		lhs.Name == rhs.Name &&
		lhs.Namespace == rhs.Namespace &&
		lhs.Mode == rhs.Mode &&
		lhs.Debug == rhs.Debug
}

// Copy points w/o erasing EndPoints
func (lhs *ManagedListener) Copy(rhs *ManagedListener) *ManagedListener {
	lhs.Key = rhs.Key
	lhs.Source = rhs.Source
	lhs.Sink = rhs.Sink
	lhs.EnableEp = rhs.EnableEp
	lhs.Name = rhs.Name
	lhs.Namespace = rhs.Namespace
	lhs.Debug = rhs.Debug
	lhs.Mode = rhs.Mode
	lhs.Active = rhs.Active
	return lhs
}
