package ui

import (
	"fmt"
	"os"
)

func Info(format string, a ...any) {
	fmt.Println(infoStyle.Render(fmt.Sprintf(format, a...)))
}

func Error(format string, a ...any) {
	fmt.Fprintln(os.Stderr, errorStyle.Render("Error: "+fmt.Sprintf(format, a...)))
}
