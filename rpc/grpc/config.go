// Copyright (c) 2017 Cisco and/or its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package grpc

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/namsral/flag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"go.ligato.io/cn-infra/v2/config"
	"go.ligato.io/cn-infra/v2/infra"
)

// Config is a configuration for GRPC netListener
// It is meant to be extended with security (TLS...)
type Config struct {
	// Endpoint is an address of GRPC netListener
	Endpoint string `json:"endpoint"`

	// Three or four-digit permission setup for unix domain socket file (if used)
	Permission int `json:"permission"`

	// If set and unix type network is used, the existing socket file will be always removed and re-created
	ForceSocketRemoval bool `json:"force-socket-removal"`

	// Network defaults to "tcp" if unset, and can be set to one of the following values:
	// "tcp", "tcp4", "tcp6", "unix", "unixpacket" or any other value accepted by net.Listen
	Network string `json:"network"`

	// MaxMsgSize returns a ServerOption to set the max message size in bytes for inbound mesages.
	// If this is not set, gRPC uses the default 4MB.
	MaxMsgSize int `json:"max-msg-size"`

	// MaxConcurrentStreams returns a ServerOption that will apply a limit on the number
	// of concurrent streams to each ServerTransport.
	MaxConcurrentStreams uint32 `json:"max-concurrent-streams"`

	// TLS info:
	InsecureTransport bool     `json:"insecure-transport"`
	Certfile          string   `json:"cert-file"`
	Keyfile           string   `json:"key-file"`
	CAfiles           []string `json:"ca-files"`

	// ExtendedLogging enables detailed GRPC logging
	ExtendedLogging bool `json:"extended-logging"`

	// PrometheusMetrics enables prometheus metrics for gRPC client.
	PrometheusMetrics bool `json:"prometheus-metrics"`

	// Compression for inbound/outbound messages.
	// Supported only gzip.
	//TODO Compression string

	// see keepalive.go
	KeepaliveEnable              bool   `json:"keepalive-enable"`
	KeepaliveClientPingMinTime   uint32 `json:"keepalive-client-ping-min-interval"`
	KeepalivePermitWithoutStream bool   `json:"keepalive-permit-without-stream"`
	KeepaliveIdlePingInterval    uint32 `json:"keepalive-idle-ping-interval"`
	KeepalivePingTimeout         uint32 `json:"keepalive-ping-timeout"`
	KeepaliveMaxConnectionAge    uint32 `json:"keepalive-max-connection-age"`
	KeepaliveMaxConnectionIdle   uint32 `json:"keepalive-max-connection-idle"`
}

func (cfg *Config) getGrpcOptions() (opts []grpc.ServerOption) {
	if cfg.MaxConcurrentStreams > 0 {
		opts = append(opts, grpc.MaxConcurrentStreams(cfg.MaxConcurrentStreams))
	}
	if cfg.MaxMsgSize > 0 {
		opts = append(opts, grpc.MaxRecvMsgSize(cfg.MaxMsgSize))
	}

	if cfg.KeepaliveEnable {
		kaep := keepalive.EnforcementPolicy{
			MinTime:             20 * time.Second,
			PermitWithoutStream: false,
		}
		kasp := keepalive.ServerParameters{
			Time:    7200 * time.Second,
			Timeout: 20 * time.Second,
		}

		if cfg.KeepaliveClientPingMinTime > 0 {
			kaep.MinTime = time.Duration(cfg.KeepaliveClientPingMinTime) * time.Second
		}
		if cfg.KeepalivePermitWithoutStream {
			kaep.PermitWithoutStream = cfg.KeepalivePermitWithoutStream
		}

		if cfg.KeepalivePingTimeout > 0 {
			kasp.Timeout = time.Duration(cfg.KeepalivePingTimeout) * time.Second
		}
		if cfg.KeepaliveIdlePingInterval > 0 {
			kasp.Time = time.Duration(cfg.KeepaliveIdlePingInterval) * time.Second
		}
		if cfg.KeepaliveMaxConnectionAge > 0 {
			kasp.MaxConnectionAge = time.Duration(cfg.KeepaliveMaxConnectionAge) * time.Second
		}
		if cfg.KeepaliveMaxConnectionIdle > 0 {
			kasp.MaxConnectionIdle = time.Duration(cfg.KeepaliveMaxConnectionIdle) * time.Second
		}

		opts = append(opts, grpc.KeepaliveEnforcementPolicy(kaep), grpc.KeepaliveParams(kasp))
	}

	return
}

func (cfg *Config) getTLS() (*tls.Config, error) {
	// Check if explicitly disabled.
	if cfg.InsecureTransport {
		return nil, nil
	}
	// Minimal requirement is to get cert and key for enabling TLS.
	if cfg.Certfile == "" && cfg.Keyfile == "" {
		return nil, nil
	}

	cert, err := tls.LoadX509KeyPair(cfg.Certfile, cfg.Keyfile)
	if err != nil {
		return nil, err
	}
	tc := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	}

	// Check if we want verify client's certificate against custom CA
	if len(cfg.CAfiles) > 0 {
		caCertPool := x509.NewCertPool()
		for _, c := range cfg.CAfiles {
			cert, err := ioutil.ReadFile(c)
			if err != nil {
				return nil, err
			}

			if !caCertPool.AppendCertsFromPEM(cert) {
				return nil, fmt.Errorf("failed to add CA from '%s' file", c)
			}
		}
		tc.ClientCAs = caCertPool
		tc.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return tc, nil
}

func (cfg *Config) getSocketType() string {
	// Default to tcp socket type of not specified for backward compatibility
	if cfg.Network == "" {
		return "tcp"
	}
	return cfg.Network
}

// GetPort parses suffix from endpoint & returns integer after last ":" (otherwise it returns 0)
func (cfg *Config) GetPort() int {
	if cfg.Endpoint != "" && cfg.Endpoint != ":" {
		index := strings.LastIndex(cfg.Endpoint, ":")
		if index >= 0 {
			port, err := strconv.Atoi(cfg.Endpoint[index+1:])
			if err == nil {
				return port
			}
		}
	}

	return 0
}

// DeclareGRPCPortFlag declares GRPC port (with usage & default value) a flag for a particular plugin name
func DeclareGRPCPortFlag(pluginName infra.PluginName) {
	plugNameUpper := strings.ToUpper(string(pluginName))

	usage := "Configure Agent' " + plugNameUpper + " net listener (port & timeouts); also set via '" +
		plugNameUpper + config.EnvSuffix + "' env variable."
	flag.String(grpcPortFlag(pluginName), "", usage)
}

func grpcPortFlag(pluginName infra.PluginName) string {
	return strings.ToLower(string(pluginName)) + "-port"
}
