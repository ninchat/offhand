package relink

import (
	"errors"
)

// Error values returned by Binder, Endpoint, Link and OutgoingChannel methods.
var (
	ErrBinderClosed         = errors.New("binder closed")
	ErrBindingAlreadyExists = errors.New("binding already exists")
	ErrChannelClosed        = errors.New("channel closed")
	ErrEndpointClosed       = errors.New("endpoint closed")
	ErrLinkClosed           = errors.New("link closed")
	ErrLinkLost             = errors.New("link lost")
	ErrLinkTimeout          = errors.New("link timeout")
	ErrMessageTimeout       = errors.New("message timeout")
)
