package main

import (
	"errors"
	"fmt"
	login "github.com/kawapiki/Jonsbo-resurrection/internal/startup"
	"os"
	"os/user"
)

func startup(args []string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	args, err = startupCaller(args, u.Uid)
	if err != nil {
		return err
	}
	usage := errors.New("usage: jonsbo startup enable [--elevated] | disable | status")
	if len(args) == 0 {
		return usage
	}
	switch args[0] {
	case "status":
		if len(args) != 1 {
			return usage
		}
		mode, err := login.State()
		if err != nil {
			return err
		}
		fmt.Println(mode)
		return nil
	case "disable":
		if len(args) != 1 {
			return usage
		}
		return login.Disable()
	case "enable":
		if len(args) > 2 || (len(args) == 2 && args[1] != "--elevated") {
			return usage
		}
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		return login.Enable(exe, len(args) == 2)
	default:
		return usage
	}
}

// The unelevated caller supplies its SID so over-the-shoulder UAC credentials
// cannot accidentally register startup for a different account.
func startupCaller(args []string, currentSID string) ([]string, error) {
	if len(args) >= 2 && args[len(args)-2] == "--expected-user-sid" {
		if args[len(args)-1] != currentSID {
			return nil, errors.New("startup elevation must use the same Windows account; different administrator credentials are not supported")
		}
		return args[:len(args)-2], nil
	}
	return args, nil
}
