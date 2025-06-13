package api

import (
	"crypto/tls"
	"time"
)

type Option func(*Options)

type Options struct {
	ListenAddr      string
	ServerTLSConfig *tls.Config

	// HTTP relevant options
	GracefulTimeout time.Duration
	ReadTimeout     time.Duration
	IdleTimeout     time.Duration
	WriteTimeout    time.Duration
}

func WithListenAddr(laddr string) Option {
	return func(o *Options) {
		o.ListenAddr = laddr
	}
}

func WithServerTLSConfig(cfg *tls.Config) Option {
	return func(o *Options) {
		o.ServerTLSConfig = cfg
	}
}

func WithGracefulTimeout(d time.Duration) Option {
	return func(o *Options) {
		o.GracefulTimeout = d
	}
}

func WithReadTimeout(d time.Duration) Option {
	return func(o *Options) {
		o.ReadTimeout = d
	}
}

func WithIdleTimeout(d time.Duration) Option {
	return func(o *Options) {
		o.IdleTimeout = d
	}
}

func WithWriteTimeout(d time.Duration) Option {
	return func(o *Options) {
		o.WriteTimeout = d
	}
}
