package accounts

import "errors"

// The sizes of the admin app a user chooses among (see User.InterfaceSize):
// InterfaceCompact has screens from 48rem up 15 % denser than a phone,
// InterfaceLarge keeps a phone's sizes on every screen.
const (
	InterfaceCompact = "compact"
	InterfaceLarge   = "large"
)

// InterfaceSizes lists the sizes of the admin app, the default first.
var InterfaceSizes = []string{InterfaceCompact, InterfaceLarge}

// ErrInvalidInterfaceSize reports an interface size not in InterfaceSizes.
var ErrInvalidInterfaceSize = errors.New("invalid interface size")
