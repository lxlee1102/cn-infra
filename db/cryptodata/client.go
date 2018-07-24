//  Copyright (c) 2018 Cisco and/or its affiliates.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at:
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package cryptodata

import (
	"crypto/rsa"
	"github.com/ligato/cn-infra/db/keyval"
	"errors"
	"crypto/rand"
	"io"
	"encoding/base64"
	"hash"
	"crypto/sha256"
)

// ClientConfig is result of converting Config.PrivateKeyFile to PrivateKey
type ClientConfig struct {
	// Private key is used to decrypt encrypted keys while reading them from store
	PrivateKeys []*rsa.PrivateKey
	// Reader used for encrypting/decrypting
	Reader io.Reader
	// Hash function used for hashing while encrypting
	Hash hash.Hash
}

// Client handles encrypting/decrypting and wrapping data
type Client struct {
	ClientConfig
}

// NewClient creates new client from provided config and reader
func NewClient(clientConfig ClientConfig) (client *Client) {
	client = &Client{
		ClientConfig: clientConfig,
	}

	// If reader is nil use default rand.Reader
	if client.Reader == nil {
		client.Reader = rand.Reader
	}

	// If hash is nil use default sha256
	if client.Hash == nil {
		client.Hash = sha256.New()
	}

	return
}

// EncryptData encrypts input data using provided public key
func (client *Client) EncryptData(inData []byte, pub *rsa.PublicKey) (data []byte, err error) {
	data, err = rsa.EncryptOAEP(client.Hash, client.Reader, pub, inData, nil)
	data = []byte(base64.URLEncoding.EncodeToString(data))
	return
}

// DecryptData decrypts input data
func (client *Client) DecryptData(inData []byte) (data []byte, err error) {
	inData, err = base64.URLEncoding.DecodeString(string(inData))
	if err != nil {
		return
	}

	for _, key := range client.PrivateKeys {
		data, err := rsa.DecryptOAEP(client.Hash, client.Reader, key, inData, nil)

		if err == nil {
			return data, nil
		}
	}

	return nil, errors.New("failed to decrypt data due to no private key matching")
}

// Wrap wraps core broker watcher with support for decrypting encrypted keys
func (client *Client) Wrap(cbw keyval.CoreBrokerWatcher, decrypter ArbitraryDecrypter) keyval.CoreBrokerWatcher {
	return NewCoreBrokerWatcherWrapper(cbw, decrypter, client.DecryptData)
}
