package output

import (
	"fmt"
	"os"
)

// Println can be used for all general CLI prints and will append a magenta `dctl` prefix to all output
func Println(msg string, opts ...interface{}) {
	msg = fmt.Sprintf("%s dctl %s - %s\n", Magenta.String(), noColor, msg)
	print(msg, opts...)
}

func Error(err error) {
	msg := fmt.Sprintf("%s dctl %s - %s\n", Red.String(), noColor, err.Error())
	print(msg)
}

func PrintlnWithNameAndColor(name, msg string, c Color, opts ...interface{}) {
	msg = fmt.Sprintf("%s %s %s - %s\n", c.String(), name, None.String(), msg)
	print(msg, opts...)
}

func print(msg string, opts ...interface{}) {
	if len(opts) > 0 {
		fmt.Fprintf(os.Stdout, msg, opts...)
		return
	}
	fmt.Fprint(os.Stdout, msg)
}
