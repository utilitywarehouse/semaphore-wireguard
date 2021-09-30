package main

import (
	"fmt"
	"strings"
)

var (
	ifaceNameTooLongErr       = fmt.Errorf("Interface name length must be between 1 and 16 characters long")
	ifaceNameContainsSpaceErr = fmt.Errorf("Interface name cannot contain spaces")
	ifaceNameContainsSlashErr = fmt.Errorf("Interface name cannot contain '/' character")
)

// verifyInterfaceName check for a valid interface name based on:
// https://unix.stackexchange.com/questions/451368/allowed-chars-in-linux-network-interface-names
func verifyInterfaceName(name string) error {
	if len(name) == 0 || len(name) > 16 {
		return ifaceNameTooLongErr
	}
	if strings.Contains(name, " ") {
		return ifaceNameContainsSpaceErr
	}
	if strings.Contains(name, "/") {
		return ifaceNameContainsSlashErr
	}
	return nil
}
