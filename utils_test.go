package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerifyInterfaceName(t *testing.T) {
	err := verifyInterfaceName("")
	assert.Equal(t, ifaceNameTooLongErr, err)
	err = verifyInterfaceName("abcdefghijklmnopqrstuvwxyz")
	assert.Equal(t, ifaceNameTooLongErr, err)
	err = verifyInterfaceName(" ")
	assert.Equal(t, ifaceNameContainsSpaceErr, err)
	err = verifyInterfaceName("contains space")
	assert.Equal(t, ifaceNameContainsSpaceErr, err)
	err = verifyInterfaceName("wireguard/test")
	assert.Equal(t, ifaceNameContainsSlashErr, err)
	err = verifyInterfaceName("wireguard.test")
	assert.Equal(t, nil, err)
}
