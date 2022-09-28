//go:build linux
// +build linux

package web

import (
	"crypto/tls"
	"github.com/coreos/go-systemd/v22/activation"
	"github.com/go-chi/chi/v5"
	"github.com/librespeed/speedtest/config"
	log "github.com/sirupsen/logrus"
	"net"
	"net/http"
)

func startListener(conf *config.Config, r *chi.Mux) error {
	// See if systemd socket activation has been used when starting our process
	listeners, err := activation.Listeners()
	if err != nil {
		log.Fatalf("Error whilst checking for systemd socket activation %s", err)
	}

	var s error

	switch len(listeners) {
	case 0:
		addr := net.JoinHostPort(conf.BindAddress, conf.Port)
		log.Infof("Starting backend server on %s", addr)

		// TLS and HTTP/2.
		if conf.EnableTLS {
			log.Info("Use TLS connection.")
			if !(conf.EnableHTTP2) {
				srv := &http.Server{
					Addr:         addr,
					Handler:      r,
					TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
				}
				s = srv.ListenAndServeTLS(conf.TLSCertFile, conf.TLSKeyFile)
			} else {
				s = http.ListenAndServeTLS(addr, conf.TLSCertFile, conf.TLSKeyFile, r)
			}
		} else {
			if conf.EnableHTTP2 {
				log.Errorf("TLS is mandatory for HTTP/2. Ignore settings that enable HTTP/2.")
			}
			s = http.ListenAndServe(addr, r)
		}
	case 1:
		log.Info("Starting backend server on inherited file descriptor via systemd socket activation")
		if conf.BindAddress != "" || conf.Port != "" {
			log.Errorf("Both an address/port (%s:%s) has been specificed in the config AND externally configured socket activation has been detected", conf.BindAddress, conf.Port)
			log.Fatal(`Please deconfigure socket activation (e.g. in systemd unit files), or set both 'bind_address' and 'listen_port' to ''`)
		}
		s = http.Serve(listeners[0], r)
	default:
		log.Fatalf("Asked to listen on %d sockets via systemd activation.  Sorry we currently only support listening on 1 socket.", len(listeners))
	}
	return s
}
