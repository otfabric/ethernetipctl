// SPDX-License-Identifier: MIT

package eip

import (
	"bytes"
	"testing"

	"github.com/otfabric/ethernetipctl/internal/config"
)

// The connection path of forward-open and forward-close names the configuration instance
// always, instance 0 for --config 0, as go-ethernetip's scanner does for io: the same flags
// give the same path in all three.
func TestConnectionPathAlwaysNamesTheConfigInstance(t *testing.T) {
	for _, c := range []struct {
		name string
		p    config.ConnectionPathConfig
		data []byte
		want []byte
	}{
		{"config 0", config.ConnectionPathConfig{Output: 150, Input: 100},
			nil, []byte{0x20, 0x04, 0x24, 0x00, 0x2C, 0x96, 0x2C, 0x64}},
		{"config 151", config.ConnectionPathConfig{Config: 151, Output: 150, Input: 100},
			nil, []byte{0x20, 0x04, 0x24, 0x97, 0x2C, 0x96, 0x2C, 0x64}},
		{"config 0 with data", config.ConnectionPathConfig{Output: 152, Input: 100},
			[]byte{1, 2}, []byte{0x20, 0x04, 0x24, 0x00, 0x2C, 0x98, 0x2C, 0x64, 0x80, 0x01, 0x01, 0x02}},
	} {
		path, err := connectionPath(c.p, c.data)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got, err := path.Encode()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !bytes.Equal(got, c.want) {
			t.Errorf("%s: path % X, want % X", c.name, got, c.want)
		}
	}
}
