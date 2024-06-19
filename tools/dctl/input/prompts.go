package input

import (
	"bufio"
	"os"
)

// Get is a helper function to get input from the user. If the user types "quit" then the program will exit.
func Get() string {
	var i string
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		i = scanner.Text()
	}
	if i == "quit" {
		os.Exit(0)
	}
	return i
}

// ConfirmDefaultDeny is a helper function to confirm with the user for a question. It will only return true if the user types "yes".
// If the user types "quit" then the program will exit. This method does NOT offer a text prompt to the user. You must do that
// before calling this method. For example
//
//		output.Println("Are you sure you want to do this? (yes/NO)")
//	    if !input.ConfirmDefaultDeny() {
//	        return
//	    }
//		output.Println("Doing the thing...")
//
// The prompt can be whatever you want, but you should include the final `(yes/NO)` part as if the user does
// not type anything it will default to "no".
func ConfirmDefaultDeny() bool {
	i := Get()
	return i == "yes"
}

// ConfirmDefaultAllow is a helper function to confirm with the user for a question. It will only return false if the user types "no".
// If the user types "quit" then the program will exit. This method does NOT offer a text prompt to the user. You must do that
// before calling this method. For example
//
//		output.Println("Are you sure you want to do this? (YES/no)")
//	    if input.ConfirmDefaultAllow() {
//	        return
//	    }
//		output.Println("Doing the thing...")
//
// The prompt can be whatever you want, but you should include the final `(YES/no)` part as if the user does
// not type anything it will default to "yes".
func ConfirmDefaultAllow() bool {
	i := Get()
	return i != "no"
}
