package command

import "fmt"

func Remove(appimage string, keep bool) {
	fmt.Println("removing:", appimage)
	fmt.Println("keep file:", keep)
}
