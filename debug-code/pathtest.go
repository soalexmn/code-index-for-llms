package main

import (
	"fmt"
	"path/filepath"
	"os"
)

func main() {
	p, _ := filepath.Abs("/tmp/pytest")
	fmt.Println("filepath.Abs /tmp/pytest:", p)
	fmt.Println("os.TempDir:", os.TempDir())
}
